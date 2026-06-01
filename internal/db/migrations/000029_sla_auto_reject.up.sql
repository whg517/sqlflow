-- Add auto_reject_enabled to SLA config (default: false)
-- When enabled, tickets that breach their SLA deadline are automatically rejected.
ALTER TABLE sla_config ADD COLUMN auto_reject_enabled INTEGER NOT NULL DEFAULT 0;
