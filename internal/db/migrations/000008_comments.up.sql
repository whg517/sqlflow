-- Comments table for ticket discussions
CREATE TABLE IF NOT EXISTS comments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id    INTEGER NOT NULL,
    user_id     INTEGER NOT NULL,
    content     TEXT    NOT NULL,
    parent_id   INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (order_id) REFERENCES tickets(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_comments_order_id ON comments(order_id);
CREATE INDEX IF NOT EXISTS idx_comments_parent_id ON comments(parent_id);
