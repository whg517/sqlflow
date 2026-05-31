package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

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

func TestDashboardService_GetOverview_Empty(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))

	overview, err := svc.GetOverview(context.Background(), TimeRangeLast30d)
	if err != nil {
		t.Fatalf("GetOverview failed: %v", err)
	}

	if overview.Stats.PendingTickets != 0 {
		t.Errorf("PendingTickets = %d, want 0", overview.Stats.PendingTickets)
	}
	if len(overview.QueryTrend) != 0 {
		t.Errorf("QueryTrend length = %d, want 0", len(overview.QueryTrend))
	}
	if len(overview.TicketStatusDist) != 0 {
		t.Errorf("TicketStatusDist length = %d, want 0", len(overview.TicketStatusDist))
	}
	if len(overview.RecentActivities) != 0 {
		t.Errorf("RecentActivities length = %d, want 0", len(overview.RecentActivities))
	}
}

func TestDashboardService_GetOverview_WithData(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))
	ctx := context.Background()

	// Seed user
	testDB.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')`,
	)

	// Seed tickets in different statuses
	for _, status := range []string{"SUBMITTED", "PENDING_APPROVAL", "DONE", "REJECTED"} {
		testDB.ExecContext(ctx,
			`INSERT INTO tickets (submitter_id, datasource_id, sql_content, status) VALUES (1, 1, 'SELECT 1', ?)`,
			status,
		)
	}

	// Seed query history entries
	for i := 0; i < 3; i++ {
		testDB.ExecContext(ctx,
			`INSERT INTO query_history (user_id, datasource_id, sql_content, created_at) VALUES (1, 1, 'SELECT 1', datetime('now', ?))`,
			"-1 day",
		)
	}

	// Seed audit logs
	for i := 0; i < 5; i++ {
		testDB.ExecContext(ctx,
			`INSERT INTO audit_logs (user_id, action, sql_summary, created_at) VALUES (1, 'QUERY', 'SELECT * FROM t', datetime('now', ?))`,
			"-1 hour",
		)
	}

	overview, err := svc.GetOverview(ctx, TimeRangeLast30d)
	if err != nil {
		t.Fatalf("GetOverview failed: %v", err)
	}

	// Basic stats
	if overview.Stats.TotalUsers != 1 {
		t.Errorf("TotalUsers = %d, want 1", overview.Stats.TotalUsers)
	}

	// Ticket status distribution should have entries
	if len(overview.TicketStatusDist) == 0 {
		t.Error("TicketStatusDist should not be empty")
	}

	// Query trend should have entries
	if len(overview.QueryTrend) == 0 {
		t.Error("QueryTrend should not be empty")
	}

	// Recent activities should have entries
	if len(overview.RecentActivities) == 0 {
		t.Error("RecentActivities should not be empty")
	}
	if len(overview.RecentActivities) > 10 {
		t.Errorf("RecentActivities length = %d, want <= 10", len(overview.RecentActivities))
	}
}

func TestDashboardService_GetOverview_TimeRange(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(mustWrapDB(testDB))
	ctx := context.Background()

	// Seed user
	testDB.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')`,
	)

	// Test each time range doesn't error
	ranges := []TimeRange{TimeRangeToday, TimeRangeThisWeek, TimeRangeThisMonth, TimeRangeLast30d}
	for _, tr := range ranges {
		_, err := svc.GetOverview(ctx, tr)
		if err != nil {
			t.Errorf("GetOverview with range %s failed: %v", tr, err)
		}
	}
}
