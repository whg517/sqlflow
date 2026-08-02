package audit

import (
	"context"
	"database/sql"
	"errors"
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
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT a.user_id, COALESCE(u.username, ''), COUNT(*) as count
		 FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id
		 WHERE a.created_at >= $1
		 GROUP BY a.user_id, u.username ORDER BY count DESC LIMIT 10`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query top users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []UserActionStat
	for rows.Next() {
		var u UserActionStat
		if err := rows.Scan(&u.UserID, &u.Username, &u.Count); err != nil {
			return nil, fmt.Errorf("scan top user: %w", err)
		}
		result = append(result, u)
	}
	return result, rows.Err()
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

	// Total errors (audit logs with non-empty error_message)
	err := s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE created_at >= $1 AND error_message != ''`, startDate,
	).Scan(&stats.TotalErrors)
	if err != nil {
		return nil, fmt.Errorf("query total errors: %w", err)
	}

	// Total actions for error rate
	var totalActions int64
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE created_at >= $1`, startDate,
	).Scan(&totalActions)
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
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT action, COUNT(*) as count FROM audit_logs WHERE created_at >= $1 AND error_message != '' GROUP BY action ORDER BY count DESC LIMIT 10`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query top error types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []ErrorTypeStat
	for rows.Next() {
		var e ErrorTypeStat
		if err := rows.Scan(&e.Action, &e.Count); err != nil {
			return nil, fmt.Errorf("scan top error type: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *ReportService) queryRecentErrors(ctx context.Context, startDate time.Time) ([]RecentErrorEntry, error) {
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT a.id, a.action, a.database, a.error_message, COALESCE(u.username, ''), a.created_at
		 FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id
		 WHERE a.created_at >= $1 AND a.error_message != ''
		 ORDER BY a.created_at DESC LIMIT 20`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query recent errors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []RecentErrorEntry
	for rows.Next() {
		var e RecentErrorEntry
		if err := rows.Scan(&e.ID, &e.Action, &e.Database, &e.ErrorMessage, &e.Username, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan recent error: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
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

	// Average execution time
	var avgMs sql.NullFloat64
	err := s.database.DB.QueryRowContext(ctx,
		`SELECT CAST(COALESCE(AVG(execution_time_ms), 0) AS REAL) FROM audit_logs WHERE created_at >= $1 AND execution_time_ms > 0`, startDate,
	).Scan(&avgMs)
	if err != nil {
		return nil, fmt.Errorf("query avg execution time: %w", err)
	}
	stats.AvgExecutionMs = avgMs.Float64

	// Max execution time
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(execution_time_ms), 0) FROM audit_logs WHERE created_at >= $1`, startDate,
	).Scan(&stats.MaxExecutionMs)
	if err != nil {
		return nil, fmt.Errorf("query max execution time: %w", err)
	}

	// P95 execution time (approximate via subquery with LIMIT)
	// First count qualifying rows to guard against negative OFFSET
	var p95Count int64
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE created_at >= $1 AND execution_time_ms > 0`, startDate,
	).Scan(&p95Count)
	if err != nil {
		log.Printf("P95 count query failed: %v", err)
		stats.P95ExecutionMs = 0
	} else if p95Count < 2 {
		// Not enough data for a meaningful percentile
		stats.P95ExecutionMs = 0
	} else {
		offset := p95Count*95/100 - 1
		if offset < 0 {
			offset = 0
		}
		err = s.database.DB.QueryRowContext(ctx,
			`SELECT execution_time_ms FROM audit_logs WHERE created_at >= $1 AND execution_time_ms > 0 ORDER BY execution_time_ms ASC LIMIT 1 OFFSET $2`, startDate, offset,
		).Scan(&stats.P95ExecutionMs)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				log.Printf("P95 query failed: %v", err)
			}
			stats.P95ExecutionMs = 0
		}
	}

	// Total result rows
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(result_rows), 0) FROM audit_logs WHERE created_at >= $1`, startDate,
	).Scan(&stats.TotalResultRows)
	if err != nil {
		return nil, fmt.Errorf("query total result rows: %w", err)
	}

	// Total affected rows
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(affected_rows), 0) FROM audit_logs WHERE created_at >= $1`, startDate,
	).Scan(&stats.AffectedRows)
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

	// Status counts
	err := s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= $1`, startDate,
	).Scan(&stats.TotalTickets)
	if err != nil {
		return nil, fmt.Errorf("query total tickets: %w", err)
	}

	// Pending (SUBMITTED + AI_REVIEWED + PENDING_APPROVAL)
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= $1 AND status IN ('SUBMITTED', 'AI_REVIEWED', 'PENDING_APPROVAL')`, startDate,
	).Scan(&stats.PendingCount)
	if err != nil {
		return nil, fmt.Errorf("query pending tickets: %w", err)
	}

	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= $1 AND status = 'APPROVED'`, startDate,
	).Scan(&stats.ApprovedCount)
	if err != nil {
		return nil, fmt.Errorf("query approved tickets: %w", err)
	}

	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= $1 AND status = 'REJECTED'`, startDate,
	).Scan(&stats.RejectedCount)
	if err != nil {
		return nil, fmt.Errorf("query rejected tickets: %w", err)
	}

	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= $1 AND status = 'DONE'`, startDate,
	).Scan(&stats.DoneCount)
	if err != nil {
		return nil, fmt.Errorf("query done tickets: %w", err)
	}

	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= $1 AND status = 'CANCELLED'`, startDate,
	).Scan(&stats.CancelledCount)
	if err != nil {
		return nil, fmt.Errorf("query cancelled tickets: %w", err)
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
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT risk_level, COUNT(*) as count FROM tickets WHERE created_at >= $1 AND risk_level != '' GROUP BY risk_level ORDER BY count DESC`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query risk distribution: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []RiskDistEntry
	for rows.Next() {
		var r RiskDistEntry
		if err := rows.Scan(&r.RiskLevel, &r.Count); err != nil {
			return nil, fmt.Errorf("scan risk distribution: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
