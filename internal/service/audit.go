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
// Supported filters: userID, action, datasourceID, start/end (time), keyword (searches sql_content, username, action).
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
		keywordLike := "%" + escapeLike(keyword) + "%"
		filters = append(filters, FilterClause{
			Condition: "(a.sql_content LIKE ? ESCAPE '\\' OR u.username LIKE ? ESCAPE '\\' OR a.action LIKE ? ESCAPE '\\')",
			Args:      []interface{}{keywordLike, keywordLike, keywordLike},
		})
	}

	whereClause, args := BuildWhereClause(filters)

	// Count total. When keyword filter references u.username, we need the JOIN.
	var total int64
	countTable := "audit_logs a"
	if keyword != "" {
		countTable = "audit_logs a LEFT JOIN users u ON a.user_id = u.id"
	}
	countSQL := PaginatedCountSQL(countTable, whereClause)
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

// SearchParams holds the parameters for full-text search on audit logs.
type SearchParams struct {
	Keyword  string // required: search query
	Page     int
	PageSize int
	UserID   string
	Action   string
	Start    string // time range start (inclusive)
	End      string // time range end (inclusive)
}

// SearchResult wraps the FTS5 search results with pagination.
type SearchResult struct {
	Logs  []model.AuditLogSearch
	Total int64
}

// RebuildFTS rebuilds the FTS5 index by clearing and re-populating from audit_logs.
func (s *AuditService) RebuildFTS(ctx context.Context) error {
	// Clear existing FTS data.
	_, err := s.db.ExecContext(ctx, `DELETE FROM audit_logs_fts`)
	if err != nil {
		return fmt.Errorf("clear FTS index: %w", err)
	}

	// Re-populate from audit_logs.
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO audit_logs_fts(audit_id, sql_content, sql_summary, action, error_message, database)
		 SELECT id, sql_content, sql_summary, action, error_message, database FROM audit_logs`,
	)
	if err != nil {
		return fmt.Errorf("populate FTS index: %w", err)
	}
	return nil
}

// buildFTSSearchWhere constructs the WHERE clause for FTS5 search queries.
// The MATCH condition always comes first, followed by any filter conditions.
func buildFTSSearchWhere(matchQuery string, filterClauses []FilterClause) (string, []interface{}) {
	// Start with the MATCH condition.
	conds := []string{"audit_logs_fts MATCH ?"}
	args := []interface{}{matchQuery}

	// Append filter conditions.
	for _, f := range filterClauses {
		conds = append(conds, f.Condition)
		args = append(args, f.Args...)
	}

	return "WHERE " + strings.Join(conds, " AND "), args
}

// Search performs full-text search on audit logs using FTS5.
// Supports keyword, action filter, time range, and user_id filter.
// Returns results with snippet highlighting and relevance ranking.
func (s *AuditService) Search(ctx context.Context, params SearchParams) (*SearchResult, error) {
	if strings.TrimSpace(params.Keyword) == "" {
		return &SearchResult{Logs: []model.AuditLogSearch{}, Total: 0}, nil
	}

	p := ParsePagination(params.Page, params.PageSize)

	// Escape the keyword as a phrase for FTS5.
	ftsQuery := escapeFTS5(params.Keyword)

	// Build filter clauses on the content table.
	var filters []FilterClause
	if params.UserID != "" {
		filters = append(filters, FilterClause{Condition: "a.user_id = ?", Args: []interface{}{params.UserID}})
	}
	if params.Action != "" {
		filters = append(filters, FilterClause{Condition: "a.action = ?", Args: []interface{}{params.Action}})
	}
	if params.Start != "" {
		filters = append(filters, FilterClause{Condition: "a.created_at >= ?", Args: []interface{}{params.Start}})
	}
	if params.End != "" {
		filters = append(filters, FilterClause{Condition: "a.created_at <= ?", Args: []interface{}{params.End}})
	}

	whereClause, allArgs := buildFTSSearchWhere(ftsQuery, filters)

	// Count total matches.
	countSQL := fmt.Sprintf(
		`SELECT COUNT(*) FROM audit_logs_fts JOIN audit_logs a ON audit_logs_fts.audit_id = a.id %s`,
		whereClause,
	)
	var total int64
	if err := s.db.QueryRowContext(ctx, countSQL, allArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("FTS search count: %w", err)
	}

	// Search with snippet highlighting and ranking.
	// FTS5 column indices: 0=audit_id, 1=sql_content, 2=sql_summary, 3=action, 4=error_message, 5=database
	querySQL := fmt.Sprintf(
		`SELECT a.id, a.user_id, a.action, a.datasource_id, a.database, a.sql_content, a.sql_summary,
		        a.result_rows, a.affected_rows, a.execution_time_ms, a.error_message,
		        a.desensitized_fields, a.ip_address, a.created_at,
		        COALESCE(u.username, '') AS username,
		        snippet(audit_logs_fts, 1, '<mark>', '</mark>', '...', 32) AS hl_sql_content,
		        snippet(audit_logs_fts, 2, '<mark>', '</mark>', '...', 32) AS hl_sql_summary,
		        rank
		 FROM audit_logs_fts
		 JOIN audit_logs a ON audit_logs_fts.audit_id = a.id
		 LEFT JOIN users u ON a.user_id = u.id
		 %s
		 ORDER BY rank
		 LIMIT ? OFFSET ?`,
		whereClause,
	)
	queryArgs := append(allArgs, p.PageSize, p.Offset)

	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("FTS search query: %w", err)
	}
	defer rows.Close()

	logs := make([]model.AuditLogSearch, 0)
	for rows.Next() {
		var a model.AuditLogSearch
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Action, &a.DatasourceID, &a.Database,
			&a.SQLContent, &a.SQLSummary,
			&a.ResultRows, &a.AffectedRows, &a.ExecutionTimeMs,
			&a.ErrorMessage, &a.DesensitizedFields, &a.IPAddress, &a.CreatedAt,
			&a.Username,
			&a.HighlightSQLContent, &a.HighlightSQLSummary, &a.Rank,
		); err != nil {
			continue
		}
		logs = append(logs, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FTS search rows: %w", err)
	}

	return &SearchResult{Logs: logs, Total: total}, nil
}

// escapeFTS5 wraps the search string in double-quotes to treat it as a phrase,
// and escapes any embedded double-quotes.
func escapeFTS5(s string) string {
	s = strings.ReplaceAll(s, `"`, `""`)
	return `"` + s + `"`
}

// escapeLike escapes special LIKE wildcard characters (%, _, \) in a string.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
