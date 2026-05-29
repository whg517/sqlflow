CREATE TABLE IF NOT EXISTS web_vitals (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    metric_name      TEXT    NOT NULL,
    value            REAL    NOT NULL,
    rating           TEXT    NOT NULL DEFAULT '',
    path             TEXT    NOT NULL DEFAULT '',
    navigation_type  TEXT    NOT NULL DEFAULT '',
    user_agent       TEXT    NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_web_vitals_metric ON web_vitals(metric_name);
CREATE INDEX IF NOT EXISTS idx_web_vitals_created ON web_vitals(created_at);
