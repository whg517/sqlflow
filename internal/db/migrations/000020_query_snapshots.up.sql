CREATE TABLE IF NOT EXISTS query_snapshots (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    label      TEXT    NOT NULL DEFAULT '',
    columns    TEXT    NOT NULL,
    rows       TEXT    NOT NULL,
    row_count  INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_query_snapshots_user ON query_snapshots(user_id);
