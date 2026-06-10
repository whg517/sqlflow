-- SF-FEAT0055: Notification preferences per user
CREATE TABLE IF NOT EXISTS notification_preferences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    channels TEXT NOT NULL DEFAULT '[]',
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    CONSTRAINT uq_user_event UNIQUE (user_id, event_type)
);
CREATE INDEX IF NOT EXISTS idx_notif_prefs_user ON notification_preferences(user_id);
