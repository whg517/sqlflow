-- export_tasks (async export jobs)
CREATE TABLE IF NOT EXISTS export_tasks (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL,
    username      TEXT    NOT NULL DEFAULT '',
    export_type   TEXT    NOT NULL DEFAULT '',
    status        TEXT    NOT NULL DEFAULT 'pending',
    filename      TEXT    NOT NULL DEFAULT '',
    file_path     TEXT    NOT NULL DEFAULT '',
    total_rows    INTEGER NOT NULL DEFAULT 0,
    file_bytes    INTEGER NOT NULL DEFAULT 0,
    filters_json  TEXT    NOT NULL DEFAULT '{}',
    error_msg     TEXT    NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    completed_at  DATETIME
);

CREATE INDEX IF NOT EXISTS idx_export_tasks_user ON export_tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_export_tasks_status ON export_tasks(status);
