package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/db/ent/queryhistory"
	"github.com/whg517/sqlflow/internal/model"
)

const maxHistoryPerUser = 200

// QueryHistoryService handles query history logic.
type QueryHistoryService struct {
	database *db.DB
	client   *ent.Client
}

// NewQueryHistoryService creates a new QueryHistoryService.
func NewQueryHistoryService(database *db.DB) *QueryHistoryService {
	return &QueryHistoryService{database: database, client: database.Client()}
}

// CreateHistory inserts a new query history record and auto-cleans old records.
func (s *QueryHistoryService) CreateHistory(ctx context.Context, h *model.QueryHistory) (int64, error) {
	// Compute SQL hash for deduplication (normalized: trimmed + lowercase + collapsed whitespace)
	normalized := strings.TrimSpace(strings.ToLower(h.SQLContent))
	// Collapse multiple spaces into one for robust dedup
	for strings.Contains(normalized, "  ") {
		normalized = strings.ReplaceAll(normalized, "  ", " ")
	}
	hash := sha256.Sum256([]byte(normalized))
	sqlHash := fmt.Sprintf("%x", hash[:])

	saved, err := s.client.QueryHistory.Create().
		SetUserID(h.UserID).
		SetDatasourceID(h.DatasourceID).
		SetDatabase(h.Database).
		SetSQLContent(h.SQLContent).
		SetSQLHash(sqlHash).
		SetSQLSummary(h.SQLSummary).
		SetDbType(h.DBType).
		SetExecutionTime(h.ExecutionTime).
		SetResultRows(h.ResultRows).
		SetAffectedRows(h.AffectedRows).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("insert query history: %w", err)
	}

	h.ID = int64(saved.ID)

	// Auto-cleanup: keep only the latest 200 records per user
	go s.cleanupOldRecords(h.UserID)

	return h.ID, nil
}

// ListHistory returns paginated query history for a user.
// Uses raw SQL for the LIKE keyword search across multiple fields.
func (s *QueryHistoryService) ListHistory(ctx context.Context, userID int64, page, pageSize int, keyword string) ([]model.QueryHistory, int, error) {
	p := ParsePagination(page, pageSize)

	filters := []FilterClause{
		{Condition: "user_id = ?", Args: []interface{}{userID}},
	}
	if keyword != "" {
		keywordLike := "%" + escapeLike(keyword) + "%"
		filters = append(filters, FilterClause{
			Condition: "(sql_content LIKE ? ESCAPE '\\' OR sql_summary LIKE ? ESCAPE '\\')",
			Args:      []interface{}{keywordLike, keywordLike},
		})
	}
	whereClause, args := BuildWhereClause(filters)

	var total int
	countSQL := PaginatedCountSQL("query_history", whereClause)
	if err := s.database.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count query history: %w", err)
	}

	querySQL := PaginatedQuerySQL(
		"SELECT id, user_id, datasource_id, database, sql_content, sql_summary, db_type, execution_time, result_rows, affected_rows, created_at",
		"query_history", whereClause, "id DESC", p,
	)
	queryArgs := AppendLimitArgs(args, p)
	rows, err := s.database.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []model.QueryHistory
	for rows.Next() {
		var h model.QueryHistory
		var createdAt string
		if err := rows.Scan(&h.ID, &h.UserID, &h.DatasourceID, &h.Database,
			&h.SQLContent, &h.SQLSummary, &h.DBType, &h.ExecutionTime,
			&h.ResultRows, &h.AffectedRows, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("scan query history: %w", err)
		}
		h.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		list = append(list, h)
	}

	return list, total, rows.Err()
}

// DeleteHistory deletes a single query history record (only if it belongs to the user).
func (s *QueryHistoryService) DeleteHistory(ctx context.Context, id, userID int64) error {
	n, err := s.client.QueryHistory.Delete().
		Where(queryhistory.ID(int(id)), queryhistory.UserID(userID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete query history: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("记录不存在或无权删除")
	}
	return nil
}

// ClearHistory deletes all query history for a user.
func (s *QueryHistoryService) ClearHistory(ctx context.Context, userID int64) error {
	_, err := s.client.QueryHistory.Delete().
		Where(queryhistory.UserID(userID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("clear query history: %w", err)
	}
	return nil
}

// FrequentQuery represents a deduplicated frequent SQL template.
type FrequentQuery struct {
	SQLContent     string `json:"sql_content"`
	SQLHash        string `json:"sql_hash"`
	Snippet        string `json:"snippet"`
	ExecutionCount int    `json:"execution_count"`
	LastExecutedAt string `json:"last_executed_at"` // SQLite stores datetime as text
}

const (
	frequentQueryLimit = 20
	frequentQueryCacheTTL = 5 * time.Minute
)

// frequentQueryCache stores per-user frequent query results.
var (
	fqCache   = make(map[int64][]FrequentQuery)
	fqCacheAt = make(map[int64]time.Time)
	fqMu      sync.RWMutex
)

// GetFrequentQueries returns top 20 frequent SQL templates for the given user.
// Results are deduplicated by SQL hash and cached for 5 minutes.
func (s *QueryHistoryService) GetFrequentQueries(ctx context.Context, userID int64) ([]FrequentQuery, error) {
	// Check cache
	fqMu.RLock()
	if cached, ok := fqCache[userID]; ok && time.Since(fqCacheAt[userID]) < frequentQueryCacheTTL {
		fqMu.RUnlock()
		return cached, nil
	}
	fqMu.RUnlock()

	// Query DB: group by sql_hash, count executions, get latest
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT
			sql_hash,
			sql_content,
			COUNT(*) as execution_count,
			MAX(created_at) as last_executed_at
		FROM query_history
		WHERE user_id = ? AND sql_hash != ''
		GROUP BY sql_hash
		ORDER BY execution_count DESC
		LIMIT ?`,
		userID, frequentQueryLimit,
	)
	if err != nil {
		return nil, fmt.Errorf("query frequent templates: %w", err)
	}
	defer rows.Close()

	results := make([]FrequentQuery, 0, frequentQueryLimit)
	for rows.Next() {
		var fq FrequentQuery
		if err := rows.Scan(&fq.SQLHash, &fq.SQLContent, &fq.ExecutionCount, &fq.LastExecutedAt); err != nil {
			continue
		}
		// Snippet: first 80 chars of SQL
		snippet := fq.SQLContent
		if len(snippet) > 80 {
			snippet = snippet[:80]
		}
		fq.Snippet = snippet
		results = append(results, fq)
	}

	// Update cache
	fqMu.Lock()
	fqCache[userID] = results
	fqCacheAt[userID] = time.Now()
	fqMu.Unlock()

	return results, nil
}

// InvalidateFrequentQueryCache clears the frequent query cache (for testing).
func InvalidateFrequentQueryCache() {
	fqMu.Lock()
	fqCache = make(map[int64][]FrequentQuery)
	fqCacheAt = make(map[int64]time.Time)
	fqMu.Unlock()
}

// cleanupOldRecords removes records exceeding the per-user limit.
// Uses raw SQL for the subquery-based DELETE (ent doesn't support this pattern directly).
func (s *QueryHistoryService) cleanupOldRecords(userID int64) {
	_, _ = s.database.Exec(
		`DELETE FROM query_history WHERE user_id = ? AND id NOT IN (
			SELECT id FROM query_history WHERE user_id = ? ORDER BY id DESC LIMIT ?
		)`,
		userID, userID, maxHistoryPerUser,
	)
}
