package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a database/sql connection for SQLite.
type DB struct {
	*sql.DB
}

// Open creates a new SQLite connection and enables WAL mode.
func Open(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// Use file: URI with WAL pragma so every connection from the pool
	// automatically gets the correct settings. modernc.org/sqlite
	// requires this because PRAGMAs set via Exec only apply to the
	// single connection that executed them, not to pooled connections.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", dbPath)

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite only allows one writer at a time. Limit the pool to 1
	// open connection to avoid "database is locked" / I/O errors
	// when multiple goroutines write concurrently.
	conn.SetMaxOpenConns(1)

	return &DB{conn}, nil
}

// Migrate runs schema migrations for all tables.
func (db *DB) Migrate() error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    username    TEXT    NOT NULL UNIQUE,
    password_hash TEXT  NOT NULL,
    role        TEXT    NOT NULL DEFAULT 'developer',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate users: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS datasources (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    name                TEXT    NOT NULL UNIQUE,
    type                TEXT    NOT NULL,
    host                TEXT    NOT NULL,
    port                INTEGER NOT NULL,
    username            TEXT    NOT NULL DEFAULT '',
    password_encrypted  TEXT    NOT NULL DEFAULT '',
    database            TEXT    NOT NULL DEFAULT '',
    max_open            INTEGER NOT NULL DEFAULT 10,
    max_idle            INTEGER NOT NULL DEFAULT 5,
    max_lifetime        INTEGER NOT NULL DEFAULT 3600,
    max_idle_time       INTEGER NOT NULL DEFAULT 600,
    status              TEXT    NOT NULL DEFAULT 'active',
    created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at          DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate datasources: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS casbin_rule (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    ptype TEXT    NOT NULL DEFAULT '',
    v0    TEXT    NOT NULL DEFAULT '',
    v1    TEXT    NOT NULL DEFAULT '',
    v2    TEXT    NOT NULL DEFAULT '',
    v3    TEXT    NOT NULL DEFAULT '',
    v4    TEXT    NOT NULL DEFAULT '',
    v5    TEXT    NOT NULL DEFAULT ''
);
	`)
	if err != nil {
		return fmt.Errorf("migrate casbin_rule: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS query_history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL,
    datasource_id   INTEGER NOT NULL,
    database        TEXT    NOT NULL DEFAULT '',
    sql_content     TEXT    NOT NULL,
    sql_summary     TEXT    NOT NULL DEFAULT '',
    db_type         TEXT    NOT NULL DEFAULT 'mysql',
    execution_time  INTEGER NOT NULL DEFAULT 0,
    result_rows     INTEGER NOT NULL DEFAULT 0,
    affected_rows   INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate query_history: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_query_history_user_id ON query_history(user_id)`)
	if err != nil {
		return fmt.Errorf("migrate query_history index: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_query_history_exec_time ON query_history(execution_time)`)
	if err != nil {
		return fmt.Errorf("migrate query_history exec_time index: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_query_history_created_at ON query_history(created_at)`)
	if err != nil {
		return fmt.Errorf("migrate query_history created_at index: %w", err)
	}

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
		return fmt.Errorf("migrate audit_logs: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id)`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs index: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action)`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs action index: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_logs_datasource_id ON audit_logs(datasource_id)`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs datasource_id index: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at)`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs created_at index: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_logs_ticket_id ON audit_logs(ticket_id)`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs ticket_id index: %w", err)
	}

	// FTS5 virtual table for full-text search on audit logs.
	// Uses unicode61 tokenizer which handles CJK characters reasonably well.
	// Standalone FTS5 table (no content=) — data synced via triggers.
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
		return fmt.Errorf("migrate audit_logs_fts: %w", err)
	}

	// Triggers to keep FTS5 index in sync with audit_logs.
	// Insert trigger.
	_, err = db.Exec(`
CREATE TRIGGER IF NOT EXISTS audit_logs_fts_insert AFTER INSERT ON audit_logs BEGIN
    INSERT INTO audit_logs_fts(rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES (new.id, new.id, new.sql_content, new.sql_summary, new.action, new.error_message, new.database);
END;
	`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs_fts_insert trigger: %w", err)
	}

	// Delete trigger.
	_, err = db.Exec(`
CREATE TRIGGER IF NOT EXISTS audit_logs_fts_delete AFTER DELETE ON audit_logs BEGIN
    INSERT INTO audit_logs_fts(audit_logs_fts, rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES ('delete', old.id, old.id, old.sql_content, old.sql_summary, old.action, old.error_message, old.database);
END;
	`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs_fts_delete trigger: %w", err)
	}

	// Update trigger.
	_, err = db.Exec(`
CREATE TRIGGER IF NOT EXISTS audit_logs_fts_update AFTER UPDATE ON audit_logs BEGIN
    INSERT INTO audit_logs_fts(audit_logs_fts, rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES ('delete', old.id, old.id, old.sql_content, old.sql_summary, old.action, old.error_message, old.database);
    INSERT INTO audit_logs_fts(rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES (new.id, new.id, new.sql_content, new.sql_summary, new.action, new.error_message, new.database);
END;
	`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs_fts_update trigger: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS mask_rules (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    datasource_id   INTEGER NOT NULL DEFAULT 0,
    database        TEXT    NOT NULL DEFAULT '',
    table_name      TEXT    NOT NULL DEFAULT '',
    field           TEXT    NOT NULL DEFAULT '',
    mask_type       TEXT    NOT NULL DEFAULT '',
    custom_regex    TEXT    NOT NULL DEFAULT '',
    custom_template TEXT    NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate mask_rules: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS sensitive_tables (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    datasource_id     INTEGER NOT NULL DEFAULT 0,
    database          TEXT    NOT NULL DEFAULT '',
    table_name        TEXT    NOT NULL DEFAULT '',
    sensitivity_level TEXT    NOT NULL DEFAULT 'medium',
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at        DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate sensitive_tables: %w", err)
	}

	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_sensitive_tables_unique ON sensitive_tables(datasource_id, database, table_name)`)
	if err != nil {
		return fmt.Errorf("migrate sensitive_tables index: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS tickets (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    submitter_id     INTEGER NOT NULL,
    datasource_id    INTEGER NOT NULL,
    database         TEXT    NOT NULL DEFAULT '',
    sql_content      TEXT    NOT NULL,
    sql_summary      TEXT    NOT NULL DEFAULT '',
    db_type          TEXT    NOT NULL DEFAULT 'mysql',
    change_reason    TEXT    NOT NULL DEFAULT '',
    status           TEXT    NOT NULL DEFAULT 'SUBMITTED',
    risk_level       TEXT    NOT NULL DEFAULT '',
    ai_review_result TEXT    NOT NULL DEFAULT '',
    reviewer_id      INTEGER NOT NULL DEFAULT 0,
    review_comment   TEXT    NOT NULL DEFAULT '',
    scheduled_at     DATETIME,
    executed_at      DATETIME,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate tickets: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tickets_submitter_id ON tickets(submitter_id)`)
	if err != nil {
		return fmt.Errorf("migrate tickets index submitter_id: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status)`)
	if err != nil {
		return fmt.Errorf("migrate tickets index status: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tickets_datasource_id ON tickets(datasource_id)`)
	if err != nil {
		return fmt.Errorf("migrate tickets index datasource_id: %w", err)
	}

	// Refresh tokens table
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    token      TEXT    NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    revoked    INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate refresh_tokens: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id)`)
	if err != nil {
		return fmt.Errorf("migrate refresh_tokens index user_id: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_token ON refresh_tokens(token)`)
	if err != nil {
		return fmt.Errorf("migrate refresh_tokens index token: %w", err)
	}

	// Comments table for ticket discussions
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS comments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id    INTEGER NOT NULL,
    user_id     INTEGER NOT NULL,
    content     TEXT    NOT NULL,
    parent_id   INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (order_id) REFERENCES tickets(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
	`)
	if err != nil {
		return fmt.Errorf("migrate comments: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_comments_order_id ON comments(order_id)`)
	if err != nil {
		return fmt.Errorf("migrate comments index order_id: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_comments_parent_id ON comments(parent_id)`)
	if err != nil {
		return fmt.Errorf("migrate comments index parent_id: %w", err)
	}

	// Add scheduled_at column if it doesn't exist (migration for existing DBs)
	_, err = db.Exec(`ALTER TABLE tickets ADD COLUMN scheduled_at DATETIME`)
	if err != nil {
		// Column may already exist, ignore the error
		log.Printf("add scheduled_at column: %v", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tickets_scheduled_at ON tickets(scheduled_at)`)
	if err != nil {
		return fmt.Errorf("migrate tickets index scheduled_at: %w", err)
	}

	// DingTalk OAuth columns for users table (idempotent ALTER TABLE).
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN dingtalk_user_id TEXT DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN dingtalk_union_id TEXT DEFAULT ''`)

	// PostgreSQL datasource support: sslmode and schema_name columns
	_, _ = db.Exec(`ALTER TABLE datasources ADD COLUMN sslmode TEXT DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE datasources ADD COLUMN schema_name TEXT DEFAULT ''`)

	// Git links table for associating tickets/audit logs with git commits and PRs.
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS git_links (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type  TEXT    NOT NULL DEFAULT 'ticket',
    entity_id    INTEGER NOT NULL DEFAULT 0,
    link_type    TEXT    NOT NULL DEFAULT 'commit',
    commit_hash  TEXT    NOT NULL DEFAULT '',
    commit_msg   TEXT    NOT NULL DEFAULT '',
    author_name  TEXT    NOT NULL DEFAULT '',
    author_email TEXT    NOT NULL DEFAULT '',
    pr_number    INTEGER NOT NULL DEFAULT 0,
    pr_title     TEXT    NOT NULL DEFAULT '',
    pr_url       TEXT    NOT NULL DEFAULT '',
    repo_url     TEXT    NOT NULL DEFAULT '',
    branch       TEXT    NOT NULL DEFAULT '',
    created_by   INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (created_by) REFERENCES users(id)
);
	`)
	if err != nil {
		return fmt.Errorf("migrate git_links: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_git_links_entity ON git_links(entity_type, entity_id)`)
	if err != nil {
		return fmt.Errorf("migrate git_links index: %w", err)
	}

	// --- api_tokens ---
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS api_tokens (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL,
    name         TEXT    NOT NULL,
    token_hash   TEXT    NOT NULL,
    token_prefix TEXT    NOT NULL DEFAULT '',
    scopes       TEXT    NOT NULL DEFAULT '',
    expires_at   DATETIME NOT NULL DEFAULT (datetime('now', '+365 days')),
    last_used_at DATETIME,
    use_count    INTEGER NOT NULL DEFAULT 0,
    is_active    INTEGER NOT NULL DEFAULT 1,
    description  TEXT    NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (user_id) REFERENCES users(id)
);
	`)
	if err != nil {
		return fmt.Errorf("migrate api_tokens: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id)`)
	if err != nil {
		return fmt.Errorf("migrate api_tokens user index: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash)`)
	if err != nil {
		return fmt.Errorf("migrate api_tokens hash index: %w", err)
	}

	return nil
}
