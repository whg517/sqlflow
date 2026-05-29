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

CREATE INDEX IF NOT EXISTS idx_query_history_user_id ON query_history(user_id);
CREATE INDEX IF NOT EXISTS idx_query_history_exec_time ON query_history(execution_time);
CREATE INDEX IF NOT EXISTS idx_query_history_created_at ON query_history(created_at);
