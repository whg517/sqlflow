CREATE TABLE IF NOT EXISTS permission_requests (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    applicant_id    INTEGER NOT NULL,
    datasource_id   INTEGER NOT NULL,
    database        TEXT    NOT NULL,
    table_name      TEXT    NOT NULL DEFAULT '',
    actions         TEXT    NOT NULL DEFAULT '',
    reason          TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'PENDING',
    approver_id     INTEGER,
    approve_comment TEXT    NOT NULL DEFAULT '',
    approved_at     DATETIME,
    expires_at      DATETIME NOT NULL DEFAULT (datetime('now', '+24 hours')),
    revoked_at      DATETIME,
    revoked_by      INTEGER,
    revoke_reason   TEXT    NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (applicant_id) REFERENCES users(id),
    FOREIGN KEY (approver_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_perm_req_applicant ON permission_requests(applicant_id);
CREATE INDEX IF NOT EXISTS idx_perm_req_status ON permission_requests(status);
CREATE INDEX IF NOT EXISTS idx_perm_req_ds_db ON permission_requests(datasource_id, database);
