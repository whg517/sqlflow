CREATE TABLE roles (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    is_builtin   INTEGER NOT NULL DEFAULT 0,
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO roles (name, display_name, description, is_builtin, status) VALUES
    ('admin', '管理员', '拥有平台全部管理权限的内置角色', 1, 'active'),
    ('dba', 'DBA', '负责数据库变更、审批与数据治理的内置角色', 1, 'active'),
    ('developer', '开发人员', '默认只读查询与工单提交角色', 1, 'active');

-- Preserve custom policy subjects created before role management existed.
INSERT OR IGNORE INTO roles (name, display_name, description, is_builtin, status)
SELECT DISTINCT v0, v0, '由既有权限策略迁移生成', 0, 'active'
FROM casbin_rule
WHERE ptype = 'p'
  AND v0 <> ''
  AND v0 NOT LIKE 'user:%';

CREATE INDEX idx_roles_status ON roles(status);
