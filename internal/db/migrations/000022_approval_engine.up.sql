-- Approval policies table
CREATE TABLE IF NOT EXISTS approval_policies (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT NOT NULL DEFAULT 0,
    -- Condition JSON: {"risk_levels":["high","critical"],"sql_types":["DROP","ALTER"],"environments":["production"]}
    conditions JSON NOT NULL DEFAULT '{}',
    -- Chain JSON: [{"role":"team_lead","auto_skip_same_submitter":false},{"role":"dba","auto_skip_same_submitter":true}]
    approval_chain JSON NOT NULL DEFAULT '[]',
    auto_approve_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    auto_approve_reason TEXT,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- Approval records table
CREATE TABLE IF NOT EXISTS approval_records (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    ticket_id BIGINT NOT NULL,
    policy_id BIGINT,
    stage INT NOT NULL DEFAULT 0,
    total_stages INT NOT NULL DEFAULT 0,
    approver_role VARCHAR(64) NOT NULL DEFAULT '',
    approver_id BIGINT,
    approver_name VARCHAR(255),
    action VARCHAR(32) NOT NULL DEFAULT '',
    comment TEXT,
    auto_approved BOOLEAN NOT NULL DEFAULT FALSE,
    auto_reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_approval_records_ticket_id (ticket_id),
    INDEX idx_approval_records_approver_id (approver_id)
);

-- Extend tickets table
ALTER TABLE tickets ADD COLUMN revision INT NOT NULL DEFAULT 1;
ALTER TABLE tickets ADD COLUMN current_stage INT NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN total_stages INT NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN auto_approved BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE tickets ADD COLUMN auto_approve_reason TEXT;
ALTER TABLE tickets ADD COLUMN policy_id BIGINT;
