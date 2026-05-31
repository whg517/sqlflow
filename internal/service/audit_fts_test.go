package service

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newAuditFTSTestDB creates an in-memory SQLite database with audit_logs, users, and FTS5 tables.
func newAuditFTSTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)

	// Create audit_logs table.
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
    ai_review_result    TEXT    NOT NULL DEFAULT '',
    ticket_id           INTEGER NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		t.Fatalf("create audit_logs: %v", err)
	}

	// Create users table.
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

	// Create FTS5 virtual table.
	_, err = db.Exec(`
CREATE VIRTUAL TABLE IF NOT EXISTS audit_logs_fts USING fts5(
    audit_id,
    sql_content,
    sql_summary,
    action,
    error_message,
    database,
    tokenize='unicode61'
);
	`)
	if err != nil {
		t.Fatalf("create FTS5 table: %v", err)
	}

	// Triggers for auto-sync.
	_, err = db.Exec(`
CREATE TRIGGER IF NOT EXISTS audit_logs_fts_insert AFTER INSERT ON audit_logs BEGIN
    INSERT INTO audit_logs_fts(rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES (new.id, new.id, new.sql_content, new.sql_summary, new.action, new.error_message, new.database);
END;
	`)
	if err != nil {
		t.Fatalf("create insert trigger: %v", err)
	}

	_, err = db.Exec(`
CREATE TRIGGER IF NOT EXISTS audit_logs_fts_delete AFTER DELETE ON audit_logs BEGIN
    INSERT INTO audit_logs_fts(audit_logs_fts, rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES ('delete', old.id, old.id, old.sql_content, old.sql_summary, old.action, old.error_message, old.database);
END;
	`)
	if err != nil {
		t.Fatalf("create delete trigger: %v", err)
	}

	_, err = db.Exec(`
CREATE TRIGGER IF NOT EXISTS audit_logs_fts_update AFTER UPDATE ON audit_logs BEGIN
    INSERT INTO audit_logs_fts(audit_logs_fts, rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES ('delete', old.id, old.id, old.sql_content, old.sql_summary, old.action, old.error_message, old.database);
    INSERT INTO audit_logs_fts(rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES (new.id, new.id, new.sql_content, new.sql_summary, new.action, new.error_message, new.database);
END;
	`)
	if err != nil {
		t.Fatalf("create update trigger: %v", err)
	}

	return db
}

// seedAuditFTSData inserts test data for FTS search tests.
func seedAuditFTSData(t *testing.T, svc *AuditService) {
	t.Helper()
	records := []AuditRecord{
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT * FROM orders WHERE status = 'active'", SQLSummary: "Query active orders"},
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT * FROM users WHERE role = 'admin'", SQLSummary: "Query admin users"},
		{UserID: 2, Action: "export", SQLContent: "SELECT * FROM orders WHERE created_at > '2024-01-01'", SQLSummary: "Export recent orders"},
		{UserID: 2, Action: "ticket_create", SQLContent: "UPDATE products SET price = 99.99 WHERE id = 1", SQLSummary: "Update product price"},
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT * FROM payments WHERE amount > 1000", SQLSummary: "Query large payments"},
	}
	for _, r := range records {
		svc.Write(context.Background(), r)
	}
}

func TestAuditService_Search_BasicKeyword(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('bob', 'hash', 'admin')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(mustWrapDB(db), 0, 0)
	seedAuditFTSData(t, svc)

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "orders",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("expected 2 results for 'orders', got %d", result.Total)
	}
	if len(result.Logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(result.Logs))
	}
}

func TestAuditService_Search_EmptyKeyword(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	svc := NewAuditService(mustWrapDB(db), 0, 0)
	seedAuditFTSData(t, svc)

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 for empty keyword, got %d", result.Total)
	}
	if len(result.Logs) != 0 {
		t.Errorf("expected 0 logs, got %d", len(result.Logs))
	}
}

func TestAuditService_Search_WhitespaceKeyword(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	svc := NewAuditService(mustWrapDB(db), 0, 0)

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "   ",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 for whitespace keyword, got %d", result.Total)
	}
}

func TestAuditService_Search_FilterByAction(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('bob', 'hash', 'admin')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(mustWrapDB(db), 0, 0)
	seedAuditFTSData(t, svc)

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "orders",
		Page:     1,
		PageSize: 10,
		Action:   "export",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// Only 1 of the 2 "orders" results has action=export.
	if result.Total != 1 {
		t.Errorf("expected 1 for action=export, got %d", result.Total)
	}
	if len(result.Logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(result.Logs))
	}
	if result.Logs[0].Action != "export" {
		t.Errorf("expected action 'export', got %q", result.Logs[0].Action)
	}
}

func TestAuditService_Search_FilterByUserID(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('bob', 'hash', 'admin')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(mustWrapDB(db), 0, 0)
	seedAuditFTSData(t, svc)

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "orders",
		Page:     1,
		PageSize: 10,
		UserID:   "2",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// User 2 has 1 "orders" record (the export).
	if result.Total != 1 {
		t.Errorf("expected 1 for user_id=2, got %d", result.Total)
	}
}

func TestAuditService_Search_Pagination(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(mustWrapDB(db), 0, 0)

	// Insert many records with "SELECT" keyword.
	for i := 0; i < 20; i++ {
		svc.Write(context.Background(), AuditRecord{
			UserID:     1,
			Action:     "query_execute",
			SQLContent: fmt.Sprintf("SELECT * FROM table_%d", i),
		})
	}

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "SELECT",
		Page:     1,
		PageSize: 5,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 20 {
		t.Errorf("expected total 20, got %d", result.Total)
	}
	if len(result.Logs) != 5 {
		t.Errorf("expected 5 logs on page 1, got %d", len(result.Logs))
	}

	// Page 2.
	result2, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "SELECT",
		Page:     2,
		PageSize: 5,
	})
	if err != nil {
		t.Fatalf("search page 2: %v", err)
	}
	if result2.Total != 20 {
		t.Errorf("expected total 20, got %d", result2.Total)
	}
	if len(result2.Logs) != 5 {
		t.Errorf("expected 5 logs on page 2, got %d", len(result2.Logs))
	}
}

func TestAuditService_Search_HighlightFields(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(mustWrapDB(db), 0, 0)
	svc.Write(context.Background(), AuditRecord{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT * FROM orders WHERE status = 'active'",
		SQLSummary: "Query active orders",
	})

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "orders",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(result.Logs))
	}

	// Check that highlight fields contain <mark> tags.
	if result.Logs[0].HighlightSQLContent == "" {
		t.Error("expected hl_sql_content to be non-empty")
	}
	// The original SQL content should be intact.
	if result.Logs[0].SQLContent != "SELECT * FROM orders WHERE status = 'active'" {
		t.Errorf("unexpected sql_content: %q", result.Logs[0].SQLContent)
	}
	// Rank should be negative (lower rank = more relevant).
	if result.Logs[0].Rank >= 0 {
		t.Errorf("expected negative rank, got %f", result.Logs[0].Rank)
	}
}

func TestAuditService_Search_NoResults(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(mustWrapDB(db), 0, 0)
	seedAuditFTSData(t, svc)

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "nonexistent_table_xyz",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0, got %d", result.Total)
	}
	if len(result.Logs) != 0 {
		t.Errorf("expected 0 logs, got %d", len(result.Logs))
	}
}

func TestAuditService_Search_FilterByTimeRange(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(mustWrapDB(db), 0, 0)
	svc.Write(context.Background(), AuditRecord{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT * FROM orders",
	})

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "orders",
		Page:     1,
		PageSize: 10,
		Start:    "2000-01-01",
		End:      "2099-12-31",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 within time range, got %d", result.Total)
	}

	// Outside range.
	result2, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "orders",
		Page:     1,
		PageSize: 10,
		Start:    "2099-12-31",
		End:      "2100-12-31",
	})
	if err != nil {
		t.Fatalf("search outside range: %v", err)
	}
	if result2.Total != 0 {
		t.Errorf("expected 0 outside time range, got %d", result2.Total)
	}
}

func TestAuditService_Search_ChineseKeyword(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(mustWrapDB(db), 0, 0)
	svc.Write(context.Background(), AuditRecord{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT * FROM orders WHERE 订单状态 = 'active'",
		SQLSummary: "查询 订单",
	})

	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "订单",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search chinese: %v", err)
	}
	// Note: unicode61 tokenizer groups CJK characters into single tokens.
	// Standalone CJK words separated by spaces are searchable.
	// CJK characters embedded in longer sequences (e.g., 订单表) form one token
	// and substring matching is not supported. This is a known limitation.
	if result.Total < 1 {
		t.Errorf("expected at least 1 result for Chinese keyword '订单', got %d", result.Total)
	}
}

func TestAuditService_RebuildFTS(t *testing.T) {
	db := newAuditFTSTestDB(t)
	defer db.Close()

	svc := NewAuditService(mustWrapDB(db), 0, 0)

	// Insert records before FTS triggers are active (they should be, but test rebuild anyway).
	svc.Write(context.Background(), AuditRecord{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT * FROM rebuild_test",
	})

	// Rebuild should not error.
	err := svc.RebuildFTS(context.Background())
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	// Search should still work after rebuild.
	result, err := svc.Search(context.Background(), SearchParams{
		Keyword:  "rebuild_test",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search after rebuild: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 after rebuild, got %d", result.Total)
	}
}

func TestEscapeFTS5(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", `"hello"`},
		{"orders table", `"orders table"`},
		{`say "hi"`, `"say ""hi"""`},
		{`test"double"quotes`, `"test""double""quotes"`},
		{"", `""`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeFTS5(tt.input)
			if got != tt.want {
				t.Errorf("escapeFTS5(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
