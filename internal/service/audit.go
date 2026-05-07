package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// AuditService handles audit logging with synchronous writing and paginated queries.
type AuditService struct {
	db *sql.DB
}

// NewAuditService creates a new AuditService.
// The batchSize and flushInterval parameters are accepted for interface compatibility but are no longer used.
func NewAuditService(db *sql.DB, batchSize int, flushInterval time.Duration) *AuditService {
	return &AuditService{db: db}
}

// Write inserts an audit record directly into the database.
// If the receiver is nil or the insert fails, an error is logged but not returned.
func (s *AuditService) Write(ctx context.Context, rec AuditRecord) {
	if s == nil {
		return
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, result_rows, affected_rows, execution_time_ms, error_message, desensitized_fields, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.UserID, rec.Action, rec.DatasourceID, rec.Database,
		rec.SQLContent, rec.SQLSummary, rec.ResultRows, rec.AffectedRows,
		rec.ExecutionTimeMs, rec.ErrorMessage, rec.DesensitizedFields, rec.IPAddress,
	)
	if err != nil {
		log.Printf("audit write: insert: %v", err)
	}
}

// Close is a no-op kept for interface compatibility.
func (s *AuditService) Close() {}

// List retrieves a paginated list of audit logs with filtering.
// Supported filters: userID, action, datasourceID, start/end (time), keyword (searches sql_content).
func (s *AuditService) List(ctx context.Context, page, pageSize int, userID, action, datasourceID, start, end, keyword string) ([]model.AuditLog, int64, error) {
	p := ParsePagination(page, pageSize)

	var filters []FilterClause
	if userID != "" {
		filters = append(filters, FilterClause{Condition: "a.user_id = ?", Args: []interface{}{userID}})
	}
	if action != "" {
		filters = append(filters, FilterClause{Condition: "a.action = ?", Args: []interface{}{action}})
	}
	if datasourceID != "" {
		filters = append(filters, FilterClause{Condition: "a.datasource_id = ?", Args: []interface{}{datasourceID}})
	}
	if start != "" {
		filters = append(filters, FilterClause{Condition: "a.created_at >= ?", Args: []interface{}{start}})
	}
	if end != "" {
		filters = append(filters, FilterClause{Condition: "a.created_at <= ?", Args: []interface{}{end}})
	}
	if keyword != "" {
		filters = append(filters, FilterClause{Condition: "a.sql_content LIKE ? ESCAPE '\\'", Args: []interface{}{"%" + escapeLike(keyword) + "%"}})
	}

	whereClause, args := BuildWhereClause(filters)

	// Count total.
	var total int64
	countSQL := PaginatedCountSQL("audit_logs a", whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计审计日志失败: %w", err)
	}

	// Query page.
	querySQL := fmt.Sprintf(
		`SELECT a.id, a.user_id, a.action, a.datasource_id, a.database, a.sql_content, a.sql_summary,
		        a.result_rows, a.affected_rows, a.execution_time_ms, a.error_message,
		        a.desensitized_fields, a.ip_address, a.created_at,
		        COALESCE(u.username, '') AS username
		 FROM audit_logs a
		 LEFT JOIN users u ON a.user_id = u.id
		 %s ORDER BY a.created_at DESC LIMIT ? OFFSET ?`,
		whereClause,
	)
	queryArgs := AppendLimitArgs(args, p)

	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询审计日志失败: %w", err)
	}
	defer rows.Close()

	logs := make([]model.AuditLog, 0)
	for rows.Next() {
		var a model.AuditLog
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Action, &a.DatasourceID, &a.Database,
			&a.SQLContent, &a.SQLSummary,
			&a.ResultRows, &a.AffectedRows, &a.ExecutionTimeMs,
			&a.ErrorMessage, &a.DesensitizedFields, &a.IPAddress, &a.CreatedAt,
			&a.Username,
		); err != nil {
			continue
		}
		logs = append(logs, a)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历审计日志失败: %w", err)
	}

	return logs, total, nil
}

// escapeLike escapes special LIKE wildcard characters (%, _, \) in a string.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
