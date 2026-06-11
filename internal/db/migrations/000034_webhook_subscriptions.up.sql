-- SF-FEAT0057: Generic outbound webhook event subscriptions

CREATE TABLE IF NOT EXISTS webhook_subscriptions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT    NOT NULL,
    url               TEXT    NOT NULL,                       -- Target HTTPS URL (max 2048 chars, validated)
    encrypted_secret  TEXT    NOT NULL,                       -- AES-GCM encrypted HMAC-SHA256 signing key
    events            TEXT    NOT NULL DEFAULT '[]',          -- JSON array of subscribed event types
    enabled           INTEGER NOT NULL DEFAULT 1,             -- 1=enabled, 0=disabled
    failure_count     INTEGER NOT NULL DEFAULT 0,             -- consecutive failure counter
    last_triggered_at DATETIME,                               -- last successful trigger time
    created_by        TEXT    NOT NULL DEFAULT '',            -- admin username
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at        DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_webhook_subs_enabled ON webhook_subscriptions(enabled);
