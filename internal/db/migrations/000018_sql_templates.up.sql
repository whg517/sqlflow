CREATE TABLE IF NOT EXISTS sql_templates (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    sql_content TEXT    NOT NULL,
    db_type     TEXT    NOT NULL DEFAULT 'mysql',
    category    TEXT    NOT NULL DEFAULT 'general',
    params_json TEXT    NOT NULL DEFAULT '[]',
    is_public   INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id, name)
);

CREATE INDEX IF NOT EXISTS idx_sql_templates_user ON sql_templates(user_id);
CREATE INDEX IF NOT EXISTS idx_sql_templates_category ON sql_templates(category);
