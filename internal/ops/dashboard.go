package ops

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entauditlog "github.com/whg517/sqlflow/internal/db/ent/auditlog"
	entdatasource "github.com/whg517/sqlflow/internal/db/ent/datasource"
	entqueryhistory "github.com/whg517/sqlflow/internal/db/ent/queryhistory"
	entticket "github.com/whg517/sqlflow/internal/db/ent/ticket"
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
	// time.Time rather than a string: the raw version stored whatever text the
	// driver produced, which differs between drivers. Every other timestamp the
	// API returns is a marshaled time.Time, so this one now matches.
	CreatedAt time.Time `json:"created_at"`
}

// cacheEntry holds a cached DashboardFullStats with an expiry time.
type cacheEntry struct {
	stats     *DashboardFullStats
	expiresAt time.Time
}

// DashboardService provides dashboard statistics.
type DashboardService struct {
	client *ent.Client

	// Cache for full stats (60s TTL)
	cache   cacheEntry
	cacheMu sync.RWMutex
}

const dashboardCacheTTL = 60 * time.Second

// NewDashboardService creates a new DashboardService.
func NewDashboardService(database *db.DB) *DashboardService {
	return &DashboardService{client: database.Client()}
}

// openTicketStatuses are the statuses a ticket passes through before anyone has
// decided on it.
//
// Shared because the stat card, its sparkline and the two entry points all have
// to agree on what "pending" means; they were four separate literals, and a
// status added to one of them would have quietly changed only part of the
// dashboard.
var openTicketStatuses = []string{"SUBMITTED", "AI_REVIEWED", "PENDING_APPROVAL"}

// pendingTickets counts tickets awaiting a decision.
func (s *DashboardService) pendingTickets() *ent.TicketQuery {
	return s.client.Ticket.Query().Where(entticket.StatusIn(openTicketStatuses...))
}

// recentQueries counts query history from the last seven days.
func (s *DashboardService) recentQueries() *ent.QueryHistoryQuery {
	return s.client.QueryHistory.Query().
		Where(entqueryhistory.CreatedAtGTE(time.Now().AddDate(0, 0, -7)))
}

// activeDatasources counts datasources in service.
func (s *DashboardService) activeDatasources() *ent.DataSourceQuery {
	return s.client.DataSource.Query().Where(entdatasource.StatusEQ("active"))
}

// statCards fills the three counters both entry points show.
func (s *DashboardService) statCards(ctx context.Context) (pending, recent, datasources int, err error) {
	if pending, err = s.pendingTickets().Count(ctx); err != nil {
		return 0, 0, 0, fmt.Errorf("query pending tickets: %w", err)
	}
	if recent, err = s.recentQueries().Count(ctx); err != nil {
		return 0, 0, 0, fmt.Errorf("query recent queries: %w", err)
	}
	if datasources, err = s.activeDatasources().Count(ctx); err != nil {
		return 0, 0, 0, fmt.Errorf("query active datasources: %w", err)
	}
	return pending, recent, datasources, nil
}

// GetStats returns aggregated dashboard statistics (original API, backward compatible).
func (s *DashboardService) GetStats(ctx context.Context) (*DashboardStats, error) {
	pending, recent, datasources, err := s.statCards(ctx)
	if err != nil {
		return nil, err
	}
	users, err := s.client.User.Query().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("query total users: %w", err)
	}
	return &DashboardStats{
		PendingTickets:    pending,
		RecentQueries7d:   recent,
		ActiveDatasources: datasources,
		TotalUsers:        users,
	}, nil
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

	pending, recent, datasources, err := s.statCards(ctx)
	if err != nil {
		return nil, err
	}
	stats.PendingTickets, stats.RecentQueries7d, stats.ActiveDatasources = pending, recent, datasources

	// 2. Sparklines: 3 metrics x 7 days.
	//
	// Each takes a function returning a fresh query so the day filter can be
	// added per bucket; passing one built query would accumulate seven
	// overlapping WHERE clauses.
	stats.PendingTicketSparkline, err = dailyCounts(ctx, func() *ent.TicketQuery { return s.pendingTickets() },
		func(q *ent.TicketQuery, from, to time.Time) *ent.TicketQuery {
			return q.Where(entticket.CreatedAtGTE(from), entticket.CreatedAtLT(to))
		})
	if err != nil {
		log.Printf("dashboard: pending ticket sparkline error: %v", err)
	}

	stats.QuerySparkline, err = dailyCounts(ctx, func() *ent.QueryHistoryQuery { return s.client.QueryHistory.Query() },
		func(q *ent.QueryHistoryQuery, from, to time.Time) *ent.QueryHistoryQuery {
			return q.Where(entqueryhistory.CreatedAtGTE(from), entqueryhistory.CreatedAtLT(to))
		})
	if err != nil {
		log.Printf("dashboard: query sparkline error: %v", err)
	}

	// Datasources are cumulative rather than per-day: the card shows how many
	// exist, so its sparkline is the running total at each day's end.
	stats.DatasourceSparkline, err = dailyCounts(ctx, func() *ent.DataSourceQuery { return s.activeDatasources() },
		func(q *ent.DataSourceQuery, _, to time.Time) *ent.DataSourceQuery {
			return q.Where(entdatasource.CreatedAtLT(to))
		})
	if err != nil {
		log.Printf("dashboard: datasource sparkline error: %v", err)
	}

	// 3. Ticket status distribution
	var dist []struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	err = s.client.Ticket.Query().
		GroupBy(entticket.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &dist)
	if err != nil {
		return nil, fmt.Errorf("query ticket distribution: %w", err)
	}
	for _, d := range dist {
		stats.TicketStatusDistribution[d.Status] = d.Count
	}

	// 4. Query trend
	stats.QueryTrend, err = s.getQueryTrend(ctx, startDate, endDate)
	if err != nil {
		log.Printf("dashboard: query trend error: %v", err)
	}

	// 5. Recent activity (latest 10 audit logs)
	recentLogs, err := s.client.AuditLog.Query().
		Order(ent.Desc(entauditlog.FieldCreatedAt)).
		Limit(10).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query recent activity: %w", err)
	}
	for _, l := range recentLogs {
		stats.RecentActivity = append(stats.RecentActivity, RecentActivityItem{
			ID:        int64(l.ID),
			UserID:    l.UserID,
			Action:    l.Action,
			IPAddress: l.IPAddress,
			CreatedAt: l.CreatedAt,
		})
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

// dailyCounts returns one count per day for the last seven days.
//
// Generic over the entity so each metric keeps its own typed predicates: the
// previous version took a SQL string that had to contain exactly two
// placeholders in the right order, which nothing checked.
//
// Day boundaries are computed in UTC to match the timestamps the database
// writes. Local time would put today's rows in yesterday's bucket for anyone
// east of UTC.
func dailyCounts[Q any](
	ctx context.Context,
	newQuery func() Q,
	restrict func(Q, time.Time, time.Time) Q,
) ([]int, error) {
	result := make([]int, 7)
	now := time.Now().UTC()
	for i := 6; i >= 0; i-- {
		dayStart := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		q := restrict(newQuery(), dayStart, dayStart.AddDate(0, 0, 1))
		c, ok := any(q).(interface {
			Count(context.Context) (int, error)
		})
		if !ok {
			return result, fmt.Errorf("query type %T cannot count", q)
		}
		count, err := c.Count(ctx)
		if err != nil {
			// A single bad bucket renders as zero rather than failing the whole
			// dashboard; the caller logs it.
			count = 0
		}
		result[6-i] = count
	}
	return result, nil
}

// getQueryTrend returns daily query counts within the given date range.
// Defaults to last 7 days if no dates provided.
func (s *DashboardService) getQueryTrend(ctx context.Context, startDate, endDate string) ([]DailyCount, error) {
	// Default: last 7 days (UTC, 与 SQLite now() 一致)
	if startDate == "" {
		startDate = time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().UTC().Format("2006-01-02")
	}

	parsedStart, _ := time.Parse("2006-01-02", startDate)
	parsedEnd, _ := time.Parse("2006-01-02", endDate)

	if parsedEnd.Before(parsedStart) {
		return nil, fmt.Errorf("end_date must be >= start_date")
	}
	if parsedEnd.Sub(parsedStart) > 30*24*time.Hour {
		return nil, fmt.Errorf("date range cannot exceed 30 days")
	}

	// Bucketed by to_char rather than DATE(): the destination is a string used
	// as a map key, and DATE() yields a date value. The bounds stay real
	// timestamps, so the comparison does not depend on how the column renders.
	var buckets []struct {
		Day   string `json:"day"`
		Count int    `json:"count"`
	}
	err := s.client.QueryHistory.Query().
		Where(
			entqueryhistory.CreatedAtGTE(parsedStart),
			entqueryhistory.CreatedAtLT(parsedEnd.AddDate(0, 0, 1)),
		).
		Modify(func(sel *entsql.Selector) {
			day := "to_char(" + sel.C(entqueryhistory.FieldCreatedAt) + ", 'YYYY-MM-DD')"
			sel.Select().
				AppendSelect(entsql.As(day, "day"), entsql.As("COUNT(*)", "count")).
				GroupBy(day).
				OrderBy("day")
		}).
		Scan(ctx, &buckets)
	if err != nil {
		return nil, fmt.Errorf("query trend: %w", err)
	}

	// Build a map of date -> count, then fill gaps with 0
	countMap := make(map[string]int)
	for _, b := range buckets {
		countMap[b.Day] = b.Count
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
