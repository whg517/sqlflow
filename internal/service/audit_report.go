package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"
)

// --- Report Data Types ---

// UsageStats represents aggregated usage statistics for the audit report.
type UsageStats struct {
	TotalActions    int64           `json:"total_actions"`
	UniqueUsers     int64           `json:"unique_users"`
	UniqueIPs       int64           `json:"unique_ips"`
	TopUsers        []UserActionStat `json:"top_users"`
	TopActions      []ActionStat    `json:"top_actions"`
	TopDatabases    []DatabaseStat  `json:"top_databases"`
	DailyTrend      []DailyAuditTrend `json:"daily_trend"`
}

// ErrorStats represents error analysis for the audit report.
type ErrorStats struct {
	TotalErrors     int64         `json:"total_errors"`
	ErrorRate       float64       `json:"error_rate"`
	TopErrorTypes   []ErrorTypeStat `json:"top_error_types"`
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
	TotalTickets      int64              `json:"total_tickets"`
	PendingCount      int64              `json:"pending_count"`
	ApprovedCount     int64              `json:"approved_count"`
	RejectedCount     int64              `json:"rejected_count"`
	DoneCount         int64              `json:"done_count"`
	CancelledCount    int64              `json:"cancelled_count"`
	AvgApprovalTimeH  float64            `json:"avg_approval_time_h"`
	DailyTicketTrend  []DailyTicketTrend `json:"daily_ticket_trend"`
	RiskDistribution  []RiskDistEntry    `json:"risk_distribution"`
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
	Date         string  `json:"date"`
	AvgTimeMs    float64 `json:"avg_time_ms"`
	MaxTimeMs    int64   `json:"max_time_ms"`
	QueryCount   int64   `json:"query_count"`
	ResultRows   int64   `json:"result_rows"`
}

// DailyTicketTrend represents per-day ticket creation data.
type DailyTicketTrend struct {
	Date    string `json:"date"`
	Created int64  `json:"created"`
	Approved int64 `json:"approved"`
	Rejected int64 `json:"rejected"`
}

// RiskDistEntry represents ticket distribution by risk level.
type RiskDistEntry struct {
	RiskLevel string `json:"risk_level"`
	Count     int64  `json:"count"`
}

// --- Service ---

// AuditReportService provides aggregated audit and ticket report data.
type AuditReportService struct {
	db *sql.DB
}

// NewAuditReportService creates a new AuditReportService.
func NewAuditReportService(db *sql.DB) *AuditReportService {
	return &AuditReportService{db: db}
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

func (p ReportParams) startDate() string {
	days := p.normalizedDays()
	return time.Now().AddDate(0, 0, -days).Format("2006-01-02") + " 00:00:00"
}

// --- Usage Report ---

// GetUsageStats returns usage statistics for the given time range.
func (s *AuditReportService) GetUsageStats(ctx context.Context, params ReportParams) (*UsageStats, error) {
	startDate := params.startDate()

	stats := &UsageStats{}

	// Total actions
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE created_at >= ?`, startDate,
	).Scan(&stats.TotalActions)
	if err != nil {
		return nil, fmt.Errorf("query total actions: %w", err)
	}

	// Unique users
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT user_id) FROM audit_logs WHERE created_at >= ?`, startDate,
	).Scan(&stats.UniqueUsers)
	if err != nil {
		return nil, fmt.Errorf("query unique users: %w", err)
	}

	// Unique IPs
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT ip_address) FROM audit_logs WHERE created_at >= ? AND ip_address != ''`, startDate,
	).Scan(&stats.UniqueIPs)
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

func (s *AuditReportService) queryTopUsers(ctx context.Context, startDate string) ([]UserActionStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.user_id, COALESCE(u.username, ''), COUNT(*) as count
		 FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id
		 WHERE a.created_at >= ?
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

func (s *AuditReportService) queryTopActions(ctx context.Context, startDate string) ([]ActionStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT action, COUNT(*) as count FROM audit_logs WHERE created_at >= ? AND action != '' GROUP BY action ORDER BY count DESC LIMIT 10`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query top actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []ActionStat
	for rows.Next() {
		var a ActionStat
		if err := rows.Scan(&a.Action, &a.Count); err != nil {
			return nil, fmt.Errorf("scan top action: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *AuditReportService) queryTopDatabases(ctx context.Context, startDate string) ([]DatabaseStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT database, COUNT(*) as count FROM audit_logs WHERE created_at >= ? AND database != '' GROUP BY database ORDER BY count DESC LIMIT 10`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query top databases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []DatabaseStat
	for rows.Next() {
		var d DatabaseStat
		if err := rows.Scan(&d.Database, &d.Count); err != nil {
			return nil, fmt.Errorf("scan top database: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *AuditReportService) queryDailyAuditTrend(ctx context.Context, startDate string) ([]DailyAuditTrend, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DATE(created_at) as date, COUNT(*) as count FROM audit_logs WHERE created_at >= ? GROUP BY DATE(created_at) ORDER BY date`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query daily audit trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []DailyAuditTrend
	for rows.Next() {
		var d DailyAuditTrend
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			return nil, fmt.Errorf("scan daily audit trend: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// --- Error Report ---

// GetErrorStats returns error analysis for the given time range.
func (s *AuditReportService) GetErrorStats(ctx context.Context, params ReportParams) (*ErrorStats, error) {
	startDate := params.startDate()
	stats := &ErrorStats{}

	// Total errors (audit logs with non-empty error_message)
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE created_at >= ? AND error_message != ''`, startDate,
	).Scan(&stats.TotalErrors)
	if err != nil {
		return nil, fmt.Errorf("query total errors: %w", err)
	}

	// Total actions for error rate
	var totalActions int64
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE created_at >= ?`, startDate,
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

func (s *AuditReportService) queryTopErrorTypes(ctx context.Context, startDate string) ([]ErrorTypeStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT action, COUNT(*) as count FROM audit_logs WHERE created_at >= ? AND error_message != '' GROUP BY action ORDER BY count DESC LIMIT 10`, startDate)
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

func (s *AuditReportService) queryRecentErrors(ctx context.Context, startDate string) ([]RecentErrorEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.action, a.database, a.error_message, COALESCE(u.username, ''), a.created_at
		 FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id
		 WHERE a.created_at >= ? AND a.error_message != ''
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

func (s *AuditReportService) queryDailyErrorTrend(ctx context.Context, startDate string) ([]DailyAuditTrend, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DATE(created_at) as date, COUNT(*) as count FROM audit_logs WHERE created_at >= ? AND error_message != '' GROUP BY DATE(created_at) ORDER BY date`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query daily error trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []DailyAuditTrend
	for rows.Next() {
		var d DailyAuditTrend
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			return nil, fmt.Errorf("scan daily error trend: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// --- Performance Report ---

// GetPerformanceReport returns performance metrics from audit logs.
func (s *AuditReportService) GetPerformanceReport(ctx context.Context, params ReportParams) (*PerformanceReportStats, error) {
	startDate := params.startDate()
	stats := &PerformanceReportStats{}

	// Average execution time
	var avgMs sql.NullFloat64
	err := s.db.QueryRowContext(ctx,
		`SELECT CAST(COALESCE(AVG(execution_time_ms), 0) AS REAL) FROM audit_logs WHERE created_at >= ? AND execution_time_ms > 0`, startDate,
	).Scan(&avgMs)
	if err != nil {
		return nil, fmt.Errorf("query avg execution time: %w", err)
	}
	stats.AvgExecutionMs = avgMs.Float64

	// Max execution time
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(execution_time_ms), 0) FROM audit_logs WHERE created_at >= ?`, startDate,
	).Scan(&stats.MaxExecutionMs)
	if err != nil {
		return nil, fmt.Errorf("query max execution time: %w", err)
	}

	// P95 execution time (approximate via subquery with LIMIT)
	err = s.db.QueryRowContext(ctx,
		`SELECT execution_time_ms FROM audit_logs WHERE created_at >= ? AND execution_time_ms > 0 ORDER BY execution_time_ms ASC LIMIT 1 OFFSET (SELECT COUNT(*) FROM audit_logs WHERE created_at >= ? AND execution_time_ms > 0) * 95 / 100 - 1`, startDate, startDate,
	).Scan(&stats.P95ExecutionMs)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("P95 query failed: %v", err)
		}
		stats.P95ExecutionMs = 0
	}

	// Total result rows
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(result_rows), 0) FROM audit_logs WHERE created_at >= ?`, startDate,
	).Scan(&stats.TotalResultRows)
	if err != nil {
		return nil, fmt.Errorf("query total result rows: %w", err)
	}

	// Total affected rows
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(affected_rows), 0) FROM audit_logs WHERE created_at >= ?`, startDate,
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

func (s *AuditReportService) queryDailyPerfTrend(ctx context.Context, startDate string) ([]DailyPerfTrend, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DATE(created_at) as date,
		        CAST(COALESCE(AVG(execution_time_ms), 0) AS REAL) as avg_time_ms,
		        COALESCE(MAX(execution_time_ms), 0) as max_time_ms,
		        COUNT(*) as query_count,
		        COALESCE(SUM(result_rows), 0) as result_rows
		 FROM audit_logs WHERE created_at >= ?
		 GROUP BY DATE(created_at) ORDER BY date`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query daily perf trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []DailyPerfTrend
	for rows.Next() {
		var d DailyPerfTrend
		if err := rows.Scan(&d.Date, &d.AvgTimeMs, &d.MaxTimeMs, &d.QueryCount, &d.ResultRows); err != nil {
			return nil, fmt.Errorf("scan daily perf trend: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// --- Ticket Report ---

// GetTicketReport returns ticket workflow statistics.
func (s *AuditReportService) GetTicketReport(ctx context.Context, params ReportParams) (*TicketStats, error) {
	startDate := params.startDate()
	stats := &TicketStats{}

	// Status counts
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= ?`, startDate,
	).Scan(&stats.TotalTickets)
	if err != nil {
		return nil, fmt.Errorf("query total tickets: %w", err)
	}

	// Pending (SUBMITTED + AI_REVIEWED + PENDING_APPROVAL)
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= ? AND status IN ('SUBMITTED', 'AI_REVIEWED', 'PENDING_APPROVAL')`, startDate,
	).Scan(&stats.PendingCount)
	if err != nil {
		return nil, fmt.Errorf("query pending tickets: %w", err)
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= ? AND status = 'APPROVED'`, startDate,
	).Scan(&stats.ApprovedCount)
	if err != nil {
		return nil, fmt.Errorf("query approved tickets: %w", err)
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= ? AND status = 'REJECTED'`, startDate,
	).Scan(&stats.RejectedCount)
	if err != nil {
		return nil, fmt.Errorf("query rejected tickets: %w", err)
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= ? AND status = 'DONE'`, startDate,
	).Scan(&stats.DoneCount)
	if err != nil {
		return nil, fmt.Errorf("query done tickets: %w", err)
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE created_at >= ? AND status = 'CANCELLED'`, startDate,
	).Scan(&stats.CancelledCount)
	if err != nil {
		return nil, fmt.Errorf("query cancelled tickets: %w", err)
	}

	// Average approval time (from created_at to updated_at for APPROVED/DONE tickets)
	var avgApprovalH sql.NullFloat64
	err = s.db.QueryRowContext(ctx,
		`SELECT CAST(COALESCE(AVG((julianday(updated_at) - julianday(created_at)) * 24), 0) AS REAL)
		 FROM tickets WHERE created_at >= ? AND status IN ('APPROVED', 'DONE', 'REJECTED')`, startDate,
	).Scan(&avgApprovalH)
	if err != nil {
		return nil, fmt.Errorf("query avg approval time: %w", err)
	}
	stats.AvgApprovalTimeH = avgApprovalH.Float64

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

func (s *AuditReportService) queryDailyTicketTrend(ctx context.Context, startDate string) ([]DailyTicketTrend, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DATE(created_at) as date,
		        SUM(CASE WHEN 1=1 THEN 1 ELSE 0 END) as created,
		        SUM(CASE WHEN status IN ('APPROVED', 'DONE') THEN 1 ELSE 0 END) as approved,
		        SUM(CASE WHEN status = 'REJECTED' THEN 1 ELSE 0 END) as rejected
		 FROM tickets WHERE created_at >= ?
		 GROUP BY DATE(created_at) ORDER BY date`, startDate)
	if err != nil {
		return nil, fmt.Errorf("query daily ticket trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []DailyTicketTrend
	for rows.Next() {
		var d DailyTicketTrend
		if err := rows.Scan(&d.Date, &d.Created, &d.Approved, &d.Rejected); err != nil {
			return nil, fmt.Errorf("scan daily ticket trend: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *AuditReportService) queryRiskDistribution(ctx context.Context, startDate string) ([]RiskDistEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT risk_level, COUNT(*) as count FROM tickets WHERE created_at >= ? AND risk_level != '' GROUP BY risk_level ORDER BY count DESC`, startDate)
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
