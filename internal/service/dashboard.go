package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

// DashboardStats holds aggregated statistics for the dashboard overview.
// Kept for backward compatibility — the original API returns this.
type DashboardStats struct {
	PendingTickets    int `json:"pending_tickets"`
	RecentQueries7d   int `json:"recent_queries_7d"`
	ActiveDatasources int `json:"active_datasources"`
	TotalUsers        int `json:"total_users"`
}

// DashboardFullStats returns all dashboard data in a single response.
type DashboardFullStats struct {
	// Stat cards
	PendingTickets    int `json:"pending_tickets"`
	RecentQueries7d   int `json:"recent_queries_7d"`
	ActiveDatasources int `json:"active_datasources"`

	// Sparkline: 3 metrics × 7 days
	PendingTicketSparkline []int `json:"pending_ticket_sparkline"`
	QuerySparkline         []int `json:"query_sparkline"`
	DatasourceSparkline    []int `json:"datasource_sparkline"`

	// Ticket status distribution
	TicketStatusDistribution map[string]int `json:"ticket_status_distribution"`

	// Query trend: daily query counts within [startDate, endDate]
	QueryTrend []DailyCount `json:"query_trend"`

	// Recent activity: latest 10 audit logs
	RecentActivity []RecentActivityItem `json:"recent_activity"`
}

// DailyCount represents a single day's aggregated count.
type DailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// RecentActivityItem represents a single audit log entry for the activity feed.
type RecentActivityItem struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Action    string `json:"action"`
	IPAddress string `json:"ip_address"`
	CreatedAt string `json:"created_at"`
}

// cacheEntry holds a cached DashboardFullStats with an expiry time.
type cacheEntry struct {
	stats    *DashboardFullStats
	expiresAt time.Time
}

// DashboardService provides dashboard statistics.
type DashboardService struct {
	database *db.DB

	// Cache for full stats (60s TTL)
	cache   cacheEntry
	cacheMu sync.RWMutex
}

const dashboardCacheTTL = 60 * time.Second

// NewDashboardService creates a new DashboardService.
func NewDashboardService(database *db.DB) *DashboardService {
	return &DashboardService{database: database}
}

// GetStats returns aggregated dashboard statistics (original API, backward compatible).
func (s *DashboardService) GetStats(ctx context.Context) (*DashboardStats, error) {
	stats := &DashboardStats{}

	err := s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE status IN ('SUBMITTED', 'AI_REVIEWED', 'PENDING_APPROVAL')`,
	).Scan(&stats.PendingTickets)
	if err != nil {
		return nil, fmt.Errorf("query pending tickets: %w", err)
	}

	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM query_history WHERE created_at >= datetime('now', '-7 days')`,
	).Scan(&stats.RecentQueries7d)
	if err != nil {
		return nil, fmt.Errorf("query recent queries: %w", err)
	}

	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM datasources WHERE status = 'active'`,
	).Scan(&stats.ActiveDatasources)
	if err != nil {
		return nil, fmt.Errorf("query active datasources: %w", err)
	}

	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users`,
	).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("query total users: %w", err)
	}

	return stats, nil
}

// GetFullStats returns all dashboard data in a single request.
// startDate/endDate control the query trend time range (default: last 7 days).
func (s *DashboardService) GetFullStats(ctx context.Context, startDate, endDate string) (*DashboardFullStats, error) {
	// Validate date parameters early
	if startDate != "" {
		if _, err := time.Parse("2006-01-02", startDate); err != nil {
			return nil, fmt.Errorf("invalid start_date: %w", err)
		}
	}
	if endDate != "" {
		if _, err := time.Parse("2006-01-02", endDate); err != nil {
			return nil, fmt.Errorf("invalid end_date: %w", err)
		}
	}
	// Validate date range logic
	if startDate != "" && endDate != "" {
		ps, _ := time.Parse("2006-01-02", startDate)
		pe, _ := time.Parse("2006-01-02", endDate)
		if pe.Before(ps) {
			return nil, fmt.Errorf("end_date must be >= start_date")
		}
		if pe.Sub(ps) > 30*24*time.Hour {
			return nil, fmt.Errorf("date range cannot exceed 30 days")
		}
	}

	// Check cache
	s.cacheMu.RLock()
	if time.Now().Before(s.cache.expiresAt) && s.cache.stats != nil {
		cached := s.cache.stats
		s.cacheMu.RUnlock()
		// Return a copy with fresh query trend (date-filtered)
		result := *cached
		trend, err := s.getQueryTrend(ctx, startDate, endDate)
		if err != nil {
			log.Printf("dashboard: query trend error: %v", err)
		}
		result.QueryTrend = trend
		return &result, nil
	}
	s.cacheMu.RUnlock()

	stats := &DashboardFullStats{
		TicketStatusDistribution: make(map[string]int),
	}

	// 1. Stat cards (parallelizable but keep simple for SQLite)
	var err error
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE status IN ('SUBMITTED', 'AI_REVIEWED', 'PENDING_APPROVAL')`,
	).Scan(&stats.PendingTickets)
	if err != nil {
		return nil, fmt.Errorf("query pending tickets: %w", err)
	}

	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM query_history WHERE created_at >= datetime('now', '-7 days')`,
	).Scan(&stats.RecentQueries7d)
	if err != nil {
		return nil, fmt.Errorf("query recent queries: %w", err)
	}

	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM datasources WHERE status = 'active'`,
	).Scan(&stats.ActiveDatasources)
	if err != nil {
		return nil, fmt.Errorf("query active datasources: %w", err)
	}

	// 2. Sparklines: 3 metrics × 7 days
	stats.PendingTicketSparkline, err = s.getSparkline(ctx,
		`SELECT COUNT(*) FROM tickets WHERE status IN ('SUBMITTED', 'AI_REVIEWED', 'PENDING_APPROVAL') AND created_at >= ? AND created_at < ?`)
	if err != nil {
		log.Printf("dashboard: pending ticket sparkline error: %v", err)
	}

	stats.QuerySparkline, err = s.getSparkline(ctx,
		`SELECT COUNT(*) FROM query_history WHERE created_at >= ? AND created_at < ?`)
	if err != nil {
		log.Printf("dashboard: query sparkline error: %v", err)
	}

	stats.DatasourceSparkline, err = s.getSparkline(ctx,
		`SELECT COUNT(*) FROM datasources WHERE status = 'active' AND created_at <= ?`)
	if err != nil {
		log.Printf("dashboard: datasource sparkline error: %v", err)
	}

	// 3. Ticket status distribution
	distRows, err := s.database.DB.QueryContext(ctx,
		`SELECT status, COUNT(*) as cnt FROM tickets GROUP BY status`,
	)
	if err != nil {
		return nil, fmt.Errorf("query ticket distribution: %w", err)
	}
	defer func() { _ = distRows.Close() }()

	for distRows.Next() {
		var status string
		var cnt int
		if err := distRows.Scan(&status, &cnt); err != nil {
			continue
		}
		stats.TicketStatusDistribution[status] = cnt
	}

	// 4. Query trend
	stats.QueryTrend, err = s.getQueryTrend(ctx, startDate, endDate)
	if err != nil {
		log.Printf("dashboard: query trend error: %v", err)
	}

	// 5. Recent activity (latest 10 audit logs)
	activityRows, err := s.database.DB.QueryContext(ctx,
		`SELECT id, user_id, action, ip_address, created_at FROM audit_logs ORDER BY created_at DESC LIMIT 10`,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent activity: %w", err)
	}
	defer func() { _ = activityRows.Close() }()

	for activityRows.Next() {
		var item RecentActivityItem
		if err := activityRows.Scan(&item.ID, &item.UserID, &item.Action, &item.IPAddress, &item.CreatedAt); err != nil {
			continue
		}
		stats.RecentActivity = append(stats.RecentActivity, item)
	}

	// Update cache
	s.cacheMu.Lock()
	s.cache = cacheEntry{
		stats:     stats,
		expiresAt: time.Now().Add(dashboardCacheTTL),
	}
	s.cacheMu.Unlock()

	return stats, nil
}

// getSparkline returns 7 daily counts for the given query.
// The query MUST have exactly 2 placeholders: start_time and end_time.
func (s *DashboardService) getSparkline(ctx context.Context, query string) ([]int, error) {
	result := make([]int, 7)
	now := time.Now()

	for i := 6; i >= 0; i-- {
		dayStart := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		dayEnd := dayStart.AddDate(0, 0, 1)

		var count int
		err := s.database.DB.QueryRowContext(ctx, query,
			dayStart.Format("2006-01-02 15:04:05"),
			dayEnd.Format("2006-01-02 15:04:05"),
		).Scan(&count)
		if err != nil {
			count = 0
		}
		result[6-i] = count
	}

	return result, nil
}

// getQueryTrend returns daily query counts within the given date range.
// Defaults to last 7 days if no dates provided.
func (s *DashboardService) getQueryTrend(ctx context.Context, startDate, endDate string) ([]DailyCount, error) {
	// Default: last 7 days
	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	parsedStart, _ := time.Parse("2006-01-02", startDate)
	parsedEnd, _ := time.Parse("2006-01-02", endDate)

	if parsedEnd.Before(parsedStart) {
		return nil, fmt.Errorf("end_date must be >= start_date")
	}
	if parsedEnd.Sub(parsedStart) > 30*24*time.Hour {
		return nil, fmt.Errorf("date range cannot exceed 30 days")
	}

	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT DATE(created_at) as day, COUNT(*) as cnt
		 FROM query_history
		 WHERE created_at >= ? AND created_at < ?
		 GROUP BY DATE(created_at)
		 ORDER BY day`,
		startDate+" 00:00:00",
		parsedEnd.AddDate(0, 0, 1).Format("2006-01-02")+" 00:00:00",
	)
	if err != nil {
		return nil, fmt.Errorf("query trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Build a map of date -> count, then fill gaps with 0
	countMap := make(map[string]int)
	for rows.Next() {
		var day string
		var cnt int
		if err := rows.Scan(&day, &cnt); err != nil {
			continue
		}
		countMap[day] = cnt
	}

	var result []DailyCount
	for d := parsedStart; !d.After(parsedEnd); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		result = append(result, DailyCount{
			Date:  dateStr,
			Count: countMap[dateStr],
		})
	}

	return result, nil
}
