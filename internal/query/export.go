package query

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entauditlog "github.com/whg517/sqlflow/internal/db/ent/auditlog"
	"github.com/whg517/sqlflow/internal/db/ent/predicate"
	entticket "github.com/whg517/sqlflow/internal/db/ent/ticket"
	entuser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/security"
)

const (
	// ExportMaxRows is the maximum number of rows allowed in a single export.
	ExportMaxRows = 10000
	// ExportStreamFlushInterval controls how often to flush during streaming.
	ExportStreamFlushInterval = 1000
)

var (
	// ErrExportExceedsLimit indicates the export result exceeds the maximum allowed rows.
	ErrExportExceedsLimit = errors.New("导出数据超过10000行上限，请添加筛选条件缩小范围")
	// ErrExportNoPermission indicates the user lacks export permission.
	ErrExportNoPermission = errors.New("没有导出权限")
	// ErrExportTypeInvalid indicates an invalid export type.
	ErrExportTypeInvalid = errors.New("不支持的导出类型，仅支持 audit 和 ticket")
)

// ExportType defines the type of data that can be exported.
type ExportType string

const (
	ExportTypeAudit  ExportType = "audit"
	ExportTypeTicket ExportType = "ticket"
)

// ExportResult holds the generated export CSV content and metadata.
type ExportResult struct {
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	TotalRows   int64     `json:"total_rows"`
	GeneratedAt time.Time `json:"generated_at"`
	// CSVBytes is the full CSV content including BOM header.
	CSVBytes []byte `json:"-"`
}

// ExportActor is the authenticated caller an export runs as.
//
// Every export entry point takes one, because the row boundary is derived from
// the actor rather than from the request: the filter structs carry no owner
// field, so no query parameter can widen what an export returns.
type ExportActor struct {
	UserID   int64
	Username string
	Role     string
}

// exportObjects names the Casbin object that guards the platform-wide view of
// each export type. They live in the system domain because exporting tickets or
// audit logs is a platform-management act, not one scoped to a datasource.
var exportObjects = map[ExportType]string{
	ExportTypeAudit:  "audit",
	ExportTypeTicket: "ticket",
}

// ExportService handles data export for audit logs and tickets.
type ExportService struct {
	client   *ent.Client
	permSvc  *security.Service
	auditSvc auditlog.Writer
}

// NewExportService creates a new ExportService.
func NewExportService(database *db.DB, permSvc *security.Service, auditSvc auditlog.Writer) *ExportService {
	return &ExportService{
		client:   database.Client(),
		permSvc:  permSvc,
		auditSvc: auditlog.OrDiscard(auditSvc),
	}
}

// canExportAll reports whether the actor may export every record of a class,
// as opposed to only the ones they own.
//
// This replaced a hand-written role switch whose ticket branch returned true for
// every role — including unknown and empty — while the query it guarded applied
// no owner predicate. Any authenticated developer could therefore download every
// ticket in the system: SQL, change reason, submitter and reviewer identity.
// The decision now runs through the same EnforceActor seam the query export path
// uses, so widening it takes a policy rather than an edit to a switch statement.
func (s *ExportService) canExportAll(ctx context.Context, actor ExportActor, exportType ExportType) (bool, error) {
	obj, ok := exportObjects[exportType]
	if !ok {
		return false, ErrExportTypeInvalid
	}
	if s.permSvc == nil {
		// Without a policy engine there is no way to establish the grant.
		// Refusing narrows a ticket export to the actor's own rows and denies an
		// audit export outright; granting would reinstate the defect above.
		return false, nil
	}
	allowed, err := s.permSvc.EnforceActor(ctx, actor.UserID, actor.Role, authz.SystemDomain, obj, "export")
	if err != nil {
		return false, fmt.Errorf("导出权限校验失败: %w", err)
	}
	return allowed, nil
}

// authorizeExportRequest answers whether the actor may start an export of this
// class at all, before any row is read.
//
// It is not the row boundary. An audit export is all-or-nothing; a ticket export
// is open to every authenticated user and narrowed to their own tickets in
// ticketExportPredicates. Both answers come from canExportAll, so there is one
// policy question — asked here so a refused request fails before it queries, and
// again where the query is built, which is the copy no caller can skip.
func (s *ExportService) authorizeExportRequest(ctx context.Context, actor ExportActor, exportType ExportType) error {
	allowed, err := s.canExportAll(ctx, actor, exportType)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}
	if exportType == ExportTypeTicket && actor.UserID > 0 {
		// A submitter exports their own tickets, exactly as the ticket list
		// shows them their own. Without an identity there is nothing to scope
		// to, so that case falls through to the refusal below.
		return nil
	}
	// Audit logs have no per-actor slice: the whole class is governance
	// evidence, so the platform-wide grant is the only way in.
	return ErrExportNoPermission
}

// ValidateExport checks permissions and row count before exporting.
// Returns total rows or an error if export is not allowed.
func (s *ExportService) ValidateExport(ctx context.Context, actor ExportActor, exportType ExportType, filters interface{}) (int64, error) {
	if err := s.authorizeExportRequest(ctx, actor, exportType); err != nil {
		return 0, err
	}

	var total int64
	var err error

	switch exportType {
	case ExportTypeAudit:
		auditFilters, ok := filters.(AuditExportFilters)
		if !ok {
			return 0, ErrExportTypeInvalid
		}
		total, err = s.countAuditLogs(ctx, actor, auditFilters)
	case ExportTypeTicket:
		ticketFilters, ok := filters.(TicketExportFilters)
		if !ok {
			return 0, ErrExportTypeInvalid
		}
		total, err = s.countTickets(ctx, actor, ticketFilters)
	default:
		return 0, ErrExportTypeInvalid
	}

	if err != nil {
		return 0, err
	}

	if total > ExportMaxRows {
		return total, ErrExportExceedsLimit
	}

	return total, nil
}

// countAuditLogs counts audit logs matching the filters.
func (s *ExportService) countAuditLogs(ctx context.Context, actor ExportActor, filters AuditExportFilters) (int64, error) {
	preds, err := s.auditExportPredicates(ctx, actor, filters)
	if err != nil {
		return 0, err
	}
	total, err := s.client.AuditLog.Query().Where(preds...).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("统计审计日志失败: %w", err)
	}
	return int64(total), nil
}

// countTickets counts tickets matching the filters.
//
// It shares ticketExportPredicates with the row readers, so the count the caller
// is shown and the rows it later streams answer to the same boundary.
func (s *ExportService) countTickets(ctx context.Context, actor ExportActor, filters TicketExportFilters) (int64, error) {
	preds, err := s.ticketExportPredicates(ctx, actor, filters)
	if err != nil {
		return 0, err
	}
	total, err := s.client.Ticket.Query().Where(preds...).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("统计工单失败: %w", err)
	}
	return int64(total), nil
}

// ---------------------------------------------------------------------------
// Streaming Export
// ---------------------------------------------------------------------------

// StreamExportAuditLogs streams audit logs as CSV to the given writer.
// The caller is responsible for writing the BOM header before calling this.
// Returns total rows written.
func (s *ExportService) StreamExportAuditLogs(ctx context.Context, w io.Writer, actor ExportActor, filters AuditExportFilters, columns map[string]int) (int64, error) {
	csvW := csv.NewWriter(w)
	defer csvW.Flush()

	// Header and cells are selected by the same index list.
	//
	// The header used to be a second literal copy of auditColumnNames and the
	// rows were written out in full, so a caller that asked for three columns
	// got sixteen. The Excel writer already honored the selection; carrying it
	// here is what makes the format an output detail rather than a difference
	// in what the export contains.
	colIndices := sortColumnIndices(columns, len(auditColumnNames))
	_ = csvW.Write(selectStrings(auditColumnNames, colIndices))

	rows, err := s.fetchAuditExportRows(ctx, actor, filters)
	if err != nil {
		return 0, err
	}

	var written int64
	for _, a := range rows {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		createdAtStr := ""
		if !a.CreatedAt.IsZero() {
			createdAtStr = a.CreatedAt.Format("2006-01-02 15:04:05")
		}

		_ = csvW.Write(selectStrings([]string{
			fmt.Sprintf("%d", a.ID),
			createdAtStr,
			escapeCSVFormula(a.Username),
			escapeCSVFormula(a.Action),
			fmt.Sprintf("%d", a.DatasourceID),
			escapeCSVFormula(a.Database),
			escapeCSVFormula(a.SQLContent),
			escapeCSVFormula(a.SQLSummary),
			fmt.Sprintf("%d", a.ResultRows),
			fmt.Sprintf("%d", a.AffectedRows),
			fmt.Sprintf("%d", a.ExecutionTimeMs),
			escapeCSVFormula(a.ErrorMessage),
			escapeCSVFormula(a.DesensitizedFields),
			escapeCSVFormula(a.IPAddress),
			escapeCSVFormula(a.AIReviewResult),
			fmt.Sprintf("%d", a.TicketID),
		}, colIndices))
		written++

		if written%ExportStreamFlushInterval == 0 {
			csvW.Flush()
		}
	}

	return written, nil
}

// StreamExportTickets streams tickets as CSV to the given writer.
// The caller is responsible for writing the BOM header before calling this.
func (s *ExportService) StreamExportTickets(ctx context.Context, w io.Writer, actor ExportActor, filters TicketExportFilters, columns map[string]int) (int64, error) {
	csvW := csv.NewWriter(w)
	defer csvW.Flush()

	colIndices := sortColumnIndices(columns, len(ticketColumnNames))
	_ = csvW.Write(selectStrings(ticketColumnNames, colIndices))

	rows, err := s.fetchTicketExportRows(ctx, actor, filters)
	if err != nil {
		return 0, err
	}

	var written int64
	for _, t := range rows {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		scheduledAtStr := formatOptionalTime(t.ScheduledAt)
		executedAtStr := formatOptionalTime(t.ExecutedAt)

		_ = csvW.Write(selectStrings([]string{
			fmt.Sprintf("%d", t.ID),
			escapeCSVFormula(t.SubmitterName),
			fmt.Sprintf("%d", t.SubmitterID),
			fmt.Sprintf("%d", t.DatasourceID),
			escapeCSVFormula(t.Database),
			escapeCSVFormula(t.SQLContent),
			escapeCSVFormula(t.SQLSummary),
			escapeCSVFormula(t.DBType),
			escapeCSVFormula(t.ChangeReason),
			escapeCSVFormula(t.Status),
			escapeCSVFormula(t.RiskLevel),
			escapeCSVFormula(t.ReviewerName),
			escapeCSVFormula(t.ReviewComment),
			scheduledAtStr,
			executedAtStr,
			t.CreatedAt.Format("2006-01-02 15:04:05"),
			t.UpdatedAt.Format("2006-01-02 15:04:05"),
		}, colIndices))
		written++

		if written%ExportStreamFlushInterval == 0 {
			csvW.Flush()
		}
	}

	return written, nil
}

// ---------------------------------------------------------------------------
// Synchronous Export (legacy, kept for backward compatibility)
// ---------------------------------------------------------------------------

// ExportAuditLogs exports audit logs as CSV with the given filters (synchronous, in-memory).
// Deprecated: Use StreamExportAuditLogs for streaming, or ValidateExport + StreamExportAuditLogs.
func (s *ExportService) ExportAuditLogs(ctx context.Context, actor ExportActor, filters AuditExportFilters) (*ExportResult, error) {
	if err := s.authorizeExportRequest(ctx, actor, ExportTypeAudit); err != nil {
		return nil, err
	}

	total, err := s.countAuditLogs(ctx, actor, filters)
	if err != nil {
		return nil, err
	}
	if total > ExportMaxRows {
		return nil, ErrExportExceedsLimit
	}

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM

	written, err := s.StreamExportAuditLogs(ctx, &buf, actor, filters, nil)
	if err != nil {
		return nil, err
	}

	// Append watermark
	fmt.Fprintf(&buf, "\n# 导出水印: 导出人=%s | 导出时间=%s | 仅限内部使用\n",
		actor.Username,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)

	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     actor.UserID,
		Action:     "audit_export",
		ResultRows: written,
	})

	return &ExportResult{
		Filename:    fmt.Sprintf("audit_logs_%s.csv", time.Now().Format("2006-01-02")),
		ContentType: "text/csv; charset=utf-8",
		TotalRows:   total,
		GeneratedAt: time.Now(),
		CSVBytes:    []byte(buf.String()),
	}, nil
}

// ExportTickets exports tickets as CSV with the given filters (synchronous, in-memory).
// Deprecated: Use StreamExportTickets for streaming, or ValidateExport + StreamExportTickets.
func (s *ExportService) ExportTickets(ctx context.Context, actor ExportActor, filters TicketExportFilters) (*ExportResult, error) {
	if err := s.authorizeExportRequest(ctx, actor, ExportTypeTicket); err != nil {
		return nil, err
	}

	total, err := s.countTickets(ctx, actor, filters)
	if err != nil {
		return nil, err
	}
	if total > ExportMaxRows {
		return nil, ErrExportExceedsLimit
	}

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM

	written, err := s.StreamExportTickets(ctx, &buf, actor, filters, nil)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(&buf, "\n# 导出水印: 导出人=%s | 导出时间=%s | 仅限内部使用\n",
		actor.Username,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)

	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     actor.UserID,
		Action:     "ticket_export",
		ResultRows: written,
	})

	return &ExportResult{
		Filename:    fmt.Sprintf("tickets_%s.csv", time.Now().Format("2006-01-02")),
		ContentType: "text/csv; charset=utf-8",
		TotalRows:   total,
		GeneratedAt: time.Now(),
		CSVBytes:    []byte(buf.String()),
	}, nil
}

// ---------------------------------------------------------------------------
// Filter helpers
// ---------------------------------------------------------------------------

// auditExportPredicates translates the export filters into typed predicates.
//
// The grant check lives here rather than only at the entry points because every
// path that reads audit rows — count, CSV stream, Excel stream — builds its
// WHERE clause through this function. A future caller that forgets to authorize
// gets an error, not the whole table.
//
// The keyword also matches the acting user's name, which lives on a joined
// table that typed predicates cannot reach. It is resolved to a set of user ids
// first: one extra query, and the whole filter stays expressible without raw
// SQL. This mirrors what the audit domain's own search does.
func (s *ExportService) auditExportPredicates(ctx context.Context, actor ExportActor, filters AuditExportFilters) ([]predicate.AuditLog, error) {
	allowed, err := s.canExportAll(ctx, actor, ExportTypeAudit)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrExportNoPermission
	}

	var preds []predicate.AuditLog
	if id, err := strconv.ParseInt(filters.UserID, 10, 64); err == nil && filters.UserID != "" {
		preds = append(preds, entauditlog.UserIDEQ(id))
	}
	if filters.Action != "" {
		preds = append(preds, entauditlog.ActionEQ(filters.Action))
	}
	if id, err := strconv.ParseInt(filters.DatasourceID, 10, 64); err == nil && filters.DatasourceID != "" {
		preds = append(preds, entauditlog.DatasourceIDEQ(id))
	}
	if from, ok := parseDayStart(filters.Start); ok {
		preds = append(preds, entauditlog.CreatedAtGTE(from))
	}
	if to, ok := parseDayStart(filters.End); ok {
		// A date bound resolves to midnight, which would drop everything
		// recorded on the end date the caller asked for.
		preds = append(preds, entauditlog.CreatedAtLT(to.AddDate(0, 0, 1)))
	}
	if filters.Keyword != "" {
		userIDs, err := s.client.User.Query().
			Where(entuser.UsernameContainsFold(filters.Keyword)).
			IDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("解析用户关键词失败: %w", err)
		}
		match := []predicate.AuditLog{
			entauditlog.SQLContentContainsFold(filters.Keyword),
			entauditlog.SQLSummaryContainsFold(filters.Keyword),
			entauditlog.ActionContainsFold(filters.Keyword),
			entauditlog.ErrorMessageContainsFold(filters.Keyword),
			entauditlog.DatabaseContainsFold(filters.Keyword),
			entauditlog.IPAddressContainsFold(filters.Keyword),
		}
		if len(userIDs) > 0 {
			ids := make([]int64, len(userIDs))
			for i, id := range userIDs {
				ids[i] = int64(id)
			}
			match = append(match, entauditlog.UserIDIn(ids...))
		}
		preds = append(preds, entauditlog.Or(match...))
	}
	return preds, nil
}

// ticketExportPredicates translates the ticket export filters and confines the
// result to the rows the actor is allowed to see.
//
// The owner boundary is applied here, not by the callers, because every path
// that reads ticket rows — count, CSV stream, Excel stream, and the async worker
// behind all three — builds its WHERE clause through this function. It is the
// same boundary ticket.Service.ListTickets draws: an actor without the
// platform-wide grant sees only what they submitted. Export is a second
// entrance to those rows, and a second entrance that answered differently is
// exactly how a developer came to be able to download every ticket in the
// system.
func (s *ExportService) ticketExportPredicates(ctx context.Context, actor ExportActor, filters TicketExportFilters) ([]predicate.Ticket, error) {
	allowed, err := s.canExportAll(ctx, actor, ExportTypeTicket)
	if err != nil {
		return nil, err
	}

	var preds []predicate.Ticket
	if !allowed {
		if actor.UserID <= 0 {
			// No grant and no identity to scope to. Refusing beats returning an
			// empty file, which reads like "there are no tickets".
			return nil, ErrExportNoPermission
		}
		preds = append(preds, entticket.SubmitterIDEQ(actor.UserID))
	}
	if filters.Status != "" {
		preds = append(preds, entticket.StatusEQ(filters.Status))
	}
	if id, err := strconv.ParseInt(filters.DatasourceID, 10, 64); err == nil && filters.DatasourceID != "" {
		preds = append(preds, entticket.DatasourceIDEQ(id))
	}
	if filters.RiskLevel != "" {
		preds = append(preds, entticket.RiskLevelEQ(filters.RiskLevel))
	}
	if filters.Keyword != "" {
		preds = append(preds, entticket.Or(
			entticket.SQLContentContainsFold(filters.Keyword),
			entticket.ChangeReasonContainsFold(filters.Keyword),
			entticket.SQLSummaryContainsFold(filters.Keyword),
		))
	}
	return preds, nil
}

// ---------------------------------------------------------------------------
// Types and helpers
// ---------------------------------------------------------------------------

// AuditExportFilters holds the filter parameters for audit log export.
type AuditExportFilters struct {
	UserID       string `json:"user_id"`
	Action       string `json:"action"`
	DatasourceID string `json:"datasource_id"`
	Start        string `json:"start"`
	End          string `json:"end"`
	Keyword      string `json:"keyword"`
}

// TicketExportFilters holds the filter parameters for ticket export.
type TicketExportFilters struct {
	Status       string `json:"status"`
	DatasourceID string `json:"datasource_id"`
	RiskLevel    string `json:"risk_level"`
	Keyword      string `json:"keyword"`
}

// auditCSVRow holds the scan target for a single audit log row.
//
// Flat rather than embedding model.AuditLog: ent's scanner matches result
// columns to fields by tag and does not descend into embedded structs. The
// omitempty tags the model carries would also not match the columns, since the
// scanner reads the tag name verbatim.
type auditCSVRow struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id"`
	Username           string    `json:"username"`
	Action             string    `json:"action"`
	DatasourceID       int64     `json:"datasource_id"`
	Database           string    `json:"database"`
	SQLContent         string    `json:"sql_content"`
	SQLSummary         string    `json:"sql_summary"`
	ResultRows         int64     `json:"result_rows"`
	AffectedRows       int64     `json:"affected_rows"`
	ExecutionTimeMs    int64     `json:"execution_time_ms"`
	ErrorMessage       string    `json:"error_message"`
	DesensitizedFields string    `json:"desensitized_fields"`
	IPAddress          string    `json:"ip_address"`
	AIReviewResult     string    `json:"ai_review_result"`
	TicketID           int64     `json:"ticket_id"`
	CreatedAt          time.Time `json:"created_at"`
}

// fetchAuditExportRows reads the rows an audit export writes out.
//
// Shared by the CSV and Excel writers, which each built the same query with
// their own placeholder numbering. One of them got it wrong: the Excel path
// interpolated a WHERE clause written with ? and then hard-coded $1 for the
// limit, so every filtered Excel export failed with a syntax error. Only the
// unfiltered case — the one the tests covered — produced a valid statement.
func (s *ExportService) fetchAuditExportRows(ctx context.Context, actor ExportActor, filters AuditExportFilters) ([]auditCSVRow, error) {
	preds, err := s.auditExportPredicates(ctx, actor, filters)
	if err != nil {
		return nil, err
	}
	var rows []auditCSVRow
	err = s.client.AuditLog.Query().
		Where(preds...).
		Modify(func(sel *entsql.Selector) {
			u := entsql.Table(entuser.Table).As("u")
			sel.LeftJoin(u).On(sel.C(entauditlog.FieldUserID), u.C(entuser.FieldID))
			cols := make([]string, 0, len(entauditlog.Columns)+1)
			for _, c := range entauditlog.Columns {
				cols = append(cols, sel.C(c))
			}
			sel.Select(cols...).
				AppendSelect(entsql.As("COALESCE("+u.C(entuser.FieldUsername)+", '')", "username")).
				OrderBy(entsql.Desc(sel.C(entauditlog.FieldCreatedAt))).
				Limit(ExportMaxRows)
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("查询审计日志失败: %w", err)
	}
	return rows, nil
}

// fetchTicketExportRows reads the rows a ticket export writes out.
//
// Two joins on users, because a ticket names both a submitter and a reviewer.
func (s *ExportService) fetchTicketExportRows(ctx context.Context, actor ExportActor, filters TicketExportFilters) ([]ticketCSVRow, error) {
	preds, err := s.ticketExportPredicates(ctx, actor, filters)
	if err != nil {
		return nil, err
	}
	var rows []ticketCSVRow
	err = s.client.Ticket.Query().
		Where(preds...).
		Modify(func(sel *entsql.Selector) {
			su := entsql.Table(entuser.Table).As("su")
			rev := entsql.Table(entuser.Table).As("rev")
			sel.LeftJoin(su).On(sel.C(entticket.FieldSubmitterID), su.C(entuser.FieldID))
			sel.LeftJoin(rev).On(sel.C(entticket.FieldReviewerID), rev.C(entuser.FieldID))
			sel.Select(
				sel.C(entticket.FieldID),
				sel.C(entticket.FieldSubmitterID),
				sel.C(entticket.FieldDatasourceID),
				sel.C(entticket.FieldDatabase),
				sel.C(entticket.FieldSQLContent),
				sel.C(entticket.FieldSQLSummary),
				sel.C(entticket.FieldDbType),
				sel.C(entticket.FieldChangeReason),
				sel.C(entticket.FieldStatus),
				sel.C(entticket.FieldRiskLevel),
				sel.C(entticket.FieldReviewerID),
				sel.C(entticket.FieldReviewComment),
				sel.C(entticket.FieldScheduledAt),
				sel.C(entticket.FieldExecutedAt),
				sel.C(entticket.FieldCreatedAt),
				sel.C(entticket.FieldUpdatedAt),
			).AppendSelect(
				entsql.As("COALESCE("+su.C(entuser.FieldUsername)+", '')", "submitter_name"),
				entsql.As("COALESCE("+rev.C(entuser.FieldUsername)+", '')", "reviewer_name"),
			).OrderBy(entsql.Desc(sel.C(entticket.FieldCreatedAt))).
				Limit(ExportMaxRows)
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("查询工单失败: %w", err)
	}
	return rows, nil
}

// ticketCSVRow holds the scan target for a single ticket row.
// The tags are how ent's scanner maps result columns onto fields; the names
// must match the columns the select produces, including the two joined
// usernames.
type ticketCSVRow struct {
	ID            int64      `json:"id"`
	SubmitterID   int64      `json:"submitter_id"`
	SubmitterName string     `json:"submitter_name"`
	DatasourceID  int64      `json:"datasource_id"`
	Database      string     `json:"database"`
	SQLContent    string     `json:"sql_content"`
	SQLSummary    string     `json:"sql_summary"`
	DBType        string     `json:"db_type"`
	ChangeReason  string     `json:"change_reason"`
	Status        string     `json:"status"`
	RiskLevel     string     `json:"risk_level"`
	ReviewerID    int64      `json:"reviewer_id"`
	ReviewerName  string     `json:"reviewer_name"`
	ReviewComment string     `json:"review_comment"`
	ScheduledAt   *time.Time `json:"scheduled_at"`
	ExecutedAt    *time.Time `json:"executed_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// formatOptionalTime renders a nullable timestamp, or "" when it is unset.
func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// WriteAuditExportLog records an audit log entry for an audit export.
func (s *ExportService) WriteAuditExportLog(ctx context.Context, userID int64, rows int64) {
	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     userID,
		Action:     "audit_export",
		ResultRows: rows,
	})
}

// WriteTicketExportLog records an audit log entry for a ticket export.
func (s *ExportService) WriteTicketExportLog(ctx context.Context, userID int64, rows int64) {
	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     userID,
		Action:     "ticket_export",
		ResultRows: rows,
	})
}

// escapeCSVFormula escapes CSV formula injection by prefixing dangerous characters.
func escapeCSVFormula(s string) string {
	if len(s) == 0 {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// addBOM prepends a UTF-8 BOM to the CSV content for Excel compatibility.
func addBOM(content string) []byte {
	bom := []byte{0xEF, 0xBB, 0xBF}
	return append(bom, content...)
}
