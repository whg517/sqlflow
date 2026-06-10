DROP INDEX IF EXISTS idx_query_history_sql_hash;
ALTER TABLE query_history DROP COLUMN sql_hash;
