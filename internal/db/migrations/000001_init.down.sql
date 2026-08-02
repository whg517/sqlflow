-- 回滚初始 schema。
--
-- 逐表 DROP 需要与建表顺序完全相反，任何一处遗漏都会因外键残留而失败；
-- 初始 migration 的回滚目标本就是「空库」，所以直接重建 schema。
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
