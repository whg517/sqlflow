package service

import (
	"context"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

// TimeRange represents a dashboard time range filter.
type TimeRange string

const (
	TimeRangeToday    TimeRange = "today"
	TimeRangeThisWeek TimeRange = "this_week"
	TimeRangeThisMonth TimeRange = "this_month"
	TimeRangeLast30d  TimeRange = "last_30d"
)

// parseTimeRange converts a TimeRange to (start, end) timestamps in UTC.
// end is always "now".
func parseTimeRange(tr TimeRange) (time.Time, time.Time) {
	now := time.Now().UTC()
	var start time.Time
	switch tr {
	case TimeRangeToday:
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case TimeRangeThisWeek:
		// Monday of this week
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start = time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, time.UTC)
	case TimeRangeThisMonth:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	case TimeRangeLast30d:
		start = now.AddDate(0, 0, -30)
	default:
		// Default to last 7 days
		start = now.AddDate(0, 0, -7)
	}
	return start, now
}

// DashboardStats holds aggregated statistics for the dashboard overview.
type DashboardStats struct {
	PendingTickets    int              `json:"pending_tickets"`
	RecentQueries7d   int              `json:"recent_queries_7d"`
	ActiveDatasources int              `json:"active_datasources"`
	TotalUsers        int              `json:"total_users"`
}

// DailyCount represents a single data point for sparklines and trend charts.
type DailyCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// TicketStatusCount represents ticket count for a given status.
type TicketStatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

// AuditLogEntry is a lightweight audit log entry for the dashboard activity feed.
type AuditLogEntry struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	Username  string `json:"username"`
	Action    string `json:"action"`
	Summary   string `json:"summary"`
}

// DashboardOverview is the aggregated response for the new dashboard API.
type DashboardOverview struct {
	// Stat cards with sparkline data (last 7 days always)
	Stats             DashboardStats      `json:"stats"`
	QueryTrend        []DailyCount        `json:"query_trend"`
	QuerySparkline    []DailyCount        `json:"query_sparkline"`
	TicketSparkline   []DailyCount        `json:"ticket_sparkline"`
	TicketStatusDist  []TicketStatusCount `json:"ticket_status_dist"`
	RecentActivities  []AuditLogEntry     `json:"recent_activities"`
}

// DashboardService provides dashboard statistics.
type DashboardService struct {
	database *db.DB
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(database *db.DB) *DashboardService {
	return &DashboardService{database: database}
}

// GetStats returns aggregated dashboard statistics (legacy endpoint, unchanged).
func (s *DashboardService) GetStats(ctx context.Context) (*DashboardStats, error) {
	stats := &DashboardStats{}

	// PendingTickets: tickets with status in (SUBMITTED, AI_REVIEWED, PENDING_APPROVAL)
	err := s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE status IN ('SUBMITTED', 'AI_REVIEWED', 'PENDING_APPROVAL')`,
	).Scan(&stats.PendingTickets)
	if err != nil {
		return nil, fmt.Errorf("query pending tickets: %w", err)
	}

	// RecentQueries7d: query_history in the last 7 days
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM query_history WHERE created_at >= datetime('now', '-7 days')`,
	).Scan(&stats.RecentQueries7d)
	if err != nil {
		return nil, fmt.Errorf("query recent queries: %w", err)
	}

	// ActiveDatasources: datasources with status = 'active'
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM datasources WHERE status = 'active'`,
	).Scan(&stats.ActiveDatasources)
	if err != nil {
		return nil, fmt.Errorf("query active datasources: %w", err)
	}

	// TotalUsers: total user count
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users`,
	).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("query total users: %w", err)
	}

	return stats, nil
}

// GetOverview returns the full dashboard overview for the given time range.
func (s *DashboardService) GetOverview(ctx context.Context, timeRange TimeRange) (*DashboardOverview, error) {
	overview := &DashboardOverview{}

	// 1. Basic stats (same as legacy)
	stats, err := s.GetStats(ctx)
	if err != nil {
		return nil, err
	}
	overview.Stats = *stats

	// 2. Time range bounds
	start, _ := parseTimeRange(timeRange)
	startStr := start.UTC().Format("2006-01-02 15:04:05")

	// 3. Query trend (daily counts within time range)
	queryTrend, err := s.getDailyCounts(ctx, "query_history", "created_at", startStr)
	if err != nil {
		return nil, fmt.Errorf("query trend: %w", err)
	}
	overview.QueryTrend = queryTrend

	// 4. Query sparkline (always last 7 days)
	sparkStart := time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")
	querySpark, err := s.getDailyCounts(ctx, "query_history", "created_at", sparkStart)
	if err != nil {
		return nil, fmt.Errorf("query sparkline: %w", err)
	}
	overview.QuerySparkline = querySpark

	// 5. Ticket sparkline (always last 7 days, new tickets created)
	ticketSpark, err := s.getDailyCounts(ctx, "tickets", "created_at", sparkStart)
	if err != nil {
		return nil, fmt.Errorf("ticket sparkline: %w", err)
	}
	overview.TicketSparkline = ticketSpark

	// 6. Ticket status distribution (all non-terminal statuses)
	overview.TicketStatusDist, err = s.getTicketStatusDist(ctx)
	if err != nil {
		return nil, fmt.Errorf("ticket status dist: %w", err)
	}

	// 7. Recent audit activities (last 10)
	overview.RecentActivities, err = s.getRecentActivities(ctx)
	if err != nil {
		return nil, fmt.Errorf("recent activities: %w", err)
	}

	return overview, nil
}

// getDailyCounts queries daily count aggregation for a table/column within the given start time.
func (s *DashboardService) getDailyCounts(ctx context.Context, table, column, startStr string) ([]DailyCount, error) {
	query := fmt.Sprintf(
		`SELECT date(%s) as d, COUNT(*) as cnt FROM %s WHERE %s >= ? GROUP BY d ORDER BY d`,
		column, table, column,
	)
	rows, err := s.database.DB.QueryContext(ctx, query, startStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DailyCount
	for rows.Next() {
		var dc DailyCount
		if err := rows.Scan(&dc.Date, &dc.Count); err != nil {
			return nil, err
		}
		result = append(result, dc)
	}
	return result, rows.Err()
}

// getTicketStatusDist returns the count of tickets grouped by status.
func (s *DashboardService) getTicketStatusDist(ctx context.Context) ([]TicketStatusCount, error) {
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT status, COUNT(*) as cnt FROM tickets GROUP BY status ORDER BY cnt DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TicketStatusCount
	for rows.Next() {
		var tc TicketStatusCount
		if err := rows.Scan(&tc.Status, &tc.Count); err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, rows.Err()
}

// getRecentActivities returns the 10 most recent audit log entries with username.
func (s *DashboardService) getRecentActivities(ctx context.Context) ([]AuditLogEntry, error) {
	query := `
		SELECT a.id, a.created_at, u.username, a.action,
			COALESCE(a.sql_summary, CASE WHEN a.action = 'LOGIN' THEN '用户登录' ELSE a.action END)
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.user_id
		ORDER BY a.created_at DESC
		LIMIT 10
	`
	rows, err := s.database.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AuditLogEntry
	for rows.Next() {
		var entry AuditLogEntry
		if err := rows.Scan(&entry.ID, &entry.CreatedAt, &entry.Username, &entry.Action, &entry.Summary); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}
