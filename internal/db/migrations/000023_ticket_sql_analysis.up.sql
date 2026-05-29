-- Add sql_type and affected_tables columns to tickets
ALTER TABLE tickets ADD COLUMN sql_type TEXT NOT NULL DEFAULT '';
ALTER TABLE tickets ADD COLUMN affected_tables TEXT NOT NULL DEFAULT '[]';
