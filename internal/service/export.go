package service

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

const (
	// ExportMaxRows is the maximum number of rows allowed in a single export.
	ExportMaxRows = 10000
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
	ExportTypeAudit ExportType = "audit"
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

// ExportPermissionChecker checks if a user has export permission.
// Export permission requires: viewing permission (admin/dba for audit, authenticated for ticket) + independent export permission.
func (s *ExportService) hasExportPermission(role string, exportType ExportType) bool {
	switch exportType {
	case ExportTypeAudit:
		// Only admin and dba can export audit logs
		return role == "admin" || role == "dba"
	case ExportTypeTicket:
		// All authenticated users can export tickets
		return true
	default:
		return false
	}
}

// ExportAuditLogs exports audit logs as CSV with the given filters.
// The export is synchronous, limited to ExportMaxRows rows.
func (s *ExportService) ExportAuditLogs(ctx context.Context, userID int64, username string, role string, filters AuditExportFilters) (*ExportResult, error) {
	if !s.hasExportPermission(role, ExportTypeAudit) {
		return nil, ErrExportNoPermission
	}

	// Build filter clauses
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

	whereClause, args := BuildWhereClause(filterClauses)

	// Check total count first
	countTable := "audit_logs a"
	if filters.Keyword != "" {
		countTable = "audit_logs a LEFT JOIN users u ON a.user_id = u.id"
	}
	countSQL := PaginatedCountSQL(countTable, whereClause)

	var total int64
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("统计审计日志失败: %w", err)
	}

	if total > ExportMaxRows {
		return nil, ErrExportExceedsLimit
	}

	// Query all matching rows
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
		return nil, fmt.Errorf("查询审计日志失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Build CSV
	csvContent := buildAuditCSV(ctx, rows, username)

	// Write audit log for the export action
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     userID,
		Action:     "audit_export",
		ResultRows: int64(len(strings.Split(csvContent, "\n"))) - 2, // subtract header + watermark
	})

	return &ExportResult{
		Filename:    fmt.Sprintf("audit_logs_%s.csv", time.Now().Format("2006-01-02")),
		ContentType: "text/csv; charset=utf-8",
		TotalRows:   total,
		GeneratedAt: time.Now(),
		CSVBytes:    addBOM(csvContent),
	}, nil
}

// ExportTickets exports tickets as CSV with the given filters.
// The export is synchronous, limited to ExportMaxRows rows.
func (s *ExportService) ExportTickets(ctx context.Context, userID int64, username string, role string, filters TicketExportFilters) (*ExportResult, error) {
	if !s.hasExportPermission(role, ExportTypeTicket) {
		return nil, ErrExportNoPermission
	}

	// Build filter clauses
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

	whereClause, args := BuildWhereClause(filterClauses)

	// Check total count
	var total int64
	countSQL := PaginatedCountSQL("tickets", whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("统计工单失败: %w", err)
	}

	if total > ExportMaxRows {
		return nil, ErrExportExceedsLimit
	}

	// Query all matching rows
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
		return nil, fmt.Errorf("查询工单失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Build CSV
	csvContent := buildTicketCSV(ctx, rows, username)

	// Write audit log for the export action
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     userID,
		Action:     "ticket_export",
		ResultRows: int64(len(strings.Split(csvContent, "\n"))) - 2,
	})

	return &ExportResult{
		Filename:    fmt.Sprintf("tickets_%s.csv", time.Now().Format("2006-01-02")),
		ContentType: "text/csv; charset=utf-8",
		TotalRows:   total,
		GeneratedAt: time.Now(),
		CSVBytes:    addBOM(csvContent),
	}, nil
}

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
	ID             int64
	SubmitterID    int64
	SubmitterName  string
	DatasourceID   int64
	Database       string
	SQLContent     string
	SQLSummary     string
	DBType         string
	ChangeReason   string
	Status         string
	RiskLevel      string
	ReviewerID     int64
	ReviewerName   string
	ReviewComment  string
	ScheduledAt    sql.NullTime
	ExecutedAt     sql.NullTime
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// buildAuditCSV generates CSV content from audit log rows with watermark.
func buildAuditCSV(ctx context.Context, rows *sql.Rows, username string) string {
	var buf strings.Builder
	w := csv.NewWriter(&buf)

	// Write header
	_ = w.Write([]string{
		"ID", "时间", "用户", "操作", "数据源ID", "数据库",
		"SQL内容", "SQL摘要", "返回行数", "影响行数", "耗时(ms)",
		"错误信息", "脱敏字段", "IP地址", "AI评审", "工单ID",
	})

	for rows.Next() {
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

		scheduledAtStr := ""
		if !a.CreatedAt.IsZero() {
			scheduledAtStr = a.CreatedAt.Format("2006-01-02 15:04:05")
		}

		_ = w.Write([]string{
			fmt.Sprintf("%d", a.ID),
			scheduledAtStr,
			a.Username,
			a.Action,
			fmt.Sprintf("%d", a.DatasourceID),
			a.Database,
			a.SQLContent,
			a.SQLSummary,
			fmt.Sprintf("%d", a.ResultRows),
			fmt.Sprintf("%d", a.AffectedRows),
			fmt.Sprintf("%d", a.ExecutionTimeMs),
			a.ErrorMessage,
			a.DesensitizedFields,
			a.IPAddress,
			a.AIReviewResult,
			fmt.Sprintf("%d", a.TicketID),
		})
	}
	_ = rows.Err()
	w.Flush()

	// Append watermark row
	buf.WriteString(fmt.Sprintf("\n# 导出水印: 导出人=%s | 导出时间=%s | 仅限内部使用\n",
		username,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	))

	return buf.String()
}

// buildTicketCSV generates CSV content from ticket rows with watermark.
func buildTicketCSV(ctx context.Context, rows *sql.Rows, username string) string {
	var buf strings.Builder
	w := csv.NewWriter(&buf)

	// Write header
	_ = w.Write([]string{
		"ID", "提交人", "提交人ID", "数据源ID", "数据库",
		"SQL内容", "SQL摘要", "数据库类型", "变更原因",
		"状态", "风险等级", "审批人", "审批意见",
		"定时执行时间", "实际执行时间", "创建时间", "更新时间",
	})

	for rows.Next() {
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

		_ = w.Write([]string{
			fmt.Sprintf("%d", t.ID),
			t.SubmitterName,
			fmt.Sprintf("%d", t.SubmitterID),
			fmt.Sprintf("%d", t.DatasourceID),
			t.Database,
			t.SQLContent,
			t.SQLSummary,
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
	}
	_ = rows.Err()
	w.Flush()

	// Append watermark row
	buf.WriteString(fmt.Sprintf("\n# 导出水印: 导出人=%s | 导出时间=%s | 仅限内部使用\n",
		username,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	))

	return buf.String()
}

// addBOM prepends a UTF-8 BOM to the CSV content for Excel compatibility.
func addBOM(content string) []byte {
	bom := []byte{0xEF, 0xBB, 0xBF}
	return append(bom, content...)
}
