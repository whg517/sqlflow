-- Remove OIDC fields from users table
-- SQLite does not support DROP COLUMN before 3.35.0; no-op for safety.
-- For a clean rollback, recreate the table without these columns.

-- Drop oidc_providers table
DROP TABLE IF EXISTS oidc_providers;

-- Drop OIDC user lookup index
DROP INDEX IF EXISTS idx_users_oidc_subject;
