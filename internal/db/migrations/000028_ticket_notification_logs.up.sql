-- SF-FEAT0049: Ticket notification log for deduplication and rate limiting

CREATE TABLE IF NOT EXISTS ticket_notification_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id   INTEGER NOT NULL,
    event_type  TEXT    NOT NULL,
    sent_at     DATETIME NOT NULL DEFAULT (datetime('now')),
    status      TEXT    NOT NULL DEFAULT 'sent'
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ticket_notif_log ON ticket_notification_logs(ticket_id, event_type);
