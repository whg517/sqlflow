package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

const maxHistoryPerUser = 200

// QueryHistoryService handles query history logic.
type QueryHistoryService struct {
	db *sql.DB
}

// NewQueryHistoryService creates a new QueryHistoryService.
func NewQueryHistoryService(db *sql.DB) *QueryHistoryService {
	return &QueryHistoryService{db: db}
}

// CreateHistory inserts a new query history record and auto-cleans old records.
func (s *QueryHistoryService) CreateHistory(ctx context.Context, h *model.QueryHistory) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO query_history (user_id, datasource_id, database, sql_content, sql_summary, db_type, execution_time, result_rows, affected_rows)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.UserID, h.DatasourceID, h.Database, h.SQLContent, h.SQLSummary,
		h.DBType, h.ExecutionTime, h.ResultRows, h.AffectedRows,
	)
	if err != nil {
		return fmt.Errorf("insert query history: %w", err)
	}

	// Auto-cleanup: keep only the latest 200 records per user
	go s.cleanupOldRecords(h.UserID)

	return nil
}

// ListHistory returns paginated query history for a user.
func (s *QueryHistoryService) ListHistory(ctx context.Context, userID int64, page, pageSize int) ([]model.QueryHistory, int, error) {
	p := ParsePagination(page, pageSize)

	filters := []FilterClause{
		{Condition: "user_id = ?", Args: []interface{}{userID}},
	}
	whereClause, args := BuildWhereClause(filters)

	var total int
	countSQL := PaginatedCountSQL("query_history", whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count query history: %w", err)
	}

	querySQL := PaginatedQuerySQL(
		"SELECT id, user_id, datasource_id, database, sql_content, sql_summary, db_type, execution_time, result_rows, affected_rows, created_at",
		"query_history", whereClause, "id DESC", p,
	)
	queryArgs := AppendLimitArgs(args, p)
	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

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
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM query_history WHERE id = ? AND user_id = ?`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete query history: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("记录不存在或无权删除")
	}
	return nil
}

// ClearHistory deletes all query history for a user.
func (s *QueryHistoryService) ClearHistory(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM query_history WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("clear query history: %w", err)
	}
	return nil
}

// cleanupOldRecords removes records exceeding the per-user limit.
func (s *QueryHistoryService) cleanupOldRecords(userID int64) {
	s.db.Exec(
		`DELETE FROM query_history WHERE user_id = ? AND id NOT IN (
			SELECT id FROM query_history WHERE user_id = ? ORDER BY id DESC LIMIT ?
		)`,
		userID, userID, maxHistoryPerUser,
	)
}
