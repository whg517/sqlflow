-- Add OIDC fields to users table
ALTER TABLE users ADD COLUMN oidc_subject TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN oidc_provider TEXT NOT NULL DEFAULT '';

-- Create oidc_providers configuration table
CREATE TABLE IF NOT EXISTS oidc_providers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    issuer     TEXT    NOT NULL,
    client_id  TEXT    NOT NULL,
    client_secret TEXT NOT NULL DEFAULT '',
    scopes     TEXT    NOT NULL DEFAULT 'openid profile email',
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Index for OIDC user lookup
CREATE INDEX idx_users_oidc_subject ON users(oidc_subject, oidc_provider);
