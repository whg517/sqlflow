-- Approval policies table
CREATE TABLE IF NOT EXISTS approval_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    priority INTEGER NOT NULL DEFAULT 0,
    -- Condition JSON: {"risk_levels":["high","critical"],"sql_types":["DROP","ALTER"],"environments":["production"]}
    conditions TEXT NOT NULL DEFAULT '{}',
    -- Chain JSON: [{"role":"team_lead","auto_skip_same_submitter":false},{"role":"dba","auto_skip_same_submitter":true}]
    approval_chain TEXT NOT NULL DEFAULT '[]',
    auto_approve_enabled INTEGER NOT NULL DEFAULT 0,
    auto_approve_reason TEXT,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_approval_policies_enabled ON approval_policies (enabled, priority);

-- Approval records table
CREATE TABLE IF NOT EXISTS approval_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id INTEGER NOT NULL,
    policy_id INTEGER,
    stage INTEGER NOT NULL DEFAULT 0,
    total_stages INTEGER NOT NULL DEFAULT 0,
    approver_role TEXT NOT NULL DEFAULT '',
    approver_id INTEGER,
    approver_name TEXT,
    action TEXT NOT NULL DEFAULT '',
    comment TEXT,
    auto_approved INTEGER NOT NULL DEFAULT 0,
    auto_reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_approval_records_ticket_id ON approval_records (ticket_id);
CREATE INDEX IF NOT EXISTS idx_approval_records_approver_id ON approval_records (approver_id);

-- Extend tickets table
ALTER TABLE tickets ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE tickets ADD COLUMN current_stage INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN total_stages INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN auto_approved INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN auto_approve_reason TEXT;
ALTER TABLE tickets ADD COLUMN policy_id INTEGER;
