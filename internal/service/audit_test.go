package service

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newAuditTestDB creates an in-memory SQLite database with the audit_logs schema.
func newAuditTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS audit_logs (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL,
    action              TEXT    NOT NULL DEFAULT '',
    datasource_id       INTEGER NOT NULL DEFAULT 0,
    database            TEXT    NOT NULL DEFAULT '',
    sql_content         TEXT    NOT NULL DEFAULT '',
    sql_summary         TEXT    NOT NULL DEFAULT '',
    result_rows         INTEGER NOT NULL DEFAULT 0,
    affected_rows       INTEGER NOT NULL DEFAULT 0,
    execution_time_ms   INTEGER NOT NULL DEFAULT 0,
    error_message       TEXT    NOT NULL DEFAULT '',
    desensitized_fields TEXT    NOT NULL DEFAULT '',
    ip_address          TEXT    NOT NULL DEFAULT '',
    created_at          DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		t.Fatalf("create audit_logs: %v", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS users (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    username     TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'developer',
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		t.Fatalf("create users: %v", err)
	}

	return db
}

func TestAuditService_WriteSync(t *testing.T) {
	db := newAuditTestDB(t)
	defer db.Close()

	svc := NewAuditService(db, 0, 0)

	// Write several records — each is persisted immediately.
	for i := 0; i < 10; i++ {
		svc.Write(context.Background(),AuditRecord{
			UserID:     int64(i + 1),
			Action:     "query_execute",
			SQLContent: fmt.Sprintf("SELECT %d", i),
			SQLSummary: fmt.Sprintf("SELECT %d", i),
		})
	}

	// Verify all records were written immediately (no Close/flush needed).
	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 records, got %d", count)
	}
}

func TestAuditService_WriteSingleRecord(t *testing.T) {
	db := newAuditTestDB(t)
	defer db.Close()

	svc := NewAuditService(db, 0, 0)

	svc.Write(context.Background(),AuditRecord{
		UserID:     1,
		Action:     "export",
		SQLContent: "SELECT * FROM orders",
	})

	// Record is immediately available without any flush.
	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 record, got %d", count)
	}
}

func TestAuditService_List_NoFilters(t *testing.T) {
	db := newAuditTestDB(t)
	defer db.Close()

	svc := NewAuditService(db, 100, 50*time.Millisecond)

	// Insert a user for the join.
	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	for i := 0; i < 5; i++ {
		svc.Write(context.Background(),AuditRecord{
			UserID:     1,
			Action:     "query_execute",
			SQLContent: fmt.Sprintf("SELECT %d", i),
		})
	}
	svc.Close()

	svc2 := NewAuditService(db, 100, 50*time.Millisecond)
	defer svc2.Close()

	logs, total, err := svc2.List(context.Background(),1, 10, "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(logs) != 5 {
		t.Errorf("expected 5 logs, got %d", len(logs))
	}
}

func TestAuditService_List_WithFilters(t *testing.T) {
	db := newAuditTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('bob', 'hash', 'admin')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// Insert records directly for deterministic testing.
	records := []AuditRecord{
		{UserID: 1, Action: "query_execute", DatasourceID: 1, SQLContent: "SELECT * FROM orders"},
		{UserID: 1, Action: "export", DatasourceID: 1, SQLContent: "SELECT * FROM users"},
		{UserID: 1, Action: "ticket_create", DatasourceID: 2, SQLContent: "UPDATE orders SET status=1"},
	}
	svc := NewAuditService(db, 100, 50*time.Millisecond)
	for _, r := range records {
		svc.Write(context.Background(),r)
	}
	svc.Close()

	svc2 := NewAuditService(db, 100, 50*time.Millisecond)
	defer svc2.Close()

	t.Run("filter by action", func(t *testing.T) {
		_, total, err := svc2.List(context.Background(),1, 10, "", "query_execute", "", "", "", "")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 {
			t.Errorf("expected 1, got %d", total)
		}
	})

	t.Run("filter by user_id", func(t *testing.T) {
		_, total, err := svc2.List(context.Background(),1, 10, "1", "", "", "", "", "")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 3 {
			t.Errorf("expected 3, got %d", total)
		}
	})

	t.Run("filter by datasource_id", func(t *testing.T) {
		_, total, err := svc2.List(context.Background(),1, 10, "", "", "2", "", "", "")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 {
			t.Errorf("expected 1, got %d", total)
		}
	})

	t.Run("filter by keyword", func(t *testing.T) {
		_, total, err := svc2.List(context.Background(),1, 10, "", "", "", "", "", "orders")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 2 {
			t.Errorf("expected 2 (orders appears in 2 records), got %d", total)
		}
	})
}

func TestAuditService_List_Pagination(t *testing.T) {
	db := newAuditTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(db, 100, 50*time.Millisecond)
	for i := 0; i < 15; i++ {
		svc.Write(context.Background(),AuditRecord{
			UserID:     1,
			Action:     "query_execute",
			SQLContent: fmt.Sprintf("SELECT %d", i),
		})
	}
	svc.Close()

	svc2 := NewAuditService(db, 100, 50*time.Millisecond)
	defer svc2.Close()

	// Page 1 with size 5.
	logs, total, err := svc2.List(context.Background(),1, 5, "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if total != 15 {
		t.Errorf("expected total 15, got %d", total)
	}
	if len(logs) != 5 {
		t.Errorf("expected 5 logs on page 1, got %d", len(logs))
	}

	// Page 3 with size 5.
	logs, _, err = svc2.List(context.Background(),3, 5, "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("list page 3: %v", err)
	}
	if len(logs) != 5 {
		t.Errorf("expected 5 logs on page 3, got %d", len(logs))
	}

	// Page 4 with size 5 should be empty.
	logs, _, err = svc2.List(context.Background(),4, 5, "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("list page 4: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("expected 0 logs on page 4, got %d", len(logs))
	}
}

func TestAuditService_List_Empty(t *testing.T) {
	db := newAuditTestDB(t)
	defer db.Close()

	svc := NewAuditService(db, 100, 50*time.Millisecond)
	defer svc.Close()

	logs, total, err := svc.List(context.Background(),1, 10, "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if logs == nil || len(logs) != 0 {
		t.Errorf("expected empty slice, got %v", logs)
	}
}

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"100%", `100\%`},
		{"user_id", `user\_id`},
		{"50%_off", `50\%\_off`},
		{`path\to\file`, `path\\to\\file`},
		{`%\_`, `\%\\\_`},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeLike(tt.input)
			if got != tt.want {
				t.Errorf("escapeLike(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAuditService_List_KeywordWithWildcards(t *testing.T) {
	db := newAuditTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(db, 0, 0)

	// Insert records that contain literal % and _ characters.
	records := []AuditRecord{
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT discount_100 FROM orders"},
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT 100% FROM stats"},
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT * FROM users"},
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT 50%_off FROM promotions"},
	}
	for _, r := range records {
		svc.Write(context.Background(),r)
	}

	t.Run("keyword with % only matches literal", func(t *testing.T) {
		_, total, err := svc.List(context.Background(),1, 10, "", "", "", "", "", "100%")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		// Should only match "SELECT 100% FROM stats", not everything ending with a number.
		if total != 1 {
			t.Errorf("expected 1 (literal 100%%), got %d", total)
		}
	})

	t.Run("keyword with _ only matches literal", func(t *testing.T) {
		_, total, err := svc.List(context.Background(),1, 10, "", "", "", "", "", "discount_100")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		// Should only match the exact "discount_100", not "discountX100".
		if total != 1 {
			t.Errorf("expected 1 (literal discount_100), got %d", total)
		}
	})

	t.Run("keyword with both % and _", func(t *testing.T) {
		_, total, err := svc.List(context.Background(),1, 10, "", "", "", "", "", "50%_off")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		// Should only match "SELECT 50%_off FROM promotions".
		if total != 1 {
			t.Errorf("expected 1 (literal 50%%_off), got %d", total)
		}
	})

	t.Run("plain keyword matches multiple", func(t *testing.T) {
		_, total, err := svc.List(context.Background(),1, 10, "", "", "", "", "", "SELECT")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 4 {
			t.Errorf("expected 4 (all contain SELECT), got %d", total)
		}
	})
}

func TestAuditService_CloseIsNoop(t *testing.T) {
	db := newAuditTestDB(t)
	defer db.Close()

	svc := NewAuditService(db, 0, 0)
	svc.Write(context.Background(),AuditRecord{UserID: 1, Action: "query_execute"})
	svc.Close()

	// Close is a no-op; data is already written.
	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 record, got %d", count)
	}
}
