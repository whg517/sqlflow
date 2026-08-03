-- SQLFlow 平台元数据 schema（PostgreSQL）
--
-- 这是一份初始 schema，而不是 SQLite 时期 72 个增量 migration 的移植。migration 的
-- 意义是演进既有数据库，而不存在任何需要被演进的 PostgreSQL 库；SQLite 时期的演进史
-- 保留在 git 历史里。见 ADR-0009。
--
-- 列类型以 ent schema 的声明为准，而不是照抄 SQLite。SQLite 用 INTEGER 存布尔值，
-- 直接搬过来会让 ent 扫描 BOOLEAN 字段时失败。
--
-- 建表顺序按外键依赖拓扑排序：SQLite 不校验引用表是否已存在，PostgreSQL 校验。
--
-- pg_trgm 供审计全文检索兜底中文：PostgreSQL 内置分词器不切分 CJK。

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS users (
    id                     BIGSERIAL PRIMARY KEY,
    username               TEXT    NOT NULL UNIQUE,
    password_hash          TEXT  NOT NULL,
    role                   TEXT    NOT NULL DEFAULT 'developer',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    dingtalk_user_id       TEXT DEFAULT '',
    dingtalk_union_id      TEXT DEFAULT '',
    oidc_subject           TEXT NOT NULL DEFAULT '',
    oidc_provider          TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS api_tokens (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT NOT NULL,
    name                   TEXT    NOT NULL,
    token_hash             TEXT    NOT NULL,
    token_prefix           TEXT    NOT NULL DEFAULT '',
    scopes                 TEXT    NOT NULL DEFAULT '',
    expires_at             TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '365 days'),
    last_used_at           TIMESTAMPTZ,
    use_count              BIGINT NOT NULL DEFAULT 0,
    is_active              BOOLEAN NOT NULL DEFAULT TRUE,
    description            TEXT    NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS approval_policies (
    id                     BIGSERIAL PRIMARY KEY,
    name                   TEXT NOT NULL UNIQUE,
    description            TEXT,
    enabled                BOOLEAN NOT NULL DEFAULT TRUE,
    priority               BIGINT NOT NULL DEFAULT 0,
    conditions             TEXT NOT NULL DEFAULT '{}',
    approval_chain         TEXT NOT NULL DEFAULT '[]',
    auto_approve_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    auto_approve_reason    TEXT,
    is_default             BOOLEAN NOT NULL DEFAULT FALSE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS approval_records (
    id                     BIGSERIAL PRIMARY KEY,
    ticket_id              BIGINT NOT NULL,
    policy_id              BIGINT,
    stage                  BIGINT NOT NULL DEFAULT 0,
    total_stages           BIGINT NOT NULL DEFAULT 0,
    approver_role          TEXT NOT NULL DEFAULT '',
    approver_id            BIGINT,
    approver_name          TEXT,
    action                 TEXT NOT NULL DEFAULT '',
    comment                TEXT,
    auto_approved          BOOLEAN NOT NULL DEFAULT FALSE,
    auto_reason            TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT NOT NULL,
    action                 TEXT    NOT NULL DEFAULT '',
    datasource_id          BIGINT NOT NULL DEFAULT 0,
    database               TEXT    NOT NULL DEFAULT '',
    sql_content            TEXT    NOT NULL DEFAULT '',
    sql_summary            TEXT    NOT NULL DEFAULT '',
    result_rows            BIGINT NOT NULL DEFAULT 0,
    affected_rows          BIGINT NOT NULL DEFAULT 0,
    execution_time_ms      BIGINT NOT NULL DEFAULT 0,
    error_message          TEXT    NOT NULL DEFAULT '',
    desensitized_fields    TEXT    NOT NULL DEFAULT '',
    ip_address             TEXT    NOT NULL DEFAULT '',
    ai_review_result       TEXT    NOT NULL DEFAULT '',
    ticket_id              BIGINT NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS casbin_rule (
    id                     BIGSERIAL PRIMARY KEY,
    ptype                  TEXT    NOT NULL DEFAULT '',
    v0                     TEXT    NOT NULL DEFAULT '',
    v1                     TEXT    NOT NULL DEFAULT '',
    v2                     TEXT    NOT NULL DEFAULT '',
    v3                     TEXT    NOT NULL DEFAULT '',
    v4                     TEXT    NOT NULL DEFAULT '',
    v5                     TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS tickets (
    id                     BIGSERIAL PRIMARY KEY,
    submitter_id           BIGINT NOT NULL,
    datasource_id          BIGINT NOT NULL,
    database               TEXT    NOT NULL DEFAULT '',
    sql_content            TEXT    NOT NULL,
    sql_summary            TEXT    NOT NULL DEFAULT '',
    db_type                TEXT    NOT NULL DEFAULT 'mysql',
    change_reason          TEXT    NOT NULL DEFAULT '',
    status                 TEXT    NOT NULL DEFAULT 'SUBMITTED',
    risk_level             TEXT    NOT NULL DEFAULT '',
    ai_review_result       TEXT    NOT NULL DEFAULT '',
    reviewer_id            BIGINT NOT NULL DEFAULT 0,
    review_comment         TEXT    NOT NULL DEFAULT '',
    scheduled_at           TIMESTAMPTZ,
    executed_at            TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    sla_deadline           TIMESTAMPTZ,
    sla_status             TEXT NOT NULL DEFAULT 'normal',
    revision               BIGINT NOT NULL DEFAULT 1,
    current_stage          BIGINT NOT NULL DEFAULT 0,
    total_stages           BIGINT NOT NULL DEFAULT 0,
    auto_approved          BOOLEAN NOT NULL DEFAULT FALSE,
    auto_approve_reason    TEXT,
    policy_id              BIGINT,
    sql_type               TEXT NOT NULL DEFAULT '',
    affected_tables        TEXT NOT NULL DEFAULT '[]',
    sql_hash               TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS comments (
    id                     BIGSERIAL PRIMARY KEY,
    order_id               BIGINT NOT NULL,
    user_id                BIGINT NOT NULL,
    content                TEXT    NOT NULL,
    parent_id              BIGINT NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    FOREIGN KEY (order_id) REFERENCES tickets(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS datasources (
    id                     BIGSERIAL PRIMARY KEY,
    name                   TEXT    NOT NULL UNIQUE,
    type                   TEXT    NOT NULL,
    host                   TEXT    NOT NULL,
    port                   BIGINT NOT NULL,
    username               TEXT    NOT NULL DEFAULT '',
    password_encrypted     TEXT    NOT NULL DEFAULT '',
    database               TEXT    NOT NULL DEFAULT '',
    max_open               BIGINT NOT NULL DEFAULT 10,
    max_idle               BIGINT NOT NULL DEFAULT 5,
    max_lifetime           BIGINT NOT NULL DEFAULT 3600,
    max_idle_time          BIGINT NOT NULL DEFAULT 600,
    status                 TEXT    NOT NULL DEFAULT 'active',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    sslmode                TEXT DEFAULT '',
    schema_name            TEXT DEFAULT '',
    es_urls                TEXT DEFAULT '',
    es_version             TEXT DEFAULT '',
    es_auth_type           TEXT DEFAULT '',
    es_api_key             TEXT DEFAULT '',
    es_index_pattern       TEXT DEFAULT '',
    es_verify_certs        BOOLEAN NOT NULL DEFAULT TRUE,
    extra_config           TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS execution_results (
    id                     BIGSERIAL PRIMARY KEY,
    ticket_id              BIGINT NOT NULL,
    statement_index        BIGINT NOT NULL DEFAULT 0,
    sql                    TEXT    NOT NULL,
    status                 TEXT    NOT NULL DEFAULT '',
    rows_affected          BIGINT NOT NULL DEFAULT 0,
    error                  TEXT    NOT NULL DEFAULT '',
    duration_ms            BIGINT NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS export_tasks (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT NOT NULL,
    username               TEXT    NOT NULL DEFAULT '',
    export_type            TEXT    NOT NULL DEFAULT '',
    status                 TEXT    NOT NULL DEFAULT 'pending',
    filename               TEXT    NOT NULL DEFAULT '',
    file_path              TEXT    NOT NULL DEFAULT '',
    total_rows             BIGINT NOT NULL DEFAULT 0,
    file_bytes             BIGINT NOT NULL DEFAULT 0,
    filters_json           TEXT    NOT NULL DEFAULT '{}',
    error_msg              TEXT    NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    completed_at           TIMESTAMPTZ,
    file_format            TEXT NOT NULL DEFAULT 'csv'
);

CREATE TABLE IF NOT EXISTS feishu_dead_letters (
    id                     BIGSERIAL PRIMARY KEY,
    webhook_id             BIGINT NOT NULL,
    payload                TEXT    NOT NULL,
    error_message          TEXT    NOT NULL DEFAULT '',
    attempt_count          BIGINT NOT NULL DEFAULT 0,
    last_attempt_at        TIMESTAMPTZ NOT NULL DEFAULT (now()),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS feishu_webhooks (
    id                     BIGSERIAL PRIMARY KEY,
    name                   TEXT    NOT NULL,
    encrypted_url          TEXT    NOT NULL,
    url_hash               TEXT    NOT NULL,
    scene                  TEXT    NOT NULL DEFAULT 'general',
    enabled                BOOLEAN NOT NULL DEFAULT TRUE,
    rate_limit_rps         DOUBLE PRECISION    NOT NULL DEFAULT 1.0,
    created_by             TEXT    NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS git_links (
    id                     BIGSERIAL PRIMARY KEY,
    entity_type            TEXT    NOT NULL DEFAULT 'ticket',
    entity_id              BIGINT NOT NULL DEFAULT 0,
    link_type              TEXT    NOT NULL DEFAULT 'commit',
    commit_hash            TEXT    NOT NULL DEFAULT '',
    commit_msg             TEXT    NOT NULL DEFAULT '',
    author_name            TEXT    NOT NULL DEFAULT '',
    author_email           TEXT    NOT NULL DEFAULT '',
    pr_number              BIGINT NOT NULL DEFAULT 0,
    pr_title               TEXT    NOT NULL DEFAULT '',
    pr_url                 TEXT    NOT NULL DEFAULT '',
    repo_url               TEXT    NOT NULL DEFAULT '',
    branch                 TEXT    NOT NULL DEFAULT '',
    created_by             BIGINT NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    FOREIGN KEY (created_by) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS mask_rules (
    id                     BIGSERIAL PRIMARY KEY,
    datasource_id          BIGINT NOT NULL DEFAULT 0,
    database               TEXT    NOT NULL DEFAULT '',
    table_name             TEXT    NOT NULL DEFAULT '',
    field                  TEXT    NOT NULL DEFAULT '',
    mask_type              TEXT    NOT NULL DEFAULT '',
    custom_regex           TEXT    NOT NULL DEFAULT '',
    custom_template        TEXT    NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS notification_preferences (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT NOT NULL,
    event_type             TEXT NOT NULL,
    channels               TEXT NOT NULL DEFAULT '[]',
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    CONSTRAINT uq_user_event UNIQUE (user_id, event_type)
);

CREATE TABLE IF NOT EXISTS oidc_providers (
    id                     BIGSERIAL PRIMARY KEY,
    name                   TEXT    NOT NULL UNIQUE,
    issuer                 TEXT    NOT NULL,
    client_id              TEXT    NOT NULL,
    client_secret          TEXT NOT NULL DEFAULT '',
    scopes                 TEXT    NOT NULL DEFAULT 'openid profile email',
    enabled                BOOLEAN NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS permission_requests (
    id                     BIGSERIAL PRIMARY KEY,
    applicant_id           BIGINT NOT NULL,
    datasource_id          BIGINT NOT NULL,
    database               TEXT    NOT NULL,
    table_name             TEXT    NOT NULL DEFAULT '',
    actions                TEXT    NOT NULL DEFAULT '',
    reason                 TEXT    NOT NULL DEFAULT '',
    status                 TEXT    NOT NULL DEFAULT 'PENDING',
    approver_id            BIGINT,
    approve_comment        TEXT    NOT NULL DEFAULT '',
    approved_at            TIMESTAMPTZ,
    expires_at             TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '24 hours'),
    revoked_at             TIMESTAMPTZ,
    revoked_by             BIGINT,
    revoke_reason          TEXT    NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    FOREIGN KEY (applicant_id) REFERENCES users(id),
    FOREIGN KEY (approver_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS query_history (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT NOT NULL,
    datasource_id          BIGINT NOT NULL,
    database               TEXT    NOT NULL DEFAULT '',
    sql_content            TEXT    NOT NULL,
    sql_summary            TEXT    NOT NULL DEFAULT '',
    db_type                TEXT    NOT NULL DEFAULT 'mysql',
    execution_time         BIGINT NOT NULL DEFAULT 0,
    result_rows            BIGINT NOT NULL DEFAULT 0,
    affected_rows          BIGINT NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    sql_hash               TEXT NOT NULL DEFAULT '',
    params_json            TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT NOT NULL,
    token                  TEXT    NOT NULL UNIQUE,
    expires_at             TIMESTAMPTZ NOT NULL,
    revoked                BOOLEAN NOT NULL DEFAULT FALSE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS roles (
    id                     BIGSERIAL PRIMARY KEY,
    name                   TEXT NOT NULL UNIQUE,
    display_name           TEXT NOT NULL,
    description            TEXT NOT NULL DEFAULT '',
    is_builtin             BOOLEAN NOT NULL DEFAULT FALSE,
    status                 TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS sensitive_tables (
    id                     BIGSERIAL PRIMARY KEY,
    datasource_id          BIGINT NOT NULL DEFAULT 0,
    database               TEXT    NOT NULL DEFAULT '',
    table_name             TEXT    NOT NULL DEFAULT '',
    sensitivity_level      TEXT    NOT NULL DEFAULT 'medium',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS shared_results (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT NOT NULL,
    username               TEXT    NOT NULL DEFAULT '',
    token                  TEXT    NOT NULL UNIQUE,
    columns_json           TEXT    NOT NULL DEFAULT '[]',
    rows_json              TEXT    NOT NULL DEFAULT '[]',
    row_count              BIGINT NOT NULL DEFAULT 0,
    expires_at             TIMESTAMPTZ NOT NULL,
    password_hash          TEXT    NOT NULL DEFAULT '',
    sql_summary            TEXT    NOT NULL DEFAULT '',
    datasource_name        TEXT    NOT NULL DEFAULT '',
    revoked                BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at             TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS sla_action_log (
    id                     BIGSERIAL PRIMARY KEY,
    ticket_id              BIGINT NOT NULL,
    action_type            TEXT    NOT NULL,
    dedup_key              TEXT    NOT NULL UNIQUE,
    notified_user          TEXT    NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    sla_config_id          BIGINT
);

CREATE TABLE IF NOT EXISTS sla_config (
    id                     BIGSERIAL PRIMARY KEY,
    priority               TEXT    NOT NULL UNIQUE,
    timeout_minutes        BIGINT NOT NULL,
    reminder_percent       BIGINT NOT NULL DEFAULT 80,
    escalate_to_role       TEXT    NOT NULL DEFAULT 'admin',
    escalate_to_user       TEXT    NOT NULL DEFAULT '',
    enabled                BOOLEAN NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    auto_reject_enabled    BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS sql_templates (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT NOT NULL,
    name                   TEXT    NOT NULL,
    description            TEXT    NOT NULL DEFAULT '',
    sql_content            TEXT    NOT NULL,
    db_type                TEXT    NOT NULL DEFAULT 'mysql',
    category               TEXT    NOT NULL DEFAULT 'general',
    params_json            TEXT    NOT NULL DEFAULT '[]',
    is_public              BOOLEAN NOT NULL DEFAULT FALSE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    UNIQUE(user_id, name)
);

CREATE TABLE IF NOT EXISTS temp_policies (
    id                     BIGSERIAL PRIMARY KEY,
    sub                    TEXT    NOT NULL,
    dom                    TEXT    NOT NULL,
    obj                    TEXT    NOT NULL,
    act                    TEXT    NOT NULL,
    expires_at             TIMESTAMPTZ NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    UNIQUE(sub, dom, obj, act)
);

CREATE TABLE IF NOT EXISTS ticket_notification_logs (
    id                     BIGSERIAL PRIMARY KEY,
    ticket_id              BIGINT NOT NULL,
    event_type             TEXT    NOT NULL,
    sent_at                TIMESTAMPTZ NOT NULL DEFAULT (now()),
    status                 TEXT    NOT NULL DEFAULT 'sent'
);

CREATE TABLE IF NOT EXISTS ticket_revisions (
    id                     BIGSERIAL PRIMARY KEY,
    ticket_id              BIGINT NOT NULL,
    revision               BIGINT NOT NULL,
    sql_content            TEXT    NOT NULL,
    sql_summary            TEXT    NOT NULL DEFAULT '',
    change_reason          TEXT    NOT NULL DEFAULT '',
    risk_level             TEXT    NOT NULL DEFAULT '',
    ai_review_result       TEXT   NOT NULL DEFAULT '',
    reviewer_id            BIGINT NOT NULL DEFAULT 0,
    review_comment         TEXT    NOT NULL DEFAULT '',
    status                 TEXT    NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS web_vitals (
    id                     BIGSERIAL PRIMARY KEY,
    metric_name            TEXT    NOT NULL,
    value                  DOUBLE PRECISION    NOT NULL,
    rating                 TEXT    NOT NULL DEFAULT '',
    path                   TEXT    NOT NULL DEFAULT '',
    navigation_type        TEXT    NOT NULL DEFAULT '',
    user_agent             TEXT    NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

CREATE TABLE IF NOT EXISTS webhook_subscriptions (
    id                     BIGSERIAL PRIMARY KEY,
    name                   TEXT    NOT NULL,
    url                    TEXT    NOT NULL,
    encrypted_secret       TEXT    NOT NULL,
    events                 TEXT    NOT NULL DEFAULT '[]',
    enabled                BOOLEAN NOT NULL DEFAULT TRUE,
    failure_count          BIGINT NOT NULL DEFAULT 0,
    last_triggered_at      TIMESTAMPTZ,
    created_by             TEXT    NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT (now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT (now())
);

-- 内置角色。
--
-- 这是 schema 的一部分而不是应用启动时的 seed：ValidateRole 在创建用户时查这张
-- 表，空表会让「创建 admin 用户」这件事本身失败——包括首次启动的初始化。
INSERT INTO roles (name, display_name, description, is_builtin, status) VALUES
    ('admin',     '管理员',   '拥有平台全部管理权限的内置角色',       TRUE, 'active'),
    ('dba',       'DBA',      '负责数据库变更、审批与数据治理的内置角色', TRUE, 'active'),
    ('developer', '开发人员', '默认只读查询与工单提交角色',           TRUE, 'active')
ON CONFLICT (name) DO NOTHING;

-- 索引

CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_approval_policies_enabled ON approval_policies (enabled, priority);
CREATE INDEX IF NOT EXISTS idx_approval_records_approver_id ON approval_records (approver_id);
CREATE INDEX IF NOT EXISTS idx_approval_records_ticket_id ON approval_records (ticket_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at_action ON audit_logs(created_at, action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at_user_id ON audit_logs(created_at, user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_datasource_id ON audit_logs(datasource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_ticket_id ON audit_logs(ticket_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_comments_order_id ON comments(order_id);
CREATE INDEX IF NOT EXISTS idx_comments_parent_id ON comments(parent_id);
CREATE INDEX IF NOT EXISTS idx_dead_letters_webhook_id ON feishu_dead_letters(webhook_id);
CREATE INDEX IF NOT EXISTS idx_execution_results_ticket_id ON execution_results(ticket_id);
CREATE INDEX IF NOT EXISTS idx_export_tasks_status ON export_tasks(status);
CREATE INDEX IF NOT EXISTS idx_export_tasks_user ON export_tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_feishu_webhooks_enabled ON feishu_webhooks(enabled);
CREATE INDEX IF NOT EXISTS idx_feishu_webhooks_scene ON feishu_webhooks(scene);
CREATE UNIQUE INDEX IF NOT EXISTS idx_feishu_webhooks_url_hash ON feishu_webhooks(url_hash);
CREATE INDEX IF NOT EXISTS idx_git_links_entity ON git_links(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_notif_prefs_user ON notification_preferences(user_id);
CREATE INDEX IF NOT EXISTS idx_perm_req_applicant ON permission_requests(applicant_id);
CREATE INDEX IF NOT EXISTS idx_perm_req_ds_db ON permission_requests(datasource_id, database);
CREATE INDEX IF NOT EXISTS idx_perm_req_status ON permission_requests(status);
CREATE INDEX IF NOT EXISTS idx_query_history_created_at ON query_history(created_at);
CREATE INDEX IF NOT EXISTS idx_query_history_exec_time ON query_history(execution_time);
CREATE INDEX IF NOT EXISTS idx_query_history_sql_hash ON query_history(user_id, sql_hash);
CREATE INDEX IF NOT EXISTS idx_query_history_user_id ON query_history(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_token ON refresh_tokens(token);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_roles_status ON roles(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sensitive_tables_unique ON sensitive_tables(datasource_id, database, table_name);
CREATE INDEX IF NOT EXISTS idx_shared_results_expires ON shared_results(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_shared_results_token ON shared_results(token);
CREATE INDEX IF NOT EXISTS idx_shared_results_user ON shared_results(user_id);
CREATE INDEX IF NOT EXISTS idx_sla_action_log_created_at ON sla_action_log(created_at);
CREATE INDEX IF NOT EXISTS idx_sla_action_log_ticket ON sla_action_log(ticket_id);
CREATE INDEX IF NOT EXISTS idx_sql_templates_category ON sql_templates(category);
CREATE INDEX IF NOT EXISTS idx_sql_templates_user ON sql_templates(user_id);
CREATE INDEX IF NOT EXISTS idx_temp_policies_expiry ON temp_policies(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ticket_notif_log ON ticket_notification_logs(ticket_id, event_type);
CREATE INDEX IF NOT EXISTS idx_ticket_revisions_ticket_id ON ticket_revisions(ticket_id);
CREATE INDEX IF NOT EXISTS idx_ticket_revisions_ticket_rev ON ticket_revisions(ticket_id, revision);
CREATE INDEX IF NOT EXISTS idx_tickets_datasource_id ON tickets(datasource_id);
CREATE INDEX IF NOT EXISTS idx_tickets_scheduled_at ON tickets(scheduled_at);
CREATE INDEX IF NOT EXISTS idx_tickets_sla_deadline ON tickets(sla_deadline);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);
CREATE INDEX IF NOT EXISTS idx_tickets_submitter_id ON tickets(submitter_id);
CREATE INDEX IF NOT EXISTS idx_users_oidc_subject ON users(oidc_subject, oidc_provider);
CREATE INDEX IF NOT EXISTS idx_web_vitals_created ON web_vitals(created_at);
CREATE INDEX IF NOT EXISTS idx_web_vitals_metric ON web_vitals(metric_name);
CREATE INDEX IF NOT EXISTS idx_webhook_subs_enabled ON webhook_subscriptions(enabled);
