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

CREATE TABLE IF NOT EXISTS sensitive_tables (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    datasource_id     INTEGER NOT NULL DEFAULT 0,
    database          TEXT    NOT NULL DEFAULT '',
    table_name        TEXT    NOT NULL DEFAULT '',
    sensitivity_level TEXT    NOT NULL DEFAULT 'medium',
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at        DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sensitive_tables_unique ON sensitive_tables(datasource_id, database, table_name);
