package audit

import (
	"context"
	"fmt"
	"log"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entauditlog "github.com/whg517/sqlflow/internal/db/ent/auditlog"
	entticket "github.com/whg517/sqlflow/internal/db/ent/ticket"
)

// --- Report Data Types ---

// UsageStats represents aggregated usage statistics for the audit report.
type UsageStats struct {
	TotalActions int64             `json:"total_actions"`
	UniqueUsers  int64             `json:"unique_users"`
	UniqueIPs    int64             `json:"unique_ips"`
	TopUsers     []UserActionStat  `json:"top_users"`
	TopActions   []ActionStat      `json:"top_actions"`
	TopDatabases []DatabaseStat    `json:"top_databases"`
	DailyTrend   []DailyAuditTrend `json:"daily_trend"`
}

// ErrorStats represents error analysis for the audit report.
type ErrorStats struct {
	TotalErrors     int64              `json:"total_errors"`
	ErrorRate       float64            `json:"error_rate"`
	TopErrorTypes   []ErrorTypeStat    `json:"top_error_types"`
	RecentErrors    []RecentErrorEntry `json:"recent_errors"`
	DailyErrorTrend []DailyAuditTrend  `json:"daily_error_trend"`
}

// PerformanceReportStats represents performance metrics from audit logs.
type PerformanceReportStats struct {
	AvgExecutionMs  float64          `json:"avg_execution_ms"`
	MaxExecutionMs  int64            `json:"max_execution_ms"`
	P95ExecutionMs  int64            `json:"p95_execution_ms"`
	TotalResultRows int64            `json:"total_result_rows"`
	AffectedRows    int64            `json:"total_affected_rows"`
	DailyPerfTrend  []DailyPerfTrend `json:"daily_perf_trend"`
}

// TicketStats represents ticket workflow statistics.
type TicketStats struct {
	TotalTickets     int64              `json:"total_tickets"`
	PendingCount     int64              `json:"pending_count"`
	ApprovedCount    int64              `json:"approved_count"`
	RejectedCount    int64              `json:"rejected_count"`
	DoneCount        int64              `json:"done_count"`
	CancelledCount   int64              `json:"cancelled_count"`
	AvgApprovalTimeH float64            `json:"avg_approval_time_h"`
	DailyTicketTrend []DailyTicketTrend `json:"daily_ticket_trend"`
	RiskDistribution []RiskDistEntry    `json:"risk_distribution"`
}

// --- Composite Types ---

// UserActionStat represents per-user action counts.
type UserActionStat struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Count    int64  `json:"count"`
}

// ActionStat represents per-action-type counts.
type ActionStat struct {
	Action string `json:"action"`
	Count  int64  `json:"count"`
}

// DatabaseStat represents per-database action counts.
type DatabaseStat struct {
	Database string `json:"database"`
	Count    int64  `json:"count"`
}

// DailyAuditTrend represents per-day aggregated audit data.
type DailyAuditTrend struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// ErrorTypeStat represents error breakdown by type.
type ErrorTypeStat struct {
	Action string `json:"action"`
	Count  int64  `json:"count"`
}

// RecentErrorEntry represents a recent error from audit logs.
type RecentErrorEntry struct {
	ID           int64  `json:"id"`
	Action       string `json:"action"`
	Database     string `json:"database"`
	ErrorMessage string `json:"error_message"`
	Username     string `json:"username"`
	CreatedAt    string `json:"created_at"`
}

// DailyPerfTrend represents per-day performance metrics.
type DailyPerfTrend struct {
	Date       string  `json:"date"`
	AvgTimeMs  float64 `json:"avg_time_ms"`
	MaxTimeMs  int64   `json:"max_time_ms"`
	QueryCount int64   `json:"query_count"`
	ResultRows int64   `json:"result_rows"`
}

// DailyTicketTrend represents per-day ticket creation data.
type DailyTicketTrend struct {
	Date     string `json:"date"`
	Created  int64  `json:"created"`
	Approved int64  `json:"approved"`
	Rejected int64  `json:"rejected"`
}

// RiskDistEntry represents ticket distribution by risk level.
type RiskDistEntry struct {
	RiskLevel string `json:"risk_level"`
	Count     int64  `json:"count"`
}

// --- Service ---

// ReportService provides aggregated audit and ticket report data.
type ReportService struct {
	database *db.DB
	client   *ent.Client
}

// NewReportService creates a new ReportService.
func NewReportService(database *db.DB) *ReportService {
	return &ReportService{database: database, client: database.Client()}
}

// topN bounds every "top" list in the reports.
//
// A single constant because the UI renders these lists side by side; they
// drifted to different limits once before.
const topN = 10

// topByColumn groups by one column and returns the topN most frequent values,
// most frequent first.
//
// ent's typed GroupBy cannot order by an aggregate or limit the result, so this
// drops to the query builder — the escape hatch ADR-0010 allows, with the reason
// being that the typed API has no expression for it.
func topByColumn(column string) func(*entsql.Selector) {
	return func(sel *entsql.Selector) {
		sel.Select(sel.C(column), entsql.As("COUNT(*)", "count")).
			GroupBy(sel.C(column)).
			OrderExpr(entsql.Expr("count DESC")).
			Limit(topN)
	}
}

// dayBucket renders a timestamp column as a YYYY-MM-DD string.
//
// The SQLite-era code did SUBSTR(created_at, 1, 10) because ent stored
// timestamps as RFC3339Nano text that SQLite's DATE() could not parse. On
// PostgreSQL created_at is a real timestamp, so that workaround is not merely
// unnecessary — SUBSTR on a timestamp is a type error.
func dayBucket(column string) string {
	return "to_char(" + column + ", 'YYYY-MM-DD')"
}

// recentErrorLimit caps the recent-errors list on the error report.
const recentErrorLimit = 20

// scalarAgg evaluates a single aggregate expression over an audit-log query.
//
// The reports are mostly one-number answers — an average, a max, a sum — and
// ent's typed Aggregate cannot carry the COALESCE each of them needs to return
// 0 rather than NULL on an empty window. Expressing them through the modifier
// keeps them inside ent and dialect-aware.
func scalarAgg[T any](ctx context.Context, q *ent.AuditLogQuery, expr string) (T, error) {
	var out []struct {
		Value T `json:"value"`
	}
	var zero T
	err := q.Modify(func(sel *entsql.Selector) {
		sel.Select(entsql.As(expr, "value"))
	}).Scan(ctx, &out)
	if err != nil || len(out) == 0 {
		return zero, err
	}
	return out[0].Value, nil
}

// countDistinct counts distinct values of one column.
//
// ent's Count() has no distinct form, so this drops to the query builder's
// modifier — the escape hatch ADR-0010 allows, used here because the typed API
// cannot express COUNT(DISTINCT x) at all.
func countDistinct(ctx context.Context, q *ent.AuditLogQuery, column string) (int64, error) {
	var result []struct {
		Count int64 `json:"count"`
	}
	err := q.Modify(func(sel *entsql.Selector) {
		sel.Select(entsql.As("COUNT(DISTINCT "+sel.C(column)+")", "count"))
	}).Scan(ctx, &result)
	if err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return 0, nil
	}
	return result[0].Count, nil
}

// ReportParams holds the common filter parameters for all reports.
type ReportParams struct {
	Days int // number of days to look back, default 7
}

func (p ReportParams) normalizedDays() int {
	if p.Days <= 0 {
		return 7
	}
	if p.Days > 365 {
		return 365
	}
	return p.Days
}

// startDate is the inclusive lower bound of the report window.
//
// It returns a time.Time rather than a formatted string: created_at is a real
// timestamp column, and comparing it against text only ever worked because
// SQLite stored timestamps as text too.
func (p ReportParams) startDate() time.Time {
	days := p.normalizedDays()
	y, m, d := time.Now().AddDate(0, 0, -days).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// --- Usage Report ---

// GetUsageStats returns usage statistics for the given time range.
func (s *ReportService) GetUsageStats(ctx context.Context, params ReportParams) (*UsageStats, error) {
	startDate := params.startDate()

	stats := &UsageStats{}

	inWindow := s.client.AuditLog.Query().Where(entauditlog.CreatedAtGTE(startDate))

	total, err := inWindow.Clone().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("query total actions: %w", err)
	}
	stats.TotalActions = int64(total)

	stats.UniqueUsers, err = countDistinct(ctx, inWindow.Clone(), entauditlog.FieldUserID)
	if err != nil {
		return nil, fmt.Errorf("query unique users: %w", err)
	}

	stats.UniqueIPs, err = countDistinct(ctx,
		inWindow.Clone().Where(entauditlog.IPAddressNEQ("")), entauditlog.FieldIPAddress)
	if err != nil {
		return nil, fmt.Errorf("query unique ips: %w", err)
	}

	// Top 10 users by action count
	stats.TopUsers, err = s.queryTopUsers(ctx, startDate)
	if err != nil {
		return nil, err
	}

	// Top 10 action types
	stats.TopActions, err = s.queryTopActions(ctx, startDate)
	if err != nil {
		return nil, err
	}

	// Top 10 databases
	stats.TopDatabases, err = s.queryTopDatabases(ctx, startDate)
	if err != nil {
		return nil, err
	}

	// Daily trend
	stats.DailyTrend, err = s.queryDailyAuditTrend(ctx, startDate)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (s *ReportService) queryTopUsers(ctx context.Context, startDate time.Time) ([]UserActionStat, error) {
	var result []UserActionStat
	err := s.client.AuditLog.Query().
		Where(entauditlog.CreatedAtGTE(startDate)).
		Modify(func(sel *entsql.Selector) {
			u := joinAuditUser(sel)
			sel.Select(
				entsql.As(sel.C(entauditlog.FieldUserID), "user_id"),
				entsql.As("COALESCE("+u.C("username")+", '')", "username"),
				entsql.As("COUNT(*)", "count"),
			).GroupBy(sel.C(entauditlog.FieldUserID), u.C("username")).
				OrderExpr(entsql.Expr("count DESC")).
				Limit(topN)
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query top users: %w", err)
	}
	return result, nil
}

func (s *ReportService) queryTopActions(ctx context.Context, startDate time.Time) ([]ActionStat, error) {
	var result []ActionStat
	err := s.client.AuditLog.Query().
		Where(entauditlog.CreatedAtGTE(startDate), entauditlog.ActionNEQ("")).
		Modify(topByColumn(entauditlog.FieldAction)).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query top actions: %w", err)
	}
	return result, nil
}

func (s *ReportService) queryTopDatabases(ctx context.Context, startDate time.Time) ([]DatabaseStat, error) {
	var result []DatabaseStat
	err := s.client.AuditLog.Query().
		Where(entauditlog.CreatedAtGTE(startDate), entauditlog.DatabaseNEQ("")).
		Modify(topByColumn(entauditlog.FieldDatabase)).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query top databases: %w", err)
	}
	return result, nil
}

func (s *ReportService) queryDailyAuditTrend(ctx context.Context, startDate time.Time) ([]DailyAuditTrend, error) {
	var result []DailyAuditTrend
	err := s.client.AuditLog.Query().
		Where(entauditlog.CreatedAtGTE(startDate)).
		Modify(func(sel *entsql.Selector) {
			day := dayBucket(sel.C(entauditlog.FieldCreatedAt))
			sel.Select(entsql.As(day, "date"), entsql.As("COUNT(*)", "count")).
				GroupBy(day).
				OrderExpr(entsql.Expr("date"))
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query daily audit trend: %w", err)
	}
	return result, nil
}

// --- Error Report ---

// GetErrorStats returns error analysis for the given time range.
func (s *ReportService) GetErrorStats(ctx context.Context, params ReportParams) (*ErrorStats, error) {
	startDate := params.startDate()
	stats := &ErrorStats{}

	inWindow := s.client.AuditLog.Query().Where(entauditlog.CreatedAtGTE(startDate))

	errored, err := inWindow.Clone().Where(entauditlog.ErrorMessageNEQ("")).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("query total errors: %w", err)
	}
	stats.TotalErrors = int64(errored)

	totalActions, err := inWindow.Clone().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("query total actions for error rate: %w", err)
	}
	if totalActions > 0 {
		stats.ErrorRate = float64(stats.TotalErrors) / float64(totalActions) * 100
	}

	// Top error types (grouped by action)
	stats.TopErrorTypes, err = s.queryTopErrorTypes(ctx, startDate)
	if err != nil {
		return nil, err
	}

	// Recent errors (last 20)
	stats.RecentErrors, err = s.queryRecentErrors(ctx, startDate)
	if err != nil {
		return nil, err
	}

	// Daily error trend
	stats.DailyErrorTrend, err = s.queryDailyErrorTrend(ctx, startDate)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (s *ReportService) queryTopErrorTypes(ctx context.Context, startDate time.Time) ([]ErrorTypeStat, error) {
	var result []ErrorTypeStat
	err := s.client.AuditLog.Query().
		Where(entauditlog.CreatedAtGTE(startDate), entauditlog.ErrorMessageNEQ("")).
		Modify(topByColumn(entauditlog.FieldAction)).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query top error types: %w", err)
	}
	return result, nil
}

func (s *ReportService) queryRecentErrors(ctx context.Context, startDate time.Time) ([]RecentErrorEntry, error) {
	var result []RecentErrorEntry
	err := s.client.AuditLog.Query().
		Where(entauditlog.CreatedAtGTE(startDate), entauditlog.ErrorMessageNEQ("")).
		Modify(func(sel *entsql.Selector) {
			u := joinAuditUser(sel)
			// created_at is rendered as text because RecentErrorEntry carries it
			// as a string all the way to the API response.
			sel.Select(
				entsql.As(sel.C(entauditlog.FieldID), "id"),
				entsql.As(sel.C(entauditlog.FieldAction), "action"),
				entsql.As(sel.C(entauditlog.FieldDatabase), "database"),
				entsql.As(sel.C(entauditlog.FieldErrorMessage), "error_message"),
				entsql.As("COALESCE("+u.C("username")+", '')", "username"),
				entsql.As("to_char("+sel.C(entauditlog.FieldCreatedAt)+", 'YYYY-MM-DD HH24:MI:SS')", "created_at"),
			).OrderBy(entsql.Desc(sel.C(entauditlog.FieldCreatedAt))).
				Limit(recentErrorLimit)
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query recent errors: %w", err)
	}
	return result, nil
}

func (s *ReportService) queryDailyErrorTrend(ctx context.Context, startDate time.Time) ([]DailyAuditTrend, error) {
	var result []DailyAuditTrend
	err := s.client.AuditLog.Query().
		Where(entauditlog.CreatedAtGTE(startDate), entauditlog.ErrorMessageNEQ("")).
		Modify(func(sel *entsql.Selector) {
			day := dayBucket(sel.C(entauditlog.FieldCreatedAt))
			sel.Select(entsql.As(day, "date"), entsql.As("COUNT(*)", "count")).
				GroupBy(day).
				OrderExpr(entsql.Expr("date"))
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query daily error trend: %w", err)
	}
	return result, nil
}

// --- Performance Report ---

// GetPerformanceReport returns performance metrics from audit logs.
func (s *ReportService) GetPerformanceReport(ctx context.Context, params ReportParams) (*PerformanceReportStats, error) {
	startDate := params.startDate()
	stats := &PerformanceReportStats{}

	inWindow := s.client.AuditLog.Query().Where(entauditlog.CreatedAtGTE(startDate))
	timed := inWindow.Clone().Where(entauditlog.ExecutionTimeMsGT(0))

	var err error
	stats.AvgExecutionMs, err = scalarAgg[float64](ctx, timed.Clone(),
		"COALESCE(AVG(execution_time_ms), 0)::float8")
	if err != nil {
		return nil, fmt.Errorf("query avg execution time: %w", err)
	}

	stats.MaxExecutionMs, err = scalarAgg[int64](ctx, inWindow.Clone(),
		"COALESCE(MAX(execution_time_ms), 0)")
	if err != nil {
		return nil, fmt.Errorf("query max execution time: %w", err)
	}

	// percentile_cont rather than the previous ORDER BY ... LIMIT 1 OFFSET n:
	// that approximation needed a separate COUNT to compute the offset, guards
	// against a negative offset, and still landed on a neighboring row.
	// PostgreSQL computes the percentile directly.
	stats.P95ExecutionMs, err = scalarAgg[int64](ctx, timed.Clone(),
		"COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY execution_time_ms), 0)::bigint")
	if err != nil {
		// A percentile is a nice-to-have on a report; failing the whole report
		// over it would hide the numbers that did compute.
		log.Printf("P95 query failed: %v", err)
		stats.P95ExecutionMs = 0
	}

	stats.TotalResultRows, err = scalarAgg[int64](ctx, inWindow.Clone(),
		"COALESCE(SUM(result_rows), 0)")
	if err != nil {
		return nil, fmt.Errorf("query total result rows: %w", err)
	}

	stats.AffectedRows, err = scalarAgg[int64](ctx, inWindow.Clone(),
		"COALESCE(SUM(affected_rows), 0)")
	if err != nil {
		return nil, fmt.Errorf("query affected rows: %w", err)
	}

	// Daily performance trend
	stats.DailyPerfTrend, err = s.queryDailyPerfTrend(ctx, startDate)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (s *ReportService) queryDailyPerfTrend(ctx context.Context, startDate time.Time) ([]DailyPerfTrend, error) {
	var result []DailyPerfTrend
	err := s.client.AuditLog.Query().
		Where(entauditlog.CreatedAtGTE(startDate)).
		Modify(func(sel *entsql.Selector) {
			day := dayBucket(sel.C(entauditlog.FieldCreatedAt))
			sel.Select(
				entsql.As(day, "date"),
				entsql.As("COALESCE(AVG(execution_time_ms), 0)::float8", "avg_time_ms"),
				entsql.As("COALESCE(MAX(execution_time_ms), 0)", "max_time_ms"),
				entsql.As("COUNT(*)", "query_count"),
				entsql.As("COALESCE(SUM(result_rows), 0)", "result_rows"),
			).GroupBy(day).OrderExpr(entsql.Expr("date"))
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query daily perf trend: %w", err)
	}
	return result, nil
}

// --- Ticket Report ---

// GetTicketReport returns ticket workflow statistics.
func (s *ReportService) GetTicketReport(ctx context.Context, params ReportParams) (*TicketStats, error) {
	startDate := params.startDate()
	stats := &TicketStats{}

	// One grouped count instead of six separate ones: they only differed in the
	// status they filtered on, so six round trips were paying for one answer.
	var byStatus []struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	err := s.client.Ticket.Query().
		Where(entticket.CreatedAtGTE(startDate)).
		Modify(func(sel *entsql.Selector) {
			sel.Select(
				entsql.As(sel.C(entticket.FieldStatus), "status"),
				entsql.As("COUNT(*)", "count"),
			).GroupBy(sel.C(entticket.FieldStatus))
		}).
		Scan(ctx, &byStatus)
	if err != nil {
		return nil, fmt.Errorf("query ticket status counts: %w", err)
	}
	for _, r := range byStatus {
		stats.TotalTickets += r.Count
		switch r.Status {
		case "SUBMITTED", "PENDING_APPROVAL":
			stats.PendingCount += r.Count
		case "APPROVED":
			stats.ApprovedCount = r.Count
		case "REJECTED":
			stats.RejectedCount = r.Count
		case "DONE":
			stats.DoneCount = r.Count
		case "CANCELLED":
			stats.CancelledCount = r.Count
		}
	}

	// Average approval time (from created_at to updated_at for APPROVED/DONE tickets)
	// julianday() was SQLite's way to get a day-valued difference; PostgreSQL
	// subtracts timestamps directly and EXTRACT(EPOCH) turns the interval into
	// seconds.
	var avg []struct {
		Hours float64 `json:"hours"`
	}
	err = s.client.Ticket.Query().
		Where(
			entticket.CreatedAtGTE(startDate),
			entticket.StatusIn("APPROVED", "DONE", "REJECTED"),
		).
		Modify(func(sel *entsql.Selector) {
			sel.Select(entsql.As(
				"COALESCE(AVG(EXTRACT(EPOCH FROM (updated_at - created_at)) / 3600), 0)::float8", "hours"))
		}).
		Scan(ctx, &avg)
	if err != nil {
		return nil, fmt.Errorf("query avg approval time: %w", err)
	}
	if len(avg) > 0 {
		stats.AvgApprovalTimeH = avg[0].Hours
	}

	// Daily ticket trend
	stats.DailyTicketTrend, err = s.queryDailyTicketTrend(ctx, startDate)
	if err != nil {
		return nil, err
	}

	// Risk distribution
	stats.RiskDistribution, err = s.queryRiskDistribution(ctx, startDate)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (s *ReportService) queryDailyTicketTrend(ctx context.Context, startDate time.Time) ([]DailyTicketTrend, error) {
	var result []DailyTicketTrend
	err := s.client.Ticket.Query().
		Where(entticket.CreatedAtGTE(startDate)).
		Modify(func(sel *entsql.Selector) {
			day := dayBucket(sel.C(entticket.FieldCreatedAt))
			sel.Select(
				entsql.As(day, "date"),
				entsql.As("COUNT(*)", "created"),
				entsql.As("COUNT(*) FILTER (WHERE status IN ('APPROVED', 'DONE'))", "approved"),
				entsql.As("COUNT(*) FILTER (WHERE status = 'REJECTED')", "rejected"),
			).GroupBy(day).OrderExpr(entsql.Expr("date"))
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query daily ticket trend: %w", err)
	}
	return result, nil
}

func (s *ReportService) queryRiskDistribution(ctx context.Context, startDate time.Time) ([]RiskDistEntry, error) {
	var result []RiskDistEntry
	err := s.client.Ticket.Query().
		Where(entticket.CreatedAtGTE(startDate), entticket.RiskLevelNEQ("")).
		Modify(func(sel *entsql.Selector) {
			sel.Select(
				entsql.As(sel.C(entticket.FieldRiskLevel), "risk_level"),
				entsql.As("COUNT(*)", "count"),
			).GroupBy(sel.C(entticket.FieldRiskLevel)).
				OrderExpr(entsql.Expr("count DESC"))
		}).
		Scan(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("query risk distribution: %w", err)
	}
	return result, nil
}
