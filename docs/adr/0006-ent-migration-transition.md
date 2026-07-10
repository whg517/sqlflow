# 分阶段从 raw SQL 迁移到 Ent

状态：Accepted（过渡性）

SQLFlow 在数据访问迁移期间同时暴露 `database/sql` 和 Ent Client，由 `golang-migrate` 继续负责 DDL，业务模块逐步迁移到 Ent。直接启用 Ent 自动迁移会受 SQLite ALTER TABLE 行为影响，并可能丢失 SQL 级默认表达式，因此当前不追求一次性切换；代价是迁移期存在两套访问方式和更高的一致性维护成本。

退出条件：核心数据访问完成迁移、现有 SQLite 数据可无损升级、迁移与回滚方案通过验证后，新 ADR 决定是否启用 Ent Schema 迁移并移除 raw SQL 兼容路径。
