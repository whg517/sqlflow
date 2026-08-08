package audit

import (
	"context"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/testutil"
)

func TestAuditReportService_GetUsageStats(t *testing.T) {
	db := testutil.NewDB(t).DB
	svc := NewReportService(testutil.WrapSQL(t, db))
	ctx := context.Background()

	// Seed audit logs
	userID := testutil.CreateUser(t, db, "report_usage")
	now := time.Now().Format("2006-01-02 15:04:05")
	for i := 0; i < 5; i++ {
		_, err := db.Exec(`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, execution_time_ms, result_rows, ip_address, created_at) VALUES ($1, 'SELECT', 1, 'testdb', 'SELECT 1', 'SELECT', 10, 1, '192.168.1.1', $2)`,
			userID, now)
		if err != nil {
			t.Fatalf("seed audit log: %v", err)
		}
	}
	// Add a different user
	userID2 := testutil.CreateUser(t, db, "report_usage2")
	_, err := db.Exec(`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, execution_time_ms, result_rows, ip_address, created_at) VALUES ($1, 'UPDATE', 1, 'otherdb', 'UPDATE x SET y=1', 'UPDATE', 20, 0, '10.0.0.1', $2)`,
		userID2, now)
	if err != nil {
		t.Fatalf("seed audit log 2: %v", err)
	}

	stats, err := svc.GetUsageStats(ctx, ReportParams{Days: 7})
	if err != nil {
		t.Fatalf("GetUsageStats failed: %v", err)
	}

	if stats.TotalActions != 6 {
		t.Errorf("expected 6 total actions, got %d", stats.TotalActions)
	}
	if stats.UniqueUsers != 2 {
		t.Errorf("expected 2 unique users, got %d", stats.UniqueUsers)
	}
	if stats.UniqueIPs != 2 {
		t.Errorf("expected 2 unique IPs, got %d", stats.UniqueIPs)
	}
	if len(stats.TopActions) != 2 {
		t.Errorf("expected 2 top actions, got %d", len(stats.TopActions))
	}
	if len(stats.DailyTrend) != 1 {
		t.Errorf("expected 1 daily trend entry, got %d", len(stats.DailyTrend))
	}
}

func TestAuditReportService_GetErrorStats(t *testing.T) {
	db := testutil.NewDB(t).DB
	svc := NewReportService(testutil.WrapSQL(t, db))
	ctx := context.Background()

	userID := testutil.CreateUser(t, db, "report_error")
	now := time.Now().Format("2006-01-02 15:04:05")

	// Insert 2 errors
	for i := 0; i < 2; i++ {
		_, err := db.Exec(`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, execution_time_ms, error_message, ip_address, created_at) VALUES ($1, 'SELECT', 1, 'testdb', 'SELECT 1', 'SELECT', 10, 'connection refused', '127.0.0.1', $2)`,
			userID, now)
		if err != nil {
			t.Fatalf("seed error log: %v", err)
		}
	}
	// Insert 1 success
	_, err := db.Exec(`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, execution_time_ms, result_rows, ip_address, created_at) VALUES ($1, 'SELECT', 1, 'testdb', 'SELECT 1', 'SELECT', 10, 1, '127.0.0.1', $2)`,
		userID, now)
	if err != nil {
		t.Fatalf("seed success log: %v", err)
	}

	stats, err := svc.GetErrorStats(ctx, ReportParams{Days: 7})
	if err != nil {
		t.Fatalf("GetErrorStats failed: %v", err)
	}

	if stats.TotalErrors != 2 {
		t.Errorf("expected 2 total errors, got %d", stats.TotalErrors)
	}
	// 2 errors / 3 total = 66.67%
	if stats.ErrorRate < 60 || stats.ErrorRate > 67 {
		t.Errorf("expected error rate ~66.67%%, got %.2f%%", stats.ErrorRate)
	}
	if len(stats.RecentErrors) != 2 {
		t.Errorf("expected 2 recent errors, got %d", len(stats.RecentErrors))
	}
}

func TestAuditReportService_GetPerformanceReport(t *testing.T) {
	db := testutil.NewDB(t).DB
	svc := NewReportService(testutil.WrapSQL(t, db))
	ctx := context.Background()

	userID := testutil.CreateUser(t, db, "report_perf")
	now := time.Now().Format("2006-01-02 15:04:05")

	// Insert audit logs with varying execution times
	times := []int64{10, 50, 100, 200, 500}
	for _, ms := range times {
		_, err := db.Exec(`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, execution_time_ms, result_rows, affected_rows, ip_address, created_at) VALUES ($1, 'SELECT', 1, 'testdb', 'SELECT 1', 'SELECT', $2, 100, 0, '127.0.0.1', $3)`,
			userID, ms, now)
		if err != nil {
			t.Fatalf("seed perf log: %v", err)
		}
	}

	stats, err := svc.GetPerformanceReport(ctx, ReportParams{Days: 7})
	if err != nil {
		t.Fatalf("GetPerformanceReport failed: %v", err)
	}

	expectedAvg := float64(10+50+100+200+500) / float64(5) // 172
	if stats.AvgExecutionMs < expectedAvg-1 || stats.AvgExecutionMs > expectedAvg+1 {
		t.Errorf("expected avg ~172ms, got %.2fms", stats.AvgExecutionMs)
	}
	if stats.MaxExecutionMs != 500 {
		t.Errorf("expected max 500ms, got %dms", stats.MaxExecutionMs)
	}
	if stats.TotalResultRows != 500 {
		t.Errorf("expected 500 total result rows, got %d", stats.TotalResultRows)
	}
	if len(stats.DailyPerfTrend) != 1 {
		t.Errorf("expected 1 daily trend entry, got %d", len(stats.DailyPerfTrend))
	}
}

func TestAuditReportService_GetTicketReport(t *testing.T) {
	db := testutil.NewDB(t).DB
	svc := NewReportService(testutil.WrapSQL(t, db))
	ctx := context.Background()

	userID := testutil.CreateUser(t, db, "report_ticket")
	now := time.Now().Format("2006-01-02 15:04:05")

	// Insert tickets with various statuses
	statuses := []struct {
		status    string
		riskLevel string
	}{
		{"APPROVED", "low"},
		{"APPROVED", "low"},
		{"REJECTED", "high"},
		{"DONE", "medium"},
		{"PENDING_APPROVAL", ""},
		{"SUBMITTED", ""},
	}
	for _, tc := range statuses {
		_, err := db.Exec(`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, status, risk_level, created_at, updated_at) VALUES ($1, 1, 'testdb', 'SELECT 1', 'SELECT', 'mysql', $2, $3, $4, $5)`,
			userID, tc.status, tc.riskLevel, now, now)
		if err != nil {
			t.Fatalf("seed ticket: %v", err)
		}
	}

	stats, err := svc.GetTicketReport(ctx, ReportParams{Days: 7})
	if err != nil {
		t.Fatalf("GetTicketReport failed: %v", err)
	}

	if stats.TotalTickets != 6 {
		t.Errorf("expected 6 total tickets, got %d", stats.TotalTickets)
	}
	if stats.PendingCount != 2 {
		t.Errorf("expected 2 pending tickets, got %d", stats.PendingCount)
	}
	if stats.ApprovedCount != 2 {
		t.Errorf("expected 2 approved tickets, got %d", stats.ApprovedCount)
	}
	if stats.RejectedCount != 1 {
		t.Errorf("expected 1 rejected ticket, got %d", stats.RejectedCount)
	}
	if stats.DoneCount != 1 {
		t.Errorf("expected 1 done ticket, got %d", stats.DoneCount)
	}
	if len(stats.RiskDistribution) != 3 {
		t.Errorf("expected 3 risk levels, got %d", len(stats.RiskDistribution))
	}
}

func TestAuditReportService_DefaultDays(t *testing.T) {
	db := testutil.NewDB(t).DB
	svc := NewReportService(testutil.WrapSQL(t, db))
	ctx := context.Background()

	// Should not fail with days=0 (defaults to 7)
	_, err := svc.GetUsageStats(ctx, ReportParams{Days: 0})
	if err != nil {
		t.Fatalf("GetUsageStats with days=0 failed: %v", err)
	}

	_, err = svc.GetErrorStats(ctx, ReportParams{})
	if err != nil {
		t.Fatalf("GetErrorStats with empty params failed: %v", err)
	}

	_, err = svc.GetPerformanceReport(ctx, ReportParams{Days: -1})
	if err != nil {
		t.Fatalf("GetPerformanceReport with negative days failed: %v", err)
	}

	_, err = svc.GetTicketReport(ctx, ReportParams{Days: 30})
	if err != nil {
		t.Fatalf("GetTicketReport with days=30 failed: %v", err)
	}
}

func TestAuditReportService_EmptyData(t *testing.T) {
	db := testutil.NewDB(t).DB
	svc := NewReportService(testutil.WrapSQL(t, db))
	ctx := context.Background()

	// Should return zero-value stats without error
	stats, err := svc.GetUsageStats(ctx, ReportParams{Days: 1})
	if err != nil {
		t.Fatalf("GetUsageStats empty failed: %v", err)
	}
	if stats.TotalActions != 0 {
		t.Errorf("expected 0 total actions for empty DB, got %d", stats.TotalActions)
	}

	errStats, err := svc.GetErrorStats(ctx, ReportParams{Days: 1})
	if err != nil {
		t.Fatalf("GetErrorStats empty failed: %v", err)
	}
	if errStats.TotalErrors != 0 {
		t.Errorf("expected 0 errors for empty DB, got %d", errStats.TotalErrors)
	}

	perfStats, err := svc.GetPerformanceReport(ctx, ReportParams{Days: 1})
	if err != nil {
		t.Fatalf("GetPerformanceReport empty failed: %v", err)
	}
	if perfStats.AvgExecutionMs != 0 {
		t.Errorf("expected 0 avg ms for empty DB, got %.2f", perfStats.AvgExecutionMs)
	}

	ticketStats, err := svc.GetTicketReport(ctx, ReportParams{Days: 1})
	if err != nil {
		t.Fatalf("GetTicketReport empty failed: %v", err)
	}
	if ticketStats.TotalTickets != 0 {
		t.Errorf("expected 0 tickets for empty DB, got %d", ticketStats.TotalTickets)
	}
}

// TestReportAggregates_ExactValues pins the numbers the reporting queries
// produce.
//
// The conversion to ent rewrote every aggregate in this file, including two
// whose semantics changed: P95 moved from an ORDER BY/OFFSET approximation to
// percentile_cont, and the six per-status ticket counts collapsed into one
// grouped query. Asserting on shapes and non-zero-ness would not have caught a
// mis-mapped status or an off-by-one percentile.
func TestReportAggregates_ExactValues(t *testing.T) {
	d := testutil.NewDB(t)
	svc := NewReportService(d)
	ctx := context.Background()

	alice := testutil.SeedUser(t, d.DB, "agg_alice", "developer")
	bob := testutil.SeedUser(t, d.DB, "agg_bob", "dba")

	// Execution times 100..1000 so the 95th percentile is unambiguous.
	for i := 1; i <= 10; i++ {
		owner := alice
		if i > 6 {
			owner = bob
		}
		if _, err := d.Exec(
			`INSERT INTO audit_logs (user_id, action, database, sql_content, sql_summary,
			                         execution_time_ms, result_rows, affected_rows, ip_address, created_at)
			 VALUES ($1, 'query_execute', 'appdb', 'SELECT 1', 'SELECT 1', $2, $3, 1, '10.0.0.1', now())`,
			owner, int64(i*100), int64(i),
		); err != nil {
			t.Fatalf("seed audit log %d: %v", i, err)
		}
	}

	perf, err := svc.GetPerformanceReport(ctx, ReportParams{Days: 7})
	if err != nil {
		t.Fatalf("GetPerformanceReport: %v", err)
	}
	if got, want := perf.AvgExecutionMs, 550.0; got != want {
		t.Errorf("AvgExecutionMs = %v, want %v", got, want)
	}
	if got, want := perf.MaxExecutionMs, int64(1000); got != want {
		t.Errorf("MaxExecutionMs = %d, want %d", got, want)
	}
	// percentile_cont interpolates: the 95th percentile of 100..1000 is 955.
	if got, want := perf.P95ExecutionMs, int64(955); got != want {
		t.Errorf("P95ExecutionMs = %d, want %d", got, want)
	}
	if got, want := perf.TotalResultRows, int64(55); got != want {
		t.Errorf("TotalResultRows = %d, want %d", got, want)
	}
	if got, want := perf.AffectedRows, int64(10); got != want {
		t.Errorf("AffectedRows = %d, want %d", got, want)
	}

	usage, err := svc.GetUsageStats(ctx, ReportParams{Days: 7})
	if err != nil {
		t.Fatalf("GetUsageStats: %v", err)
	}
	if got, want := usage.TotalActions, int64(10); got != want {
		t.Errorf("TotalActions = %d, want %d", got, want)
	}
	if got, want := usage.UniqueUsers, int64(2); got != want {
		t.Errorf("UniqueUsers = %d, want %d", got, want)
	}
	if got, want := usage.UniqueIPs, int64(1); got != want {
		t.Errorf("UniqueIPs = %d, want %d", got, want)
	}
	// The username comes from the join, which is the part most likely to break.
	if len(usage.TopUsers) != 2 {
		t.Fatalf("TopUsers = %d entries, want 2", len(usage.TopUsers))
	}
	if usage.TopUsers[0].Username != "agg_alice" || usage.TopUsers[0].Count != 6 {
		t.Errorf("TopUsers[0] = %+v, want agg_alice with 6", usage.TopUsers[0])
	}
	if usage.TopUsers[1].Username != "agg_bob" || usage.TopUsers[1].Count != 4 {
		t.Errorf("TopUsers[1] = %+v, want agg_bob with 4", usage.TopUsers[1])
	}
}

// TestTicketReport_StatusCountsMapCorrectly guards the collapse of six status
// counts into one grouped query: a status landing in the wrong bucket is
// invisible unless the counts differ per status.
func TestTicketReport_StatusCountsMapCorrectly(t *testing.T) {
	d := testutil.NewDB(t)
	svc := NewReportService(d)

	uid := testutil.SeedUser(t, d.DB, "ticket_reporter", "developer")
	dsID := testutil.SeedDatasource(t, d.DB, "ticket-report-ds")

	// Distinct counts per status so a swapped mapping cannot pass.
	seed := map[string]int{
		"SUBMITTED": 1, "PENDING_APPROVAL": 3,
		"APPROVED": 4, "REJECTED": 5, "DONE": 6, "CANCELLED": 7,
	}
	for status, n := range seed {
		for i := 0; i < n; i++ {
			if _, err := d.Exec(
				`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary,
				                      db_type, status, risk_level, created_at, updated_at)
				 VALUES ($1, $2, 'appdb', 'SELECT 1', 'SELECT 1', 'mysql', $3, 'low', now(), now())`,
				uid, dsID, status,
			); err != nil {
				t.Fatalf("seed ticket %s: %v", status, err)
			}
		}
	}

	stats, err := svc.GetTicketReport(context.Background(), ReportParams{Days: 7})
	if err != nil {
		t.Fatalf("GetTicketReport: %v", err)
	}
	checks := []struct {
		name string
		got  int64
		want int64
	}{
		{"TotalTickets", stats.TotalTickets, 26},
		{"PendingCount", stats.PendingCount, 4}, // SUBMITTED + PENDING_APPROVAL
		{"ApprovedCount", stats.ApprovedCount, 4},
		{"RejectedCount", stats.RejectedCount, 5},
		{"DoneCount", stats.DoneCount, 6},
		{"CancelledCount", stats.CancelledCount, 7},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
	if len(stats.RiskDistribution) != 1 || stats.RiskDistribution[0].RiskLevel != "low" {
		t.Errorf("RiskDistribution = %+v, want one entry for low", stats.RiskDistribution)
	}
	if stats.RiskDistribution[0].Count != 26 {
		t.Errorf("RiskDistribution count = %d, want 26", stats.RiskDistribution[0].Count)
	}
}
