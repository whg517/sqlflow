package audit

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entauditlog "github.com/whg517/sqlflow/internal/db/ent/auditlog"
	"github.com/whg517/sqlflow/internal/db/ent/predicate"
	entuser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"
)

// Service handles audit logging with synchronous writing and paginated queries.
type Service struct {
	database *db.DB
	client   *ent.Client
}

// NewService creates a new Service.
// The batchSize and flushInterval parameters are accepted for interface compatibility but are no longer used.
func NewService(database *db.DB, batchSize int, flushInterval time.Duration) *Service {
	return &Service{database: database, client: database.Client()}
}

// Write inserts an audit record directly into the database.
// If the receiver is nil or the insert fails, an error is logged but not returned.
// Write 写入审计日志。
//
// 使用独立的 context（不受调用方 context 超时/取消影响），确保即使触发审计日志的
// 操作本身已超时（如查询超时），审计日志仍能可靠写入。
// 这对系统"所有操作必须留痕"的核心设计至关重要——否则超时操作的审计记录会丢失。
func (s *Service) Write(ctx context.Context, rec auditlog.Record) {
	if s == nil {
		return
	}
	// 独立 context：不继承调用方的超时/取消，但设置 10s 上限防止无限阻塞
	auditCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.client.AuditLog.Create().
		SetUserID(rec.UserID).
		SetAction(rec.Action).
		SetDatasourceID(rec.DatasourceID).
		SetDatabase(rec.Database).
		SetSQLContent(rec.SQLContent).
		SetSQLSummary(rec.SQLSummary).
		SetResultRows(rec.ResultRows).
		SetAffectedRows(rec.AffectedRows).
		SetExecutionTimeMs(rec.ExecutionTimeMs).
		SetErrorMessage(rec.ErrorMessage).
		SetDesensitizedFields(rec.DesensitizedFields).
		SetIPAddress(rec.IPAddress).
		SetAiReviewResult(rec.AIReviewResult).
		SetTicketID(rec.TicketID).
		Save(auditCtx)
	if err != nil {
		log.Printf("audit write: insert: %v", err)
	}
}

// Close is a no-op kept for interface compatibility.
func (s *Service) Close() {}

// List retrieves a paginated list of audit logs with filtering.
//
// The username column comes from a LEFT JOIN: audit_logs.user_id is a plain
// column with no ent edge, because an audit entry must survive the deletion of
// the user it refers to. The join therefore goes through Modify rather than an
// edge traversal.
func (s *Service) List(ctx context.Context, page, pageSize int, userID, action, datasourceID, start, end, keyword string) ([]model.AuditLog, int64, error) {
	p := sqlutil.ParsePagination(page, pageSize)

	q := s.client.AuditLog.Query()

	// Filters arrive as strings from the query string. Parsing them here rather
	// than passing them straight into a comparison is not just tidier: comparing
	// a bigint column against text is an error in PostgreSQL, and only worked
	// before because SQLite applies dynamic typing.
	if id, err := strconv.ParseInt(userID, 10, 64); err == nil && userID != "" {
		q = q.Where(entauditlog.UserIDEQ(id))
	}
	if action != "" {
		q = q.Where(entauditlog.ActionEQ(action))
	}
	if id, err := strconv.ParseInt(datasourceID, 10, 64); err == nil && datasourceID != "" {
		q = q.Where(entauditlog.DatasourceIDEQ(id))
	}
	if t, ok := parseFilterTime(start); ok {
		q = q.Where(entauditlog.CreatedAtGTE(t))
	}
	if t, ok := parseFilterTime(end); ok {
		q = q.Where(entauditlog.CreatedAtLTE(t))
	}

	if keyword != "" {
		match, err := s.keywordFilter(ctx, keyword)
		if err != nil {
			return nil, 0, err
		}
		q = q.Where(match)
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("统计审计日志失败: %w", err)
	}

	// Modify is used only to attach the username column; every filter above is
	// already expressed in typed predicates.
	var rows []auditLogRow
	err = q.Modify(func(sel *entsql.Selector) {
		u := joinAuditUser(sel)
		selectAuditColumns(sel, u)
		sel.OrderBy(entsql.Desc(sel.C(entauditlog.FieldCreatedAt))).
			Limit(p.PageSize).
			Offset(p.Offset)
	}).Scan(ctx, &rows)
	if err != nil {
		return nil, 0, fmt.Errorf("查询审计日志失败: %w", err)
	}

	logs := make([]model.AuditLog, 0, len(rows))
	for _, r := range rows {
		logs = append(logs, r.toModel())
	}
	return logs, int64(total), nil
}

// joinAuditUser attaches the users table so username is available.
// It returns the joined table so callers can build properly qualified column
// references with u.C("username"); passing the string "u.username" instead makes
// the builder quote it whole, and PostgreSQL then looks for a column literally
// named `u.username`.
func joinAuditUser(sel *entsql.Selector) *entsql.SelectTable {
	u := entsql.Table("users").As("u")
	sel.LeftJoin(u).On(sel.C(entauditlog.FieldUserID), u.C("id"))
	return u
}

// keywordFilter builds the case-insensitive substring filter across every
// searchable audit column.
//
// ContainsFold rather than a hand-built LIKE: ent renders it as ILIKE on
// PostgreSQL and escapes the pattern itself, so neither the wildcard escaping
// nor the ESCAPE clause the SQLite version carried is needed here.
//
// Username lives on the joined users table, which typed predicates cannot
// reach, so it is resolved to a set of user IDs first. That costs one extra
// query and keeps the whole filter expressible without raw SQL.
func (s *Service) keywordFilter(ctx context.Context, keyword string) (predicate.AuditLog, error) {
	userIDs, err := s.client.User.Query().
		Where(entuser.UsernameContainsFold(keyword)).
		IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("解析用户名关键字失败: %w", err)
	}

	matches := []predicate.AuditLog{
		entauditlog.SQLContentContainsFold(keyword),
		entauditlog.SQLSummaryContainsFold(keyword),
		entauditlog.ActionContainsFold(keyword),
		entauditlog.ErrorMessageContainsFold(keyword),
		entauditlog.DatabaseContainsFold(keyword),
		entauditlog.IPAddressContainsFold(keyword),
	}
	if len(userIDs) > 0 {
		ids := make([]int64, 0, len(userIDs))
		for _, id := range userIDs {
			ids = append(ids, int64(id))
		}
		matches = append(matches, entauditlog.UserIDIn(ids...))
	}
	return entauditlog.Or(matches...), nil
}

// auditLogRow carries one joined row. ent scans by json tag.
type auditLogRow struct {
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	Action          string    `json:"action"`
	DatasourceID    int64     `json:"datasource_id"`
	Database        string    `json:"database"`
	SQLContent      string    `json:"sql_content"`
	SQLSummary      string    `json:"sql_summary"`
	ResultRows      int64     `json:"result_rows"`
	AffectedRows    int64     `json:"affected_rows"`
	ExecutionTimeMs int64     `json:"execution_time_ms"`
	ErrorMessage    string    `json:"error_message"`
	Desensitized    string    `json:"desensitized_fields"`
	IPAddress       string    `json:"ip_address"`
	AIReviewResult  string    `json:"ai_review_result"`
	TicketID        int64     `json:"ticket_id"`
	CreatedAt       time.Time `json:"created_at"`
	Username        string    `json:"username"`
}

func (r auditLogRow) toModel() model.AuditLog {
	return model.AuditLog{
		ID: r.ID, UserID: r.UserID, Action: r.Action,
		DatasourceID: r.DatasourceID, Database: r.Database,
		SQLContent: r.SQLContent, SQLSummary: r.SQLSummary,
		ResultRows: r.ResultRows, AffectedRows: r.AffectedRows,
		ExecutionTimeMs: r.ExecutionTimeMs, ErrorMessage: r.ErrorMessage,
		DesensitizedFields: r.Desensitized, IPAddress: r.IPAddress,
		AIReviewResult: r.AIReviewResult, TicketID: r.TicketID,
		CreatedAt: r.CreatedAt, Username: r.Username,
	}
}

// selectAuditColumns projects every audit column plus the joined username.
func selectAuditColumns(sel *entsql.Selector, u *entsql.SelectTable) {
	cols := make([]string, 0, len(entauditlog.Columns)+1)
	for _, c := range entauditlog.Columns {
		cols = append(cols, sel.C(c))
	}
	sel.Select(cols...).AppendSelect(entsql.As("COALESCE("+u.C("username")+", '')", "username"))
}

// parseFilterTime accepts the date and datetime shapes the UI sends.
func parseFilterTime(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
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
// RAW_SQL: FTS5 virtual table operations (DELETE + INSERT INTO ... SELECT) — not expressible via ent.
func (s *Service) RebuildFTS(ctx context.Context) error {
	// Clear existing FTS data.
	_, err := s.database.DB.ExecContext(ctx, `DELETE FROM audit_logs_fts`)
	if err != nil {
		return fmt.Errorf("clear FTS index: %w", err)
	}

	// Re-populate from audit_logs.
	_, err = s.database.DB.ExecContext(ctx,
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
func buildFTSSearchWhere(matchQuery string, filterClauses []sqlutil.FilterClause) (string, []interface{}) {
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
// RAW_SQL: FTS5 MATCH + JOIN + snippet highlighting — SQLite FTS5 specific, not expressible via ent.
func (s *Service) Search(ctx context.Context, params SearchParams) (*SearchResult, error) {
	if strings.TrimSpace(params.Keyword) == "" {
		return &SearchResult{Logs: []model.AuditLogSearch{}, Total: 0}, nil
	}

	p := sqlutil.ParsePagination(params.Page, params.PageSize)

	// Escape the keyword as a phrase for FTS5.
	ftsQuery := escapeFTS5(params.Keyword)

	// Build filter clauses on the content table.
	var filters []sqlutil.FilterClause
	if params.UserID != "" {
		filters = append(filters, sqlutil.FilterClause{Condition: "a.user_id = ?", Args: []interface{}{params.UserID}})
	}
	if params.Action != "" {
		filters = append(filters, sqlutil.FilterClause{Condition: "a.action = ?", Args: []interface{}{params.Action}})
	}
	if params.Start != "" {
		filters = append(filters, sqlutil.FilterClause{Condition: "a.created_at >= ?", Args: []interface{}{params.Start}})
	}
	if params.End != "" {
		filters = append(filters, sqlutil.FilterClause{Condition: "a.created_at <= ?", Args: []interface{}{params.End}})
	}

	whereClause, allArgs := buildFTSSearchWhere(ftsQuery, filters)

	// Count total matches.
	countSQL := fmt.Sprintf(
		`SELECT COUNT(*) FROM audit_logs_fts JOIN audit_logs a ON audit_logs_fts.audit_id = a.id %s`,
		whereClause,
	)
	var total int64
	if err := s.database.DB.QueryRowContext(ctx, countSQL, allArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("FTS search count: %w", err)
	}

	// Search with snippet highlighting and ranking.
	querySQL := fmt.Sprintf(
		`SELECT a.id, a.user_id, a.action, a.datasource_id, a.database, a.sql_content, a.sql_summary,
		        a.result_rows, a.affected_rows, a.execution_time_ms, a.error_message,
		        a.desensitized_fields, a.ip_address, a.ai_review_result, a.ticket_id, a.created_at,
		        COALESCE(u.username, '') AS username,
		        snippet(audit_logs_fts, 1, '<mark>', '</mark>', '...', 32) AS hl_sql_content,
		        snippet(audit_logs_fts, 2, '<mark>', '</mark>', '...', 32) AS hl_sql_summary,
		        rank
		 FROM audit_logs_fts
		 JOIN audit_logs a ON audit_logs_fts.audit_id = a.id
		 LEFT JOIN users u ON a.user_id = u.id
		 %s
		 ORDER BY rank
		 LIMIT $1 OFFSET $2`,
		whereClause,
	)
	queryArgs := append(allArgs, p.PageSize, p.Offset)

	rows, err := s.database.DB.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("FTS search query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	logs := make([]model.AuditLogSearch, 0)
	for rows.Next() {
		var a model.AuditLogSearch
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Action, &a.DatasourceID, &a.Database,
			&a.SQLContent, &a.SQLSummary,
			&a.ResultRows, &a.AffectedRows, &a.ExecutionTimeMs,
			&a.ErrorMessage, &a.DesensitizedFields, &a.IPAddress,
			&a.AIReviewResult, &a.TicketID, &a.CreatedAt,
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
