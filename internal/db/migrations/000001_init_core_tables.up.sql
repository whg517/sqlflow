-- Core tables: users, datasources, casbin_rule
CREATE TABLE IF NOT EXISTS users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    username    TEXT    NOT NULL UNIQUE,
    password_hash TEXT  NOT NULL,
    role        TEXT    NOT NULL DEFAULT 'developer',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

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
