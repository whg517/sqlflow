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

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entauditlog "github.com/whg517/sqlflow/internal/db/ent/auditlog"
	"github.com/whg517/sqlflow/internal/db/ent/predicate"
	entticket "github.com/whg517/sqlflow/internal/db/ent/ticket"
	entuser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
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

// ExportService handles data export for audit logs and tickets.
type ExportService struct {
	client   *ent.Client
	auditSvc auditlog.Writer
}

// NewExportService creates a new ExportService.
func NewExportService(database *db.DB, auditSvc auditlog.Writer) *ExportService {
	return &ExportService{client: database.Client(), auditSvc: auditlog.OrDiscard(auditSvc)}
}

// hasExportPermission checks if a user has export permission.
func (s *ExportService) hasExportPermission(role string, exportType ExportType) bool {
	switch exportType {
	case ExportTypeAudit:
		return role == "admin" || role == "dba"
	case ExportTypeTicket:
		return true
	default:
		return false
	}
}

// ValidateExport checks permissions and row count before exporting.
// Returns total rows or an error if export is not allowed.
func (s *ExportService) ValidateExport(ctx context.Context, role string, exportType ExportType, filters interface{}) (int64, error) {
	if !s.hasExportPermission(role, exportType) {
		return 0, ErrExportNoPermission
	}

	var total int64
	var err error

	switch exportType {
	case ExportTypeAudit:
		auditFilters := filters.(AuditExportFilters)
		total, err = s.countAuditLogs(ctx, auditFilters)
	case ExportTypeTicket:
		ticketFilters := filters.(TicketExportFilters)
		total, err = s.countTickets(ctx, ticketFilters)
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
func (s *ExportService) countAuditLogs(ctx context.Context, filters AuditExportFilters) (int64, error) {
	preds, err := s.auditExportPredicates(ctx, filters)
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
func (s *ExportService) countTickets(ctx context.Context, filters TicketExportFilters) (int64, error) {
	total, err := s.client.Ticket.Query().Where(ticketExportPredicates(filters)...).Count(ctx)
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
func (s *ExportService) StreamExportAuditLogs(ctx context.Context, w io.Writer, username string, filters AuditExportFilters) (int64, error) {
	csvW := csv.NewWriter(w)
	defer csvW.Flush()

	// Write CSV header
	_ = csvW.Write([]string{
		"ID", "时间", "用户", "操作", "数据源ID", "数据库",
		"SQL内容", "SQL摘要", "返回行数", "影响行数", "耗时(ms)",
		"错误信息", "脱敏字段", "IP地址", "AI评审", "工单ID",
	})

	rows, err := s.fetchAuditExportRows(ctx, filters)
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

		_ = csvW.Write([]string{
			fmt.Sprintf("%d", a.ID),
			createdAtStr,
			a.Username,
			a.Action,
			fmt.Sprintf("%d", a.DatasourceID),
			a.Database,
			escapeCSVFormula(a.SQLContent),
			escapeCSVFormula(a.SQLSummary),
			fmt.Sprintf("%d", a.ResultRows),
			fmt.Sprintf("%d", a.AffectedRows),
			fmt.Sprintf("%d", a.ExecutionTimeMs),
			a.ErrorMessage,
			a.DesensitizedFields,
			a.IPAddress,
			a.AIReviewResult,
			fmt.Sprintf("%d", a.TicketID),
		})
		written++

		if written%ExportStreamFlushInterval == 0 {
			csvW.Flush()
		}
	}

	return written, nil
}

// StreamExportTickets streams tickets as CSV to the given writer.
// The caller is responsible for writing the BOM header before calling this.
func (s *ExportService) StreamExportTickets(ctx context.Context, w io.Writer, username string, filters TicketExportFilters) (int64, error) {
	csvW := csv.NewWriter(w)
	defer csvW.Flush()

	_ = csvW.Write([]string{
		"ID", "提交人", "提交人ID", "数据源ID", "数据库",
		"SQL内容", "SQL摘要", "数据库类型", "变更原因",
		"状态", "风险等级", "审批人", "审批意见",
		"定时执行时间", "实际执行时间", "创建时间", "更新时间",
	})

	rows, err := s.fetchTicketExportRows(ctx, filters)
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

		_ = csvW.Write([]string{
			fmt.Sprintf("%d", t.ID),
			t.SubmitterName,
			fmt.Sprintf("%d", t.SubmitterID),
			fmt.Sprintf("%d", t.DatasourceID),
			t.Database,
			escapeCSVFormula(t.SQLContent),
			escapeCSVFormula(t.SQLSummary),
			t.DBType,
			t.ChangeReason,
			t.Status,
			t.RiskLevel,
			t.ReviewerName,
			t.ReviewComment,
			scheduledAtStr,
			executedAtStr,
			t.CreatedAt.Format("2006-01-02 15:04:05"),
			t.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
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
func (s *ExportService) ExportAuditLogs(ctx context.Context, userID int64, username string, role string, filters AuditExportFilters) (*ExportResult, error) {
	if !s.hasExportPermission(role, ExportTypeAudit) {
		return nil, ErrExportNoPermission
	}

	total, err := s.countAuditLogs(ctx, filters)
	if err != nil {
		return nil, err
	}
	if total > ExportMaxRows {
		return nil, ErrExportExceedsLimit
	}

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM

	written, err := s.StreamExportAuditLogs(ctx, &buf, username, filters)
	if err != nil {
		return nil, err
	}

	// Append watermark
	fmt.Fprintf(&buf, "\n# 导出水印: 导出人=%s | 导出时间=%s | 仅限内部使用\n",
		username,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)

	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     userID,
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
func (s *ExportService) ExportTickets(ctx context.Context, userID int64, username string, role string, filters TicketExportFilters) (*ExportResult, error) {
	if !s.hasExportPermission(role, ExportTypeTicket) {
		return nil, ErrExportNoPermission
	}

	total, err := s.countTickets(ctx, filters)
	if err != nil {
		return nil, err
	}
	if total > ExportMaxRows {
		return nil, ErrExportExceedsLimit
	}

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM

	written, err := s.StreamExportTickets(ctx, &buf, username, filters)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(&buf, "\n# 导出水印: 导出人=%s | 导出时间=%s | 仅限内部使用\n",
		username,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)

	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     userID,
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
// The keyword also matches the acting user's name, which lives on a joined
// table that typed predicates cannot reach. It is resolved to a set of user ids
// first: one extra query, and the whole filter stays expressible without raw
// SQL. This mirrors what the audit domain's own search does.
func (s *ExportService) auditExportPredicates(ctx context.Context, filters AuditExportFilters) ([]predicate.AuditLog, error) {
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

// ticketExportPredicates translates the ticket export filters.
func ticketExportPredicates(filters TicketExportFilters) []predicate.Ticket {
	var preds []predicate.Ticket
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
	return preds
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
func (s *ExportService) fetchAuditExportRows(ctx context.Context, filters AuditExportFilters) ([]auditCSVRow, error) {
	preds, err := s.auditExportPredicates(ctx, filters)
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
func (s *ExportService) fetchTicketExportRows(ctx context.Context, filters TicketExportFilters) ([]ticketCSVRow, error) {
	var rows []ticketCSVRow
	err := s.client.Ticket.Query().
		Where(ticketExportPredicates(filters)...).
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
