CREATE TABLE IF NOT EXISTS tickets (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    submitter_id     INTEGER NOT NULL,
    datasource_id    INTEGER NOT NULL,
    database         TEXT    NOT NULL DEFAULT '',
    sql_content      TEXT    NOT NULL,
    sql_summary      TEXT    NOT NULL DEFAULT '',
    db_type          TEXT    NOT NULL DEFAULT 'mysql',
    change_reason    TEXT    NOT NULL DEFAULT '',
    status           TEXT    NOT NULL DEFAULT 'SUBMITTED',
    risk_level       TEXT    NOT NULL DEFAULT '',
    ai_review_result TEXT    NOT NULL DEFAULT '',
    reviewer_id      INTEGER NOT NULL DEFAULT 0,
    review_comment   TEXT    NOT NULL DEFAULT '',
    scheduled_at     DATETIME,
    executed_at      DATETIME,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_tickets_submitter_id ON tickets(submitter_id);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);
CREATE INDEX IF NOT EXISTS idx_tickets_datasource_id ON tickets(datasource_id);
CREATE INDEX IF NOT EXISTS idx_tickets_scheduled_at ON tickets(scheduled_at);
