-- Remove approval engine tables and ticket columns
DROP TABLE IF EXISTS approval_records;
DROP TABLE IF EXISTS approval_policies;

ALTER TABLE tickets DROP COLUMN policy_id;
ALTER TABLE tickets DROP COLUMN auto_approve_reason;
ALTER TABLE tickets DROP COLUMN auto_approved;
ALTER TABLE tickets DROP COLUMN total_stages;
ALTER TABLE tickets DROP COLUMN current_stage;
ALTER TABLE tickets DROP COLUMN revision;
