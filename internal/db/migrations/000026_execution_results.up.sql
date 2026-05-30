-- SF-FIX0003: Execution results tracking + ticket FAILED state

-- New table for individual statement execution results
CREATE TABLE IF NOT EXISTS execution_results (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id        INTEGER NOT NULL,
    statement_index  INTEGER NOT NULL DEFAULT 0,
    sql              TEXT    NOT NULL,
    status           TEXT    NOT NULL DEFAULT '',
    rows_affected    INTEGER NOT NULL DEFAULT 0,
    error            TEXT    NOT NULL DEFAULT '',
    duration_ms      INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_execution_results_ticket_id ON execution_results(ticket_id);
