# 使用 SQLite 保存平台元数据

状态：Accepted

SQLFlow 使用 SQLite 保存用户、数据源配置、权限、工单、审计和其他平台元数据，并启用 WAL、外键、busy timeout 和单写连接。团队接受单节点写入与横向扩展受限的代价，以换取零外部平台数据库依赖、简单备份和单容器交付；目标 MySQL、PostgreSQL、MongoDB 和 Elasticsearch 只承载用户数据，不承担 SQLFlow 控制面存储。

后果：部署必须持久化数据库文件并配置备份；多个 SQLFlow 实例不能在没有新 ADR 和存储迁移方案的情况下共享同一 SQLite 文件。
