package db

import (
	"path/filepath"
	"testing"
)

func TestOpen_InMemory(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	defer database.Close()

	if database.DB == nil {
		t.Fatal("DB should not be nil")
	}
}

func TestOpen_File(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer database.Close()

	// Verify the database works by running a simple query
	var result int
	err = database.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if result != 1 {
		t.Errorf("expected 1, got %d", result)
	}
}

func TestOpen_NestedDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sub1", "sub2", "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer database.Close()
}

func TestMigrate(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Verify key tables exist by querying them
	tables := []string{"users", "datasources", "casbin_rule", "query_history", "audit_logs", "tickets", "mask_rules", "refresh_tokens", "comments", "sensitive_tables"}
	for _, table := range tables {
		var count int
		err := database.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count)
		if err != nil {
			t.Errorf("table %q should exist: %v", table, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	// Run migration twice — should not error
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate first: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate second: %v", err)
	}
}

func TestDB_MaxOpenConns(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	// SQLite should have MaxOpenConns = 1
	if database.Stats().MaxOpenConnections != 1 {
		t.Errorf("MaxOpenConns = %d, want 1", database.Stats().MaxOpenConnections)
	}
}
