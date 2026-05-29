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

CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_datasource_id ON audit_logs(datasource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_ticket_id ON audit_logs(ticket_id);
