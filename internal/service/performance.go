package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// SlowQueryParams holds parameters for listing slow queries.
type SlowQueryParams struct {
	Threshold    int64 // ms, default 1000
	Page         int
	PageSize     int
	DatasourceID int64
	StartDate    string // optional, format: 2006-01-02
	EndDate      string // optional, format: 2006-01-02
}

// DailyTrend represents per-day aggregated stats.
type DailyTrend struct {
	Date      string `json:"date"`
	Count     int    `json:"count"`
	AvgTime   int64  `json:"avg_time"`
	SlowCount int    `json:"slow_count"`
}

// DatasourceStats represents per-datasource aggregated stats.
type DatasourceStats struct {
	DatasourceID   int64  `json:"datasource_id"`
	DatasourceName string `json:"datasource_name"`
	Count          int    `json:"count"`
	AvgTime        int64  `json:"avg_time"`
}

// TopSlowQuery represents a top slow query entry.
type TopSlowQuery struct {
	ID             int64  `json:"id"`
	SQLSummary     string `json:"sql_summary"`
	ExecutionTime  int64  `json:"execution_time"`
	DatasourceName string `json:"datasource_name"`
	CreatedAt      string `json:"created_at"`
}

// PerformanceStats holds aggregated performance statistics.
type PerformanceStats struct {
	TotalQueries    int               `json:"total_queries"`
	SlowQueries     int               `json:"slow_queries"`
	AvgTime         int64             `json:"avg_time"`
	SlowQueryRate   float64           `json:"slow_query_rate"`
	DailyTrend      []DailyTrend      `json:"daily_trend"`
	DatasourceStats []DatasourceStats `json:"datasource_stats"`
	TopSlowQueries  []TopSlowQuery    `json:"top_slow_queries"`
}

// ListSlowQueries returns paginated slow queries with optional filters.
func (s *QueryHistoryService) ListSlowQueries(ctx context.Context, params SlowQueryParams) ([]model.QueryHistory, int, error) {
	p := ParsePagination(params.Page, params.PageSize)

	threshold := params.Threshold
	if threshold <= 0 {
		threshold = 1000
	}

	filters := []FilterClause{
		{Condition: "qh.execution_time >= ?", Args: []interface{}{threshold}},
	}
	if params.DatasourceID > 0 {
		filters = append(filters, FilterClause{
			Condition: "qh.datasource_id = ?", Args: []interface{}{params.DatasourceID},
		})
	}
	if params.StartDate != "" {
		filters = append(filters, FilterClause{
			Condition: "qh.created_at >= ?", Args: []interface{}{params.StartDate + " 00:00:00"},
		})
	}
	if params.EndDate != "" {
		filters = append(filters, FilterClause{
			Condition: "qh.created_at <= ?", Args: []interface{}{params.EndDate + " 23:59:59"},
		})
	}

	whereClause, args := BuildWhereClause(filters)

	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM query_history qh %s", whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count slow queries: %w", err)
	}

	querySQL := fmt.Sprintf(
		"SELECT qh.id, qh.user_id, qh.datasource_id, qh.database, qh.sql_content, qh.sql_summary, qh.db_type, qh.execution_time, qh.result_rows, qh.affected_rows, qh.created_at FROM query_history qh %s ORDER BY qh.execution_time DESC LIMIT ? OFFSET ?",
		whereClause,
	)
	queryArgs := AppendLimitArgs(args, p)

	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query slow queries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []model.QueryHistory
	for rows.Next() {
		var h model.QueryHistory
		var createdAt string
		if err := rows.Scan(&h.ID, &h.UserID, &h.DatasourceID, &h.Database,
			&h.SQLContent, &h.SQLSummary, &h.DBType, &h.ExecutionTime,
			&h.ResultRows, &h.AffectedRows, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("scan slow query: %w", err)
		}
		h.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		list = append(list, h)
	}

	return list, total, rows.Err()
}

// GetPerformanceStats returns aggregated performance statistics for the given number of days.
func (s *QueryHistoryService) GetPerformanceStats(ctx context.Context, days int) (*PerformanceStats, error) {
	if days <= 0 {
		days = 7
	}

	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02") + " 00:00:00"

	// Overall stats
	var totalQueries, slowQueries int
	var avgTime sql.NullFloat64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN execution_time >= 1000 THEN 1 ELSE 0 END), 0), CAST(COALESCE(AVG(execution_time), 0) AS REAL)
		 FROM query_history WHERE created_at >= ?`, startDate,
	).Scan(&totalQueries, &slowQueries, &avgTime)
	if err != nil {
		return nil, fmt.Errorf("get overall stats: %w", err)
	}

	slowRate := float64(0)
	if totalQueries > 0 {
		slowRate = float64(slowQueries) / float64(totalQueries) * 100
	}

	// Daily trend
	rows, err := s.db.QueryContext(ctx,
		`SELECT DATE(created_at) as date, COUNT(*) as count,
		        CAST(COALESCE(AVG(execution_time), 0) AS INTEGER) as avg_time,
		        SUM(CASE WHEN execution_time >= 1000 THEN 1 ELSE 0 END) as slow_count
		 FROM query_history WHERE created_at >= ?
		 GROUP BY DATE(created_at) ORDER BY date`, startDate)
	if err != nil {
		return nil, fmt.Errorf("get daily trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var dailyTrend []DailyTrend
	for rows.Next() {
		var d DailyTrend
		if err := rows.Scan(&d.Date, &d.Count, &d.AvgTime, &d.SlowCount); err != nil {
			return nil, fmt.Errorf("scan daily trend: %w", err)
		}
		dailyTrend = append(dailyTrend, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily trend: %w", err)
	}

	// Per-datasource stats
	dsRows, err := s.db.QueryContext(ctx,
		`SELECT qh.datasource_id, COALESCE(ds.name, '未知'), COUNT(*) as count,
		        CAST(COALESCE(AVG(qh.execution_time), 0) AS INTEGER) as avg_time
		 FROM query_history qh
		 LEFT JOIN datasources ds ON qh.datasource_id = ds.id
		 WHERE qh.created_at >= ?
		 GROUP BY qh.datasource_id, ds.name
		 ORDER BY count DESC`, startDate)
	if err != nil {
		return nil, fmt.Errorf("get datasource stats: %w", err)
	}
	defer func() { _ = dsRows.Close() }()

	var dsStats []DatasourceStats
	for dsRows.Next() {
		var d DatasourceStats
		if err := dsRows.Scan(&d.DatasourceID, &d.DatasourceName, &d.Count, &d.AvgTime); err != nil {
			return nil, fmt.Errorf("scan datasource stats: %w", err)
		}
		dsStats = append(dsStats, d)
	}
	if err := dsRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate datasource stats: %w", err)
	}

	// Top 10 slow queries
	topRows, err := s.db.QueryContext(ctx,
		`SELECT qh.id, qh.sql_summary, qh.execution_time, COALESCE(ds.name, '未知'), qh.created_at
		 FROM query_history qh
		 LEFT JOIN datasources ds ON qh.datasource_id = ds.id
		 WHERE qh.created_at >= ?
		 ORDER BY qh.execution_time DESC LIMIT 10`, startDate)
	if err != nil {
		return nil, fmt.Errorf("get top slow queries: %w", err)
	}
	defer func() { _ = topRows.Close() }()

	var topSlow []TopSlowQuery
	for topRows.Next() {
		var t TopSlowQuery
		if err := topRows.Scan(&t.ID, &t.SQLSummary, &t.ExecutionTime, &t.DatasourceName, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan top slow query: %w", err)
		}
		topSlow = append(topSlow, t)
	}
	if err := topRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate top slow queries: %w", err)
	}

	return &PerformanceStats{
		TotalQueries:    totalQueries,
		SlowQueries:     slowQueries,
		AvgTime:         int64(avgTime.Float64),
		SlowQueryRate:   slowRate,
		DailyTrend:      dailyTrend,
		DatasourceStats: dsStats,
		TopSlowQueries:  topSlow,
	}, nil
}
