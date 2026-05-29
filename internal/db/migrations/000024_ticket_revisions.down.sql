-- SF-ENG0038: Remove revision tracking

DROP INDEX IF EXISTS idx_ticket_revisions_ticket_rev;
DROP INDEX IF EXISTS idx_ticket_revisions_ticket_id;
DROP TABLE IF EXISTS ticket_revisions;

-- SQLite does not support DROP COLUMN before 3.35.0;
-- leave the revision column in place (harmless).
