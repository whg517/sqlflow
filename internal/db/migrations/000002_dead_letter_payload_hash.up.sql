-- 死信去重：为 (webhook_id, payload) 建立唯一约束。
--
-- RecordDeadLetter 的语义是「一条失败通知一行，附带重试计数」，但此前没有任何
-- 约束保证这一点：它先查后写，两个 worker 同时发送失败就会各建一行。结果是
-- 运维看到同一条失败两次，两行各自按自己的节奏重试，消息最终送达时收件人
-- 收到两遍。
--
-- 唯一索引建在 payload 的哈希上，不是 payload 本身。PostgreSQL 的 btree 索引项
-- 上限约 2704 字节，而卡片里带一条长 SQL 很容易超过——那样的通知会在插入时被
-- 拒绝，而这条路径本身就是错误处理路径，最不容易被发现。
--
-- 哈希列而非表达式索引：与同域的 feishu_webhooks.url_hash 保持一致，且 upsert
-- 的 ON CONFLICT 目标可以直接写列名。

ALTER TABLE feishu_dead_letters
    ADD COLUMN IF NOT EXISTS payload_hash TEXT NOT NULL DEFAULT '';

-- 回填。sha256 是 PostgreSQL 11 起的内置函数，不需要 pgcrypto。
UPDATE feishu_dead_letters
SET payload_hash = encode(sha256(payload::bytea), 'hex')
WHERE payload_hash = '';

-- 合并既有重复行，否则下面的唯一索引直接建不起来。
-- 重复行的存在恰恰是因为去重失效，它们的 attempt_count 是同一次逻辑失败被
-- 分开记的次数，所以求和而不是取其一——计数决定何时放弃重试并清理。
WITH ranked AS (
    SELECT id,
           row_number() OVER (PARTITION BY webhook_id, payload_hash ORDER BY id) AS rn,
           sum(attempt_count) OVER (PARTITION BY webhook_id, payload_hash) AS total,
           max(last_attempt_at) OVER (PARTITION BY webhook_id, payload_hash) AS latest
    FROM feishu_dead_letters
)
UPDATE feishu_dead_letters d
SET attempt_count = ranked.total,
    last_attempt_at = ranked.latest
FROM ranked
WHERE d.id = ranked.id AND ranked.rn = 1;

DELETE FROM feishu_dead_letters d
USING (
    SELECT id,
           row_number() OVER (PARTITION BY webhook_id, payload_hash ORDER BY id) AS rn
    FROM feishu_dead_letters
) ranked
WHERE d.id = ranked.id AND ranked.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_dead_letters_webhook_payload
    ON feishu_dead_letters (webhook_id, payload_hash);
