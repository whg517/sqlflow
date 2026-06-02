package service

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
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
	db       *sql.DB
	auditSvc *AuditService
}

// NewExportService creates a new ExportService.
func NewExportService(db *sql.DB, auditSvc *AuditService) *ExportService {
	return &ExportService{db: db, auditSvc: auditSvc}
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
	filterClauses := buildAuditFilterClauses(filters)
	whereClause, args := BuildWhereClause(filterClauses)

	countTable := "audit_logs a"
	if filters.Keyword != "" {
		countTable = "audit_logs a LEFT JOIN users u ON a.user_id = u.id"
	}
	countSQL := PaginatedCountSQL(countTable, whereClause)

	var total int64
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("统计审计日志失败: %w", err)
	}
	return total, nil
}

// countTickets counts tickets matching the filters.
func (s *ExportService) countTickets(ctx context.Context, filters TicketExportFilters) (int64, error) {
	filterClauses := buildTicketFilterClauses(filters)
	whereClause, args := BuildWhereClause(filterClauses)

	var total int64
	countSQL := PaginatedCountSQL("tickets", whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("统计工单失败: %w", err)
	}
	return total, nil
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

	filterClauses := buildAuditFilterClauses(filters)
	whereClause, args := BuildWhereClause(filterClauses)

	// Write CSV header
	_ = csvW.Write([]string{
		"ID", "时间", "用户", "操作", "数据源ID", "数据库",
		"SQL内容", "SQL摘要", "返回行数", "影响行数", "耗时(ms)",
		"错误信息", "脱敏字段", "IP地址", "AI评审", "工单ID",
	})

	querySQL := fmt.Sprintf(
		`SELECT a.id, a.user_id, a.action, a.datasource_id, a.database, a.sql_content, a.sql_summary,
		        a.result_rows, a.affected_rows, a.execution_time_ms, a.error_message,
		        a.desensitized_fields, a.ip_address, a.ai_review_result, a.ticket_id, a.created_at,
		        COALESCE(u.username, '') AS username
		 FROM audit_logs a
		 LEFT JOIN users u ON a.user_id = u.id
		 %s ORDER BY a.created_at DESC LIMIT ?`,
		whereClause,
	)
	queryArgs := append(args, ExportMaxRows)

	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return 0, fmt.Errorf("查询审计日志失败: %w", err)
	}
	defer rows.Close()

	var written int64
	for rows.Next() {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		var a auditCSVRow
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Action, &a.DatasourceID, &a.Database,
			&a.SQLContent, &a.SQLSummary,
			&a.ResultRows, &a.AffectedRows, &a.ExecutionTimeMs,
			&a.ErrorMessage, &a.DesensitizedFields, &a.IPAddress,
			&a.AIReviewResult, &a.TicketID, &a.CreatedAt,
			&a.Username,
		); err != nil {
			continue
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

	if err := rows.Err(); err != nil {
		return written, fmt.Errorf("iterate audit rows: %w", err)
	}

	return written, nil
}

// StreamExportTickets streams tickets as CSV to the given writer.
// The caller is responsible for writing the BOM header before calling this.
func (s *ExportService) StreamExportTickets(ctx context.Context, w io.Writer, username string, filters TicketExportFilters) (int64, error) {
	csvW := csv.NewWriter(w)
	defer csvW.Flush()

	filterClauses := buildTicketFilterClauses(filters)
	whereClause, args := BuildWhereClause(filterClauses)

	_ = csvW.Write([]string{
		"ID", "提交人", "提交人ID", "数据源ID", "数据库",
		"SQL内容", "SQL摘要", "数据库类型", "变更原因",
		"状态", "风险等级", "审批人", "审批意见",
		"定时执行时间", "实际执行时间", "创建时间", "更新时间",
	})

	querySQL := fmt.Sprintf(
		`SELECT t.id, t.submitter_id, COALESCE(su.username, '') AS submitter_name,
		        t.datasource_id, t.database, t.sql_content, t.sql_summary,
		        t.db_type, t.change_reason, t.status, t.risk_level,
		        t.reviewer_id, COALESCE(rev.username, '') AS reviewer_name,
		        t.review_comment, t.scheduled_at, t.executed_at,
		        t.created_at, t.updated_at
		 FROM tickets t
		 LEFT JOIN users su ON t.submitter_id = su.id
		 LEFT JOIN users rev ON t.reviewer_id = rev.id
		 %s ORDER BY t.created_at DESC LIMIT ?`,
		whereClause,
	)
	queryArgs := append(args, ExportMaxRows)

	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return 0, fmt.Errorf("查询工单失败: %w", err)
	}
	defer rows.Close()

	var written int64
	for rows.Next() {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		var t ticketCSVRow
		if err := rows.Scan(
			&t.ID, &t.SubmitterID, &t.SubmitterName,
			&t.DatasourceID, &t.Database,
			&t.SQLContent, &t.SQLSummary,
			&t.DBType, &t.ChangeReason,
			&t.Status, &t.RiskLevel,
			&t.ReviewerID, &t.ReviewerName,
			&t.ReviewComment,
			&t.ScheduledAt, &t.ExecutedAt,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			continue
		}

		scheduledAtStr := ""
		if t.ScheduledAt.Valid {
			scheduledAtStr = t.ScheduledAt.Time.Format("2006-01-02 15:04:05")
		}
		executedAtStr := ""
		if t.ExecutedAt.Valid {
			executedAtStr = t.ExecutedAt.Time.Format("2006-01-02 15:04:05")
		}

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

	if err := rows.Err(); err != nil {
		return written, fmt.Errorf("iterate ticket rows: %w", err)
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

	s.auditSvc.Write(ctx, AuditRecord{
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

	s.auditSvc.Write(ctx, AuditRecord{
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

func buildAuditFilterClauses(filters AuditExportFilters) []FilterClause {
	var filterClauses []FilterClause
	if filters.UserID != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.user_id = ?", Args: []interface{}{filters.UserID}})
	}
	if filters.Action != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.action = ?", Args: []interface{}{filters.Action}})
	}
	if filters.DatasourceID != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.datasource_id = ?", Args: []interface{}{filters.DatasourceID}})
	}
	if filters.Start != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.created_at >= ?", Args: []interface{}{filters.Start}})
	}
	if filters.End != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.created_at <= ?", Args: []interface{}{filters.End}})
	}
	if filters.Keyword != "" {
		keywordLike := "%" + escapeLike(filters.Keyword) + "%"
		filterClauses = append(filterClauses, FilterClause{
			Condition: "(a.sql_content LIKE ? ESCAPE '\\' OR a.sql_summary LIKE ? ESCAPE '\\' OR u.username LIKE ? ESCAPE '\\' OR a.action LIKE ? ESCAPE '\\' OR a.error_message LIKE ? ESCAPE '\\' OR a.database LIKE ? ESCAPE '\\' OR a.ip_address LIKE ? ESCAPE '\\')",
			Args:      []interface{}{keywordLike, keywordLike, keywordLike, keywordLike, keywordLike, keywordLike, keywordLike},
		})
	}
	return filterClauses
}

func buildTicketFilterClauses(filters TicketExportFilters) []FilterClause {
	var filterClauses []FilterClause
	if filters.Status != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "status = ?", Args: []interface{}{filters.Status}})
	}
	if filters.DatasourceID != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "datasource_id = ?", Args: []interface{}{filters.DatasourceID}})
	}
	if filters.RiskLevel != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "risk_level = ?", Args: []interface{}{filters.RiskLevel}})
	}
	if filters.Keyword != "" {
		like := "%" + escapeLike(filters.Keyword) + "%"
		filterClauses = append(filterClauses, FilterClause{Condition: "(sql_content LIKE ? OR change_reason LIKE ? OR sql_summary LIKE ?)", Args: []interface{}{like, like, like}})
	}
	return filterClauses
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
type auditCSVRow struct {
	model.AuditLog
}

// ticketCSVRow holds the scan target for a single ticket row.
type ticketCSVRow struct {
	ID            int64
	SubmitterID   int64
	SubmitterName string
	DatasourceID  int64
	Database      string
	SQLContent    string
	SQLSummary    string
	DBType        string
	ChangeReason  string
	Status        string
	RiskLevel     string
	ReviewerID    int64
	ReviewerName  string
	ReviewComment string
	ScheduledAt   sql.NullTime
	ExecutedAt    sql.NullTime
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// WriteAuditExportLog records an audit log entry for an audit export.
func (s *ExportService) WriteAuditExportLog(ctx context.Context, userID int64, rows int64) {
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     userID,
		Action:     "audit_export",
		ResultRows: rows,
	})
}

// WriteTicketExportLog records an audit log entry for a ticket export.
func (s *ExportService) WriteTicketExportLog(ctx context.Context, userID int64, rows int64) {
	s.auditSvc.Write(ctx, AuditRecord{
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
