BEGIN;
CREATE TABLE IF NOT EXISTS coverage_reports (
    id BIGSERIAL PRIMARY KEY, project_name TEXT NOT NULL, branch TEXT NOT NULL DEFAULT 'main',
    commit_hash TEXT NOT NULL DEFAULT '', build_id TEXT NOT NULL DEFAULT '', ci_job TEXT NOT NULL DEFAULT '',
    report_type TEXT NOT NULL DEFAULT 'lcov', language TEXT NOT NULL DEFAULT '',
    line_coverage NUMERIC(6,3) NOT NULL DEFAULT 0, branch_coverage NUMERIC(6,3) NOT NULL DEFAULT 0, func_coverage NUMERIC(6,3) NOT NULL DEFAULT 0,
    total_lines INTEGER NOT NULL DEFAULT 0, covered_lines INTEGER NOT NULL DEFAULT 0,
    total_branches INTEGER NOT NULL DEFAULT 0, covered_branches INTEGER NOT NULL DEFAULT 0,
    total_functions INTEGER NOT NULL DEFAULT 0, covered_functions INTEGER NOT NULL DEFAULT 0,
    raw_report_path TEXT NOT NULL DEFAULT '', raw_report_size INTEGER NOT NULL DEFAULT 0,
    uploaded_by BIGINT NOT NULL DEFAULT 0, is_merged BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_coverage_reports_project ON coverage_reports(project_name);
CREATE INDEX idx_coverage_reports_branch ON coverage_reports(branch);
CREATE INDEX idx_coverage_reports_created ON coverage_reports(created_at DESC);
CREATE INDEX idx_coverage_reports_project_branch ON coverage_reports(project_name, branch);

CREATE TABLE IF NOT EXISTS coverage_modules (
    id BIGSERIAL PRIMARY KEY, report_id BIGINT NOT NULL REFERENCES coverage_reports(id) ON DELETE CASCADE,
    module_path TEXT NOT NULL, module_name TEXT NOT NULL DEFAULT '',
    line_coverage NUMERIC(6,3) NOT NULL DEFAULT 0, branch_coverage NUMERIC(6,3) NOT NULL DEFAULT 0, func_coverage NUMERIC(6,3) NOT NULL DEFAULT 0,
    total_lines INTEGER NOT NULL DEFAULT 0, covered_lines INTEGER NOT NULL DEFAULT 0,
    total_branches INTEGER NOT NULL DEFAULT 0, covered_branches INTEGER NOT NULL DEFAULT 0,
    total_functions INTEGER NOT NULL DEFAULT 0, covered_functions INTEGER NOT NULL DEFAULT 0,
    file_count INTEGER NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_coverage_modules_report ON coverage_modules(report_id);
CREATE INDEX idx_coverage_modules_report_path ON coverage_modules(report_id, module_path);

CREATE TABLE IF NOT EXISTS coverage_files (
    id BIGSERIAL PRIMARY KEY, module_id BIGINT NOT NULL REFERENCES coverage_modules(id) ON DELETE CASCADE,
    report_id BIGINT NOT NULL REFERENCES coverage_reports(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL, file_name TEXT NOT NULL DEFAULT '',
    line_coverage NUMERIC(6,3) NOT NULL DEFAULT 0, branch_coverage NUMERIC(6,3) NOT NULL DEFAULT 0, func_coverage NUMERIC(6,3) NOT NULL DEFAULT 0,
    total_lines INTEGER NOT NULL DEFAULT 0, covered_lines INTEGER NOT NULL DEFAULT 0,
    total_branches INTEGER NOT NULL DEFAULT 0, covered_branches INTEGER NOT NULL DEFAULT 0,
    total_functions INTEGER NOT NULL DEFAULT 0, covered_functions INTEGER NOT NULL DEFAULT 0,
    uncovered_lines INTEGER[] NOT NULL DEFAULT '{}', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_coverage_files_module ON coverage_files(module_id);
CREATE INDEX idx_coverage_files_report ON coverage_files(report_id);
CREATE INDEX idx_coverage_files_report_path ON coverage_files(report_id, file_path);

CREATE TABLE IF NOT EXISTS coverage_gate_configs (
    id BIGSERIAL PRIMARY KEY, project_name TEXT NOT NULL DEFAULT '', module_pattern TEXT NOT NULL DEFAULT '',
    threshold_red NUMERIC(6,3) NOT NULL DEFAULT 50, threshold_orange NUMERIC(6,3) NOT NULL DEFAULT 70,
    threshold_yellow NUMERIC(6,3) NOT NULL DEFAULT 80, threshold_green NUMERIC(6,3) NOT NULL DEFAULT 90,
    gate_mode TEXT NOT NULL DEFAULT 'off', gate_threshold NUMERIC(6,3) NOT NULL DEFAULT 80,
    coverage_type TEXT NOT NULL DEFAULT 'line', description TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_name, module_pattern, coverage_type)
);
CREATE INDEX idx_gate_configs_project ON coverage_gate_configs(project_name);

CREATE TABLE IF NOT EXISTS coverage_audit_log (
    id BIGSERIAL PRIMARY KEY, action TEXT NOT NULL, entity_type TEXT NOT NULL DEFAULT '', entity_id BIGINT NOT NULL DEFAULT 0,
    project_name TEXT NOT NULL DEFAULT '', old_value JSONB NOT NULL DEFAULT '{}', new_value JSONB NOT NULL DEFAULT '{}',
    actor_id BIGINT NOT NULL DEFAULT 0, actor_type TEXT NOT NULL DEFAULT 'user', actor_name TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '', user_agent TEXT NOT NULL DEFAULT '',
    success BOOLEAN NOT NULL DEFAULT TRUE, error_message TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_coverage_audit_action ON coverage_audit_log(action);
CREATE INDEX idx_coverage_audit_project ON coverage_audit_log(project_name);
CREATE INDEX idx_coverage_audit_created ON coverage_audit_log(created_at DESC);
COMMIT;
