-- 回滚不恢复被合并掉的重复行：那些行的存在本身就是缺陷，合并后的计数才是
-- 对同一次失败的正确记录。
DROP INDEX IF EXISTS idx_dead_letters_webhook_payload;

ALTER TABLE feishu_dead_letters
    DROP COLUMN IF EXISTS payload_hash;
