-- SF-FEAT0052: Add sql_hash column for frequent query dedup
ALTER TABLE query_history ADD COLUMN sql_hash TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_query_history_sql_hash ON query_history(user_id, sql_hash);
