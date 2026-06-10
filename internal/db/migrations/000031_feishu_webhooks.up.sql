-- SF-FEAT0040: Feishu webhook notification channel management

CREATE TABLE IF NOT EXISTS feishu_webhooks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT    NOT NULL,
    encrypted_url   TEXT    NOT NULL,                    -- AES-256-GCM encrypted webhook URL
    url_hash        TEXT    NOT NULL,                    -- SHA-256 hash of URL for dedup
    scene           TEXT    NOT NULL DEFAULT 'general',  -- usage scene: general, sla, audit, etc.
    enabled         INTEGER NOT NULL DEFAULT 1,          -- 1=enabled, 0=disabled
    rate_limit_rps  REAL    NOT NULL DEFAULT 1.0,        -- max requests per second per webhook
    created_by      TEXT    NOT NULL DEFAULT '',          -- admin username who created
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_feishu_webhooks_url_hash ON feishu_webhooks(url_hash);
CREATE INDEX IF NOT EXISTS idx_feishu_webhooks_enabled ON feishu_webhooks(enabled);
CREATE INDEX IF NOT EXISTS idx_feishu_webhooks_scene ON feishu_webhooks(scene);

-- Dead letter queue for failed Feishu notifications
CREATE TABLE IF NOT EXISTS feishu_dead_letters (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    webhook_id      INTEGER NOT NULL,                    -- FK to feishu_webhooks.id
    payload         TEXT    NOT NULL,                     -- original JSON payload
    error_message   TEXT    NOT NULL DEFAULT '',
    attempt_count   INTEGER NOT NULL DEFAULT 0,
    last_attempt_at DATETIME NOT NULL DEFAULT (datetime('now')),
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dead_letters_webhook_id ON feishu_dead_letters(webhook_id);
