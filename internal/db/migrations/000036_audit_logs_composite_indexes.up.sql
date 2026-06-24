-- SF-FEAT0058: Composite indexes for user analytics aggregation performance

CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at_user_id ON audit_logs(created_at, user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at_action ON audit_logs(created_at, action);
