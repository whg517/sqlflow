package audit

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/whg517/sqlflow/internal/db/ent"
	entauditlog "github.com/whg517/sqlflow/internal/db/ent/auditlog"
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
	GeneratedAt         time.Time             `json:"generated_at"`
	TimeRange           string                `json:"time_range"`
	StartDate           string                `json:"start_date"`
	EndDate             string                `json:"end_date"`
	UserID              int64                 `json:"user_id,omitempty"`
	TopActiveUsers      []ActiveUserEntry     `json:"top_active_users"`
	QueryFrequency      []QueryFrequencyEntry `json:"query_frequency"`
	ActionTypeBreakdown []ActionTypeEntry     `json:"action_type_breakdown"`
	AnomalousBehaviors  []AnomalyEntry        `json:"anomalous_behaviors"`
}

// ActiveUserEntry represents a user's activity summary.
type ActiveUserEntry struct {
	Rank          int    `json:"rank"`
	UserID        int64  `json:"user_id"`
	Username      string `json:"username"`
	QueryCount    int64  `json:"query_count"`
	ApprovalCount int64  `json:"approval_count"`
	ActiveDays    int64  `json:"active_days"`
	TotalActions  int64  `json:"total_actions"`
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
	mu      sync.RWMutex
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
func (s *ReportService) GetUserAnalytics(ctx context.Context, params AnalyticsParams) (*UserAnalytics, error) {
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
		StartDate:   startDate.Format("2006-01-02"),
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
// resolveDateRange returns the inclusive start of the report window and the
// number of days it spans.
//
// It returns a time.Time rather than a formatted string: created_at is a real
// timestamp column, and comparing it against text only ever worked because
// SQLite stored timestamps as text too.
func resolveDateRange(params AnalyticsParams) (time.Time, int, error) {
	midnight := func(t time.Time) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	}
	back := func(days int) (time.Time, int, error) {
		return midnight(time.Now().AddDate(0, 0, -days)), days, nil
	}
	switch params.TimeRange {
	case "30d":
		return back(30)
	case "90d":
		return back(90)
	case "custom":
		start, err := time.Parse("2006-01-02", params.StartDate)
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("解析开始日期失败: %w", err)
		}
		end, err := time.Parse("2006-01-02", params.EndDate)
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("解析结束日期失败: %w", err)
		}
		return midnight(start), int(end.Sub(start).Hours()/24) + 1, nil
	default:
		return back(7)
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

// auditScope is the window every analytics query starts from: the report range,
// optionally narrowed to one user.
func (s *ReportService) auditScope(startDate time.Time, userID int64) *ent.AuditLogQuery {
	q := s.client.AuditLog.Query().Where(entauditlog.CreatedAtGTE(startDate))
	if userID > 0 {
		q = q.Where(entauditlog.UserIDEQ(userID))
	}
	return q
}

func (s *ReportService) queryTopActiveUsers(ctx context.Context, startDate time.Time, userID int64) ([]ActiveUserEntry, error) {
	var result []ActiveUserEntry
	err := s.auditScope(startDate, userID).
		Modify(func(sel *entsql.Selector) {
			u := joinAuditUser(sel)
			day := dayBucket(sel.C(entauditlog.FieldCreatedAt))
			sel.Select(
				entsql.As(sel.C(entauditlog.FieldUserID), "user_id"),
				entsql.As("COALESCE("+u.C("username")+", '')", "username"),
				entsql.As("COUNT(*) FILTER (WHERE action IN ('query_execute', 'query_submit'))", "query_count"),
				entsql.As("COUNT(*) FILTER (WHERE action IN ('ticket_approve', 'ticket_reject'))", "approval_count"),
				entsql.As("COUNT(DISTINCT "+day+")", "active_days"),
				entsql.As("COUNT(*)", "total_actions"),
			).GroupBy(sel.C(entauditlog.FieldUserID), u.C("username")).
				OrderExpr(entsql.Expr("total_actions DESC")).
				Limit(topN)
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("查询活跃用户失败: %w", err)
	}
	return result, nil
}

func (s *ReportService) queryFrequencyDistribution(ctx context.Context, startDate time.Time, _ int, userID int64) ([]QueryFrequencyEntry, error) {
	var result []QueryFrequencyEntry
	err := s.auditScope(startDate, userID).
		Modify(func(sel *entsql.Selector) {
			day := dayBucket(sel.C(entauditlog.FieldCreatedAt))
			sel.Select(entsql.As(day, "period"), entsql.As("COUNT(*)", "count")).
				GroupBy(day).
				OrderExpr(entsql.Expr("period"))
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("查询频率分布失败: %w", err)
	}
	return result, nil
}

func (s *ReportService) queryActionTypeBreakdown(ctx context.Context, startDate time.Time, userID int64) ([]ActionTypeEntry, error) {
	var result []ActionTypeEntry
	err := s.auditScope(startDate, userID).
		Where(entauditlog.ActionNEQ("")).
		Modify(func(sel *entsql.Selector) {
			sel.Select(
				entsql.As(sel.C(entauditlog.FieldAction), "action"),
				entsql.As("COUNT(*)", "count"),
			).GroupBy(sel.C(entauditlog.FieldAction)).
				OrderExpr(entsql.Expr("count DESC"))
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("查询操作类型分布失败: %w", err)
	}
	return result, nil
}

// anomalyLimit caps each anomaly list.
const anomalyLimit = 20

// hourBucket renders a timestamp as YYYY-MM-DDTHH, the window burst detection
// groups by.
func hourBucket(column string) string {
	return "to_char(" + column + ", 'YYYY-MM-DD\"T\"HH24')"
}

// anomalyRow is the shared shape of both anomaly scans.
type anomalyRow struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Window   string `json:"window"`
	Count    int64  `json:"count"`
}

func (s *ReportService) detectAnomalousBehaviors(ctx context.Context, startDate time.Time, userID int64) ([]AnomalyEntry, error) {
	var result []AnomalyEntry

	// 1. Short-time burst: more than BurstQueryThreshold queries within one hour.
	var bursts []anomalyRow
	err := s.auditScope(startDate, userID).
		Where(entauditlog.ActionIn("query_execute", "query_submit")).
		Modify(func(sel *entsql.Selector) {
			hour := hourBucket(sel.C(entauditlog.FieldCreatedAt))
			u := joinAuditUser(sel)
			sel.Select(
				entsql.As(sel.C(entauditlog.FieldUserID), "user_id"),
				entsql.As("COALESCE("+u.C("username")+", '')", "username"),
				entsql.As(hour, "window"),
				entsql.As("COUNT(*)", "count"),
			).GroupBy(sel.C(entauditlog.FieldUserID), u.C("username"), hour).
				Having(entsql.ExprP(fmt.Sprintf("COUNT(*) > %d", BurstQueryThreshold))).
				OrderExpr(entsql.Expr("count DESC")).
				Limit(anomalyLimit)
		}).
		Scan(ctx, &bursts)
	if err != nil {
		return nil, fmt.Errorf("查询短时大量查询失败: %w", err)
	}
	for _, b := range bursts {
		result = append(result, AnomalyEntry{
			UserID:      b.UserID,
			Username:    b.Username,
			AnomalyType: "burst_queries",
			Description: fmt.Sprintf("1小时内执行 %d 次查询（阈值 %d）", b.Count, BurstQueryThreshold),
			Count:       b.Count,
			TimeWindow:  b.Window,
		})
	}

	// 2. Off-hours activity: 22:00-08:00.
	//
	// EXTRACT(HOUR FROM ...) rather than slicing text: created_at is a timestamp,
	// and the SQLite version's SUBSTR(created_at, 12, 2) only worked because it
	// was stored as an ISO string.
	var offHours []anomalyRow
	err = s.auditScope(startDate, userID).
		Modify(func(sel *entsql.Selector) {
			hour := "EXTRACT(HOUR FROM " + sel.C(entauditlog.FieldCreatedAt) + ")"
			u := joinAuditUser(sel)
			sel.Where(entsql.ExprP(hour+" >= 22 OR "+hour+" < 8")).
				Select(
					entsql.As(sel.C(entauditlog.FieldUserID), "user_id"),
					entsql.As("COALESCE("+u.C("username")+", '')", "username"),
					entsql.As("COUNT(*)", "count"),
				).GroupBy(sel.C(entauditlog.FieldUserID), u.C("username")).
				Having(entsql.ExprP(fmt.Sprintf("COUNT(*) > %d", OffHoursThreshold))).
				OrderExpr(entsql.Expr("count DESC")).
				Limit(anomalyLimit)
		}).
		Scan(ctx, &offHours)
	if err != nil {
		return nil, fmt.Errorf("查询非工作时间活动失败: %w", err)
	}
	for _, o := range offHours {
		result = append(result, AnomalyEntry{
			UserID:      o.UserID,
			Username:    o.Username,
			AnomalyType: "off_hours_activity",
			Description: fmt.Sprintf("非工作时间（22:00-08:00）执行 %d 次操作（阈值 %d）", o.Count, OffHoursThreshold),
			Count:       o.Count,
		})
	}

	return result, nil
}

// InvalidateAnalyticsCache clears all cached analytics results.
func InvalidateAnalyticsCache() {
	globalAnalyticsCache.mu.Lock()
	globalAnalyticsCache.entries = make(map[string]*analyticsCacheEntry)
	globalAnalyticsCache.mu.Unlock()
}
