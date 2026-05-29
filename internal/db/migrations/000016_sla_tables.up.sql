-- SLA ticket extension columns
ALTER TABLE tickets ADD COLUMN sla_deadline DATETIME;
ALTER TABLE tickets ADD COLUMN sla_status TEXT NOT NULL DEFAULT 'normal';

CREATE INDEX IF NOT EXISTS idx_tickets_sla_deadline ON tickets(sla_deadline);

-- SLA configuration table
CREATE TABLE IF NOT EXISTS sla_config (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    priority         TEXT    NOT NULL UNIQUE,
    timeout_minutes  INTEGER NOT NULL,
    reminder_percent INTEGER NOT NULL DEFAULT 80,
    escalate_to_role TEXT    NOT NULL DEFAULT 'admin',
    escalate_to_user TEXT    NOT NULL DEFAULT '',
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- SLA action log (idempotent scheduling)
CREATE TABLE IF NOT EXISTS sla_action_log (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id     INTEGER NOT NULL,
    action_type   TEXT    NOT NULL,
    dedup_key     TEXT    NOT NULL UNIQUE,
    notified_user TEXT    NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    sla_config_id INTEGER
);

CREATE INDEX IF NOT EXISTS idx_sla_action_log_ticket ON sla_action_log(ticket_id);
CREATE INDEX IF NOT EXISTS idx_sla_action_log_created_at ON sla_action_log(created_at);
