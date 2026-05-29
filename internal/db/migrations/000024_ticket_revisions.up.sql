-- SF-ENG0038: Add revision tracking for ticket resubmission
-- Note: revision column already added by migration 000022 (approval engine)

-- Create ticket_revisions table to store historical snapshots
CREATE TABLE IF NOT EXISTS ticket_revisions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id       INTEGER NOT NULL,
    revision        INTEGER NOT NULL,
    sql_content     TEXT    NOT NULL,
    sql_summary     TEXT    NOT NULL DEFAULT '',
    change_reason   TEXT    NOT NULL DEFAULT '',
    risk_level      TEXT    NOT NULL DEFAULT '',
    ai_review_result TEXT   NOT NULL DEFAULT '',
    reviewer_id     INTEGER NOT NULL DEFAULT 0,
    review_comment  TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ticket_revisions_ticket_id ON ticket_revisions(ticket_id);
CREATE INDEX IF NOT EXISTS idx_ticket_revisions_ticket_rev ON ticket_revisions(ticket_id, revision);
