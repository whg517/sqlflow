package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

func setupDashboardTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return database.DB
}

func TestNewDashboardService(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))
	if svc == nil {
		t.Fatal("NewDashboardService returned nil")
	}
}

func TestDashboardService_GetStats_Empty(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))

	stats, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.PendingTickets != 0 {
		t.Errorf("PendingTickets = %d, want 0", stats.PendingTickets)
	}
	if stats.RecentQueries7d != 0 {
		t.Errorf("RecentQueries7d = %d, want 0", stats.RecentQueries7d)
	}
	if stats.ActiveDatasources != 0 {
		t.Errorf("ActiveDatasources = %d, want 0", stats.ActiveDatasources)
	}
	if stats.TotalUsers != 0 {
		t.Errorf("TotalUsers = %d, want 0", stats.TotalUsers)
	}
}

func TestDashboardService_GetStats_WithData(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))
	ctx := context.Background()

	// Seed users
	for i := 0; i < 3; i++ {
		testDB.ExecContext(ctx,
			`INSERT INTO users (username, password_hash, role) VALUES (?, 'hash', 'developer')`,
			"dev"+string(rune('0'+i)),
		)
	}

	// Seed active datasources
	for i := 0; i < 2; i++ {
		testDB.ExecContext(ctx,
			`INSERT INTO datasources (name, type, host, port, status) VALUES (?, 'mysql', 'localhost', 3306, 'active')`,
			"ds"+string(rune('0'+i)),
		)
	}
	// Seed an inactive datasource
	testDB.ExecContext(ctx,
		`INSERT INTO datasources (name, type, host, port, status) VALUES (?, 'mysql', 'localhost', 3306, 'disabled')`,
		"ds_disabled",
	)

	// Seed query history
	testDB.ExecContext(ctx,
		`INSERT INTO query_history (user_id, datasource_id, sql_content, created_at) VALUES (1, 1, 'SELECT 1', datetime('now'))`,
	)

	// Seed tickets in various statuses
	pendingStatuses := []string{"SUBMITTED", "AI_REVIEWED", "PENDING_APPROVAL"}
	for _, s := range pendingStatuses {
		testDB.ExecContext(ctx,
			`INSERT INTO tickets (submitter_id, datasource_id, sql_content, status) VALUES (1, 1, 'ALTER TABLE t ADD COLUMN c INT', ?)`,
			s,
		)
	}
	// Seed a completed ticket (should not count as pending)
	testDB.ExecContext(ctx,
		`INSERT INTO tickets (submitter_id, datasource_id, sql_content, status) VALUES (1, 1, 'DROP TABLE t', 'DONE')`,
	)

	stats, err := svc.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalUsers != 3 {
		t.Errorf("TotalUsers = %d, want 3", stats.TotalUsers)
	}
	if stats.ActiveDatasources != 2 {
		t.Errorf("ActiveDatasources = %d, want 2", stats.ActiveDatasources)
	}
	if stats.RecentQueries7d != 1 {
		t.Errorf("RecentQueries7d = %d, want 1", stats.RecentQueries7d)
	}
	if stats.PendingTickets != 3 {
		t.Errorf("PendingTickets = %d, want 3", stats.PendingTickets)
	}
}

// --- GetFullStats tests ---

func TestDashboardService_GetFullStats_Empty(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))

	stats, err := svc.GetFullStats(context.Background(), "", "")
	if err != nil {
		t.Fatalf("GetFullStats on empty DB: %v", err)
	}

	if stats.PendingTickets != 0 {
		t.Errorf("PendingTickets = %d, want 0", stats.PendingTickets)
	}
	if stats.RecentQueries7d != 0 {
		t.Errorf("RecentQueries7d = %d, want 0", stats.RecentQueries7d)
	}
	if stats.ActiveDatasources != 0 {
		t.Errorf("ActiveDatasources = %d, want 0", stats.ActiveDatasources)
	}
	if len(stats.PendingTicketSparkline) != 7 {
		t.Errorf("PendingTicketSparkline len = %d, want 7", len(stats.PendingTicketSparkline))
	}
	if len(stats.QuerySparkline) != 7 {
		t.Errorf("QuerySparkline len = %d, want 7", len(stats.QuerySparkline))
	}
	if len(stats.DatasourceSparkline) != 7 {
		t.Errorf("DatasourceSparkline len = %d, want 7", len(stats.DatasourceSparkline))
	}
	if len(stats.TicketStatusDistribution) != 0 {
		t.Errorf("TicketStatusDistribution should be empty, got %v", stats.TicketStatusDistribution)
	}
	if len(stats.RecentActivity) != 0 {
		t.Errorf("RecentActivity should be empty, got %d items", len(stats.RecentActivity))
	}
	if len(stats.QueryTrend) != 7 {
		t.Errorf("QueryTrend default should be 7 days, got %d", len(stats.QueryTrend))
	}
}

func TestDashboardService_GetFullStats_WithData(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))
	ctx := context.Background()

	// Seed datasources
	testDB.ExecContext(ctx,
		`INSERT INTO datasources (name, type, host, port, status) VALUES ('ds1', 'mysql', 'localhost', 3306, 'active')`)

	// Seed query history today
	for i := 0; i < 5; i++ {
		testDB.ExecContext(ctx,
			`INSERT INTO query_history (user_id, datasource_id, sql_content, created_at) VALUES (1, 1, 'SELECT 1', datetime('now'))`)
	}

	// Seed tickets in different statuses
	statuses := []string{"SUBMITTED", "SUBMITTED", "DONE", "REJECTED", "APPROVED"}
	for _, s := range statuses {
		testDB.ExecContext(ctx,
			`INSERT INTO tickets (submitter_id, datasource_id, sql_content, status) VALUES (1, 1, 'ALTER TABLE t ADD c INT', ?)`,
			s)
	}

	// Seed audit logs
	for i := 0; i < 15; i++ {
		testDB.ExecContext(ctx,
			`INSERT INTO audit_logs (user_id, action, ip_address, created_at) VALUES (1, 'query', '127.0.0.1', datetime('now'))`)
	}

	stats, err := svc.GetFullStats(ctx, "", "")
	if err != nil {
		t.Fatalf("GetFullStats failed: %v", err)
	}

	// Stat cards
	if stats.PendingTickets != 2 {
		t.Errorf("PendingTickets = %d, want 2", stats.PendingTickets)
	}
	if stats.RecentQueries7d != 5 {
		t.Errorf("RecentQueries7d = %d, want 5", stats.RecentQueries7d)
	}
	if stats.ActiveDatasources != 1 {
		t.Errorf("ActiveDatasources = %d, want 1", stats.ActiveDatasources)
	}

	// Sparklines should be length 7
	if len(stats.QuerySparkline) != 7 {
		t.Errorf("QuerySparkline len = %d, want 7", len(stats.QuerySparkline))
	}
	if stats.QuerySparkline[6] != 5 {
		t.Errorf("QuerySparkline[6] (today) = %d, want 5", stats.QuerySparkline[6])
	}

	// Ticket distribution
	if stats.TicketStatusDistribution["SUBMITTED"] != 2 {
		t.Errorf("SUBMITTED count = %d, want 2", stats.TicketStatusDistribution["SUBMITTED"])
	}
	if stats.TicketStatusDistribution["DONE"] != 1 {
		t.Errorf("DONE count = %d, want 1", stats.TicketStatusDistribution["DONE"])
	}
	if stats.TicketStatusDistribution["REJECTED"] != 1 {
		t.Errorf("REJECTED count = %d, want 1", stats.TicketStatusDistribution["REJECTED"])
	}
	if stats.TicketStatusDistribution["APPROVED"] != 1 {
		t.Errorf("APPROVED count = %d, want 1", stats.TicketStatusDistribution["APPROVED"])
	}

	// Recent activity capped at 10
	if len(stats.RecentActivity) != 10 {
		t.Errorf("RecentActivity len = %d, want 10", len(stats.RecentActivity))
	}

	// Query trend
	if len(stats.QueryTrend) != 7 {
		t.Errorf("QueryTrend default len = %d, want 7", len(stats.QueryTrend))
	}
}

func TestDashboardService_GetFullStats_DateRange(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))
	ctx := context.Background()

	// Seed query history 3 days ago
	testDB.ExecContext(ctx,
		`INSERT INTO query_history (user_id, datasource_id, sql_content, created_at) VALUES (1, 1, 'SELECT 1', datetime('now', '-3 days'))`)
	testDB.ExecContext(ctx,
		`INSERT INTO query_history (user_id, datasource_id, sql_content, created_at) VALUES (1, 1, 'SELECT 2', datetime('now', '-3 days'))`)
	testDB.ExecContext(ctx,
		`INSERT INTO query_history (user_id, datasource_id, sql_content, created_at) VALUES (1, 1, 'SELECT 3', datetime('now'))`)

	// Query with specific date range (5 days)
	now := time.Now()
	startDate := now.AddDate(0, 0, -4).Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	stats, err := svc.GetFullStats(ctx, startDate, endDate)
	if err != nil {
		t.Fatalf("GetFullStats with date range failed: %v", err)
	}

	if len(stats.QueryTrend) != 5 {
		t.Errorf("QueryTrend len = %d, want 5", len(stats.QueryTrend))
	}

	// Count should be 2 for 3 days ago, 1 for today
	threeDaysAgo := now.AddDate(0, 0, -3).Format("2006-01-02")
	today := now.Format("2006-01-02")

	found3d := false
	foundToday := false
	for _, d := range stats.QueryTrend {
		if d.Date == threeDaysAgo {
			found3d = true
			if d.Count != 2 {
				t.Errorf("3 days ago count = %d, want 2", d.Count)
			}
		}
		if d.Date == today {
			foundToday = true
			if d.Count != 1 {
				t.Errorf("today count = %d, want 1", d.Count)
			}
		}
	}
	if !found3d {
		t.Errorf("missing 3 days ago in trend")
	}
	if !foundToday {
		t.Errorf("missing today in trend")
	}
}

func TestDashboardService_GetFullStats_InvalidDateRange(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))

	_, err := svc.GetFullStats(context.Background(), "not-a-date", "")
	if err == nil {
		t.Error("expected error for invalid start_date")
	}

	_, err = svc.GetFullStats(context.Background(), "", "bad")
	if err == nil {
		t.Error("expected error for invalid end_date")
	}

	// end < start
	now := time.Now()
	_, err = svc.GetFullStats(context.Background(), now.Format("2006-01-02"), now.AddDate(0, 0, -1).Format("2006-01-02"))
	if err == nil {
		t.Error("expected error for end_date < start_date")
	}
}

func TestDashboardService_GetFullStats_CacheHit(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))
	ctx := context.Background()

	// First call populates cache
	stats1, err := svc.GetFullStats(ctx, "", "")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Second call should hit cache
	stats2, err := svc.GetFullStats(ctx, "", "")
	if err != nil {
		t.Fatalf("second call (cached) failed: %v", err)
	}

	// Should return same data
	if stats1.PendingTickets != stats2.PendingTickets {
		t.Errorf("cached PendingTickets mismatch: %d vs %d", stats1.PendingTickets, stats2.PendingTickets)
	}
}
