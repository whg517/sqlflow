-- FTS5 virtual table for full-text search on audit logs.
-- Uses unicode61 tokenizer which handles CJK characters reasonably well.
-- Standalone FTS5 table (no content=) — data synced via triggers.
CREATE VIRTUAL TABLE IF NOT EXISTS audit_logs_fts USING fts5(
    audit_id,
    sql_content,
    sql_summary,
    action,
    error_message,
    database,
    tokenize='unicode61'
);

-- Triggers to keep FTS5 index in sync with audit_logs.
CREATE TRIGGER IF NOT EXISTS audit_logs_fts_insert AFTER INSERT ON audit_logs BEGIN
    INSERT INTO audit_logs_fts(rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES (new.id, new.id, new.sql_content, new.sql_summary, new.action, new.error_message, new.database);
END;

CREATE TRIGGER IF NOT EXISTS audit_logs_fts_delete AFTER DELETE ON audit_logs BEGIN
    INSERT INTO audit_logs_fts(audit_logs_fts, rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES ('delete', old.id, old.id, old.sql_content, old.sql_summary, old.action, old.error_message, old.database);
END;

CREATE TRIGGER IF NOT EXISTS audit_logs_fts_update AFTER UPDATE ON audit_logs BEGIN
    INSERT INTO audit_logs_fts(audit_logs_fts, rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES ('delete', old.id, old.id, old.sql_content, old.sql_summary, old.action, old.error_message, old.database);
    INSERT INTO audit_logs_fts(rowid, audit_id, sql_content, sql_summary, action, error_message, database)
    VALUES (new.id, new.id, new.sql_content, new.sql_summary, new.action, new.error_message, new.database);
END;
