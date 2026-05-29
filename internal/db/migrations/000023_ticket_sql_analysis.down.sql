-- Remove sql_type and affected_tables columns from tickets
ALTER TABLE tickets DROP COLUMN affected_tables;
ALTER TABLE tickets DROP COLUMN sql_type;
