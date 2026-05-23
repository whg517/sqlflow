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
	svc := NewDashboardService(testDB)
	if svc == nil {
		t.Fatal("NewDashboardService returned nil")
	}
}

func TestDashboardService_GetStats_Empty(t *testing.T) {
	testDB := setupDashboardTestDB(t)
	svc := NewDashboardService(testDB)

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
	svc := NewDashboardService(testDB)
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
