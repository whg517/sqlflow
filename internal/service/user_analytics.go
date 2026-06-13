package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// --- Anomaly Detection Thresholds (configurable) ---

const (
	// BurstQueryThreshold is the maximum number of queries allowed within 1 hour before flagging as anomalous.
	BurstQueryThreshold = 50

	// OffHoursThreshold is the maximum number of operations during off-hours (22:00-08:00) before flagging.
	OffHoursThreshold = 20

	// AnalyticsCacheTTL is the default cache duration for analytics results.
	AnalyticsCacheTTL = 10 * time.Minute
)

// --- User Analytics Data Types ---

// UserAnalytics represents aggregated user behavior analytics.
type UserAnalytics struct {
	GeneratedAt          time.Time             `json:"generated_at"`
	TimeRange            string                `json:"time_range"`
	StartDate            string                `json:"start_date"`
	EndDate              string                `json:"end_date"`
	UserID               int64                 `json:"user_id,omitempty"`
	TopActiveUsers       []ActiveUserEntry     `json:"top_active_users"`
	QueryFrequency       []QueryFrequencyEntry `json:"query_frequency"`
	ActionTypeBreakdown  []ActionTypeEntry     `json:"action_type_breakdown"`
	AnomalousBehaviors   []AnomalyEntry        `json:"anomalous_behaviors"`
}

// ActiveUserEntry represents a user's activity summary.
type ActiveUserEntry struct {
	Rank            int    `json:"rank"`
	UserID          int64  `json:"user_id"`
	Username        string `json:"username"`
	QueryCount      int64  `json:"query_count"`
	ApprovalCount   int64  `json:"approval_count"`
	ActiveDays      int64  `json:"active_days"`
	TotalActions    int64  `json:"total_actions"`
}

// QueryFrequencyEntry represents query frequency distribution.
type QueryFrequencyEntry struct {
	Period string `json:"period"`
	Count  int64  `json:"count"`
}

// ActionTypeEntry represents action type distribution.
type ActionTypeEntry struct {
	Action string  `json:"action"`
	Count  int64   `json:"count"`
	Ratio  float64 `json:"ratio"`
}

// AnomalyEntry represents a detected anomalous behavior.
type AnomalyEntry struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	AnomalyType string `json:"anomaly_type"`
	Description string `json:"description"`
	Count       int64  `json:"count"`
	TimeWindow  string `json:"time_window"`
}

// --- Cache ---

// analyticsCacheEntry holds a cached analytics result with expiration.
type analyticsCacheEntry struct {
	data     *UserAnalytics
	cachedAt time.Time
}

// analyticsCache provides a per-key in-memory cache for analytics results.
type analyticsCache struct {
	mu    sync.RWMutex
	entries map[string]*analyticsCacheEntry
	ttl     time.Duration
}

var (
	globalAnalyticsCache = &analyticsCache{
		entries: make(map[string]*analyticsCacheEntry),
		ttl:     AnalyticsCacheTTL,
	}
)

func (c *analyticsCache) get(key string) (*UserAnalytics, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || time.Since(entry.cachedAt) >= c.ttl {
		return nil, false
	}
	return entry.data, true
}

func (c *analyticsCache) set(key string, data *UserAnalytics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &analyticsCacheEntry{data: data, cachedAt: time.Now()}
}

// --- Analytics Query Parameters ---

// AnalyticsParams holds the parameters for user analytics queries.
type AnalyticsParams struct {
	TimeRange string // "7d", "30d", "90d", or "custom"
	StartDate string // used when TimeRange == "custom", format: "2006-01-02"
	EndDate   string // used when TimeRange == "custom", format: "2006-01-02"
	UserID    int64  // optional, specific user drill-down (0 = all users)
}

// GetUserAnalytics returns user behavior analytics with caching.
func (s *AuditReportService) GetUserAnalytics(ctx context.Context, params AnalyticsParams) (*UserAnalytics, error) {
	// Validate params
	if err := validateAnalyticsParams(&params); err != nil {
		return nil, err
	}

	// Build cache key
	cacheKey := fmt.Sprintf("%s:%s:%s:%d", params.TimeRange, params.StartDate, params.EndDate, params.UserID)

	// Check cache
	if cached, ok := globalAnalyticsCache.get(cacheKey); ok {
		return cached, nil
	}

	// Compute date range
	startDate, days, err := resolveDateRange(params)
	if err != nil {
		return nil, err
	}

	result := &UserAnalytics{
		GeneratedAt: time.Now().UTC(),
		TimeRange:   params.TimeRange,
		StartDate:   startDate,
		EndDate:     time.Now().Format("2006-01-02"),
		UserID:      params.UserID,
	}

	// 1. Top 10 active users
	result.TopActiveUsers, err = s.queryTopActiveUsers(ctx, startDate, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("查询活跃用户失败: %w", err)
	}

	// 2. Query frequency distribution
	result.QueryFrequency, err = s.queryFrequencyDistribution(ctx, startDate, days, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("查询频率分布失败: %w", err)
	}

	// 3. Action type breakdown
	result.ActionTypeBreakdown, err = s.queryActionTypeBreakdown(ctx, startDate, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("查询操作类型分布失败: %w", err)
	}

	// 4. Anomalous behaviors
	result.AnomalousBehaviors, err = s.detectAnomalousBehaviors(ctx, startDate, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("异常行为检测失败: %w", err)
	}

	// Cache result
	globalAnalyticsCache.set(cacheKey, result)

	return result, nil
}

// validateAnalyticsParams validates and normalizes analytics parameters.
func validateAnalyticsParams(params *AnalyticsParams) error {
	switch params.TimeRange {
	case "7d", "30d", "90d":
		// valid presets
	case "custom":
		if params.StartDate == "" || params.EndDate == "" {
			return fmt.Errorf("custom 时间范围需要 start_date 和 end_date 参数")
		}
		// Validate date format
		start, err := time.Parse("2006-01-02", params.StartDate)
		if err != nil {
			return fmt.Errorf("start_date 格式无效，需要 YYYY-MM-DD: %w", err)
		}
		end, err := time.Parse("2006-01-02", params.EndDate)
		if err != nil {
			return fmt.Errorf("end_date 格式无效，需要 YYYY-MM-DD: %w", err)
		}
		if end.Before(start) {
			return fmt.Errorf("end_date 不能早于 start_date")
		}
		if start.AddDate(0, 0, 365).Before(end) {
			return fmt.Errorf("自定义时间范围不能超过 365 天")
		}
	default:
		// Default to 7d
		params.TimeRange = "7d"
	}
	return nil
}

// resolveDateRange returns the start date string and day count from params.
func resolveDateRange(params AnalyticsParams) (string, int, error) {
	switch params.TimeRange {
	case "7d":
		return time.Now().AddDate(0, 0, -7).Format("2006-01-02") + " 00:00:00", 7, nil
	case "30d":
		return time.Now().AddDate(0, 0, -30).Format("2006-01-02") + " 00:00:00", 30, nil
	case "90d":
		return time.Now().AddDate(0, 0, -90).Format("2006-01-02") + " 00:00:00", 90, nil
	case "custom":
		start, _ := time.Parse("2006-01-02", params.StartDate)
		end, _ := time.Parse("2006-01-02", params.EndDate)
		days := int(end.Sub(start).Hours()/24) + 1
		return params.StartDate + " 00:00:00", days, nil
	default:
		return time.Now().AddDate(0, 0, -7).Format("2006-01-02") + " 00:00:00", 7, nil
	}
}

// ParseAnalyticsUserID parses and validates a user_id query parameter.
func ParseAnalyticsUserID(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效的 user_id: %s", s)
	}
	return id, nil
}

func (s *AuditReportService) queryTopActiveUsers(ctx context.Context, startDate string, userID int64) ([]ActiveUserEntry, error) {
	query := `SELECT a.user_id, COALESCE(u.username, ''),
	          SUM(CASE WHEN a.action IN ('query_execute', 'query_submit') THEN 1 ELSE 0 END) as query_count,
	          SUM(CASE WHEN a.action IN ('ticket_approve', 'ticket_reject') THEN 1 ELSE 0 END) as approval_count,
	          COUNT(DISTINCT SUBSTR(a.created_at, 1, 10)) as active_days,
	          COUNT(*) as total_actions
	          FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id
	          WHERE a.created_at >= ?`
	args := []interface{}{startDate}

	if userID > 0 {
		query += ` AND a.user_id = ?`
		args = append(args, userID)
	}

	query += ` GROUP BY a.user_id, u.username ORDER BY total_actions DESC LIMIT 10`

	rows, err := s.database.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ActiveUserEntry
	rank := 1
	for rows.Next() {
		var e ActiveUserEntry
		if err := rows.Scan(&e.UserID, &e.Username, &e.QueryCount, &e.ApprovalCount, &e.ActiveDays, &e.TotalActions); err != nil {
			return nil, err
		}
		e.Rank = rank
		result = append(result, e)
		rank++
	}
	return result, rows.Err()
}

func (s *AuditReportService) queryFrequencyDistribution(ctx context.Context, startDate string, days int, userID int64) ([]QueryFrequencyEntry, error) {
	query := `SELECT SUBSTR(created_at, 1, 10) as period, COUNT(*) as count
	          FROM audit_logs WHERE created_at >= ?`
	args := []interface{}{startDate}

	if userID > 0 {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}

	query += ` GROUP BY SUBSTR(created_at, 1, 10) ORDER BY period`

	rows, err := s.database.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []QueryFrequencyEntry
	for rows.Next() {
		var e QueryFrequencyEntry
		if err := rows.Scan(&e.Period, &e.Count); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *AuditReportService) queryActionTypeBreakdown(ctx context.Context, startDate string, userID int64) ([]ActionTypeEntry, error) {
	query := `SELECT action, COUNT(*) as count FROM audit_logs WHERE created_at >= ? AND action != ''`
	args := []interface{}{startDate}

	if userID > 0 {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}

	query += ` GROUP BY action ORDER BY count DESC`

	rows, err := s.database.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ActionTypeEntry
	var total int64
	for rows.Next() {
		var e ActionTypeEntry
		if err := rows.Scan(&e.Action, &e.Count); err != nil {
			return nil, err
		}
		total += e.Count
		entries = append(entries, e)
	}

	// Calculate ratios
	for i := range entries {
		if total > 0 {
			entries[i].Ratio = float64(entries[i].Count) / float64(total) * 100
		}
	}

	return entries, rows.Err()
}

func (s *AuditReportService) detectAnomalousBehaviors(ctx context.Context, startDate string, userID int64) ([]AnomalyEntry, error) {
	var result []AnomalyEntry

	// 1. Short-time burst: >BurstQueryThreshold queries in 1 hour
	burstQuery := `SELECT a.user_id, COALESCE(u.username, ''),
	               SUBSTR(a.created_at, 1, 13) as hour_window,
	               COUNT(*) as cnt
	               FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id
	               WHERE a.created_at >= ? AND a.action IN ('query_execute', 'query_submit')`
	burstArgs := []interface{}{startDate}
	if userID > 0 {
		burstQuery += ` AND a.user_id = ?`
		burstArgs = append(burstArgs, userID)
	}
	burstQuery += fmt.Sprintf(` GROUP BY a.user_id, u.username, SUBSTR(a.created_at, 1, 13) HAVING cnt > %d ORDER BY cnt DESC LIMIT 20`, BurstQueryThreshold)

	rows, err := s.database.DB.QueryContext(ctx, burstQuery, burstArgs...)
	if err != nil {
		return nil, fmt.Errorf("查询短时大量查询失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var uid int64
		var username, hourWindow string
		var cnt int64
		if err := rows.Scan(&uid, &username, &hourWindow, &cnt); err != nil {
			return nil, err
		}
		result = append(result, AnomalyEntry{
			UserID:      uid,
			Username:    username,
			AnomalyType: "burst_queries",
			Description: fmt.Sprintf("1小时内执行 %d 次查询（阈值 %d）", cnt, BurstQueryThreshold),
			Count:       cnt,
			TimeWindow:  hourWindow,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Off-hours high-frequency: 22:00-08:00 with high activity
	offHoursQuery := `SELECT a.user_id, COALESCE(u.username, ''),
	                  SUBSTR(a.created_at, 12, 2) as hour_of_day,
	                  COUNT(*) as cnt
	                  FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id
	                  WHERE a.created_at >= ? AND (CAST(SUBSTR(a.created_at, 12, 2) AS INTEGER) >= 22 OR CAST(SUBSTR(a.created_at, 12, 2) AS INTEGER) < 8)`
	offArgs := []interface{}{startDate}
	if userID > 0 {
		offHoursQuery += ` AND a.user_id = ?`
		offArgs = append(offArgs, userID)
	}
	offHoursQuery += fmt.Sprintf(` GROUP BY a.user_id, u.username HAVING cnt > %d ORDER BY cnt DESC LIMIT 20`, OffHoursThreshold)

	rows2, err := s.database.DB.QueryContext(ctx, offHoursQuery, offArgs...)
	if err != nil {
		return nil, fmt.Errorf("查询非工作时间高频操作失败: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var uid int64
		var username string
		var hourOfDay string
		var cnt int64
		if err := rows2.Scan(&uid, &username, &hourOfDay, &cnt); err != nil {
			return nil, err
		}
		result = append(result, AnomalyEntry{
			UserID:      uid,
			Username:    username,
			AnomalyType: "off_hours_high_frequency",
			Description: fmt.Sprintf("非工作时间（22:00-08:00）执行 %d 次操作（阈值 %d）", cnt, OffHoursThreshold),
			Count:       cnt,
			TimeWindow:  "22:00-08:00",
		})
	}

	return result, rows2.Err()
}

// InvalidateAnalyticsCache clears all cached analytics results.
func InvalidateAnalyticsCache() {
	globalAnalyticsCache.mu.Lock()
	globalAnalyticsCache.entries = make(map[string]*analyticsCacheEntry)
	globalAnalyticsCache.mu.Unlock()
}
