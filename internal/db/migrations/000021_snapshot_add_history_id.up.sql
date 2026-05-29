-- Add query_history_id column to query_snapshots
ALTER TABLE query_snapshots ADD COLUMN query_history_id INTEGER NOT NULL DEFAULT 0;
