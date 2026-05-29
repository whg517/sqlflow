-- Git links table for associating tickets/audit logs with git commits and PRs.
CREATE TABLE IF NOT EXISTS git_links (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type  TEXT    NOT NULL DEFAULT 'ticket',
    entity_id    INTEGER NOT NULL DEFAULT 0,
    link_type    TEXT    NOT NULL DEFAULT 'commit',
    commit_hash  TEXT    NOT NULL DEFAULT '',
    commit_msg   TEXT    NOT NULL DEFAULT '',
    author_name  TEXT    NOT NULL DEFAULT '',
    author_email TEXT    NOT NULL DEFAULT '',
    pr_number    INTEGER NOT NULL DEFAULT 0,
    pr_title     TEXT    NOT NULL DEFAULT '',
    pr_url       TEXT    NOT NULL DEFAULT '',
    repo_url     TEXT    NOT NULL DEFAULT '',
    branch       TEXT    NOT NULL DEFAULT '',
    created_by   INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (created_by) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_git_links_entity ON git_links(entity_type, entity_id);
