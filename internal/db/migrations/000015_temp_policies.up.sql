-- temp_policies tracks temporary casbin policies with expiry
CREATE TABLE IF NOT EXISTS temp_policies (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    sub        TEXT    NOT NULL,
    dom        TEXT    NOT NULL,
    obj        TEXT    NOT NULL,
    act        TEXT    NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(sub, dom, obj, act)
);

CREATE INDEX IF NOT EXISTS idx_temp_policies_expiry ON temp_policies(expires_at);
