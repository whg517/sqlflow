# 使用基于能力声明的数据源 Driver

状态：Accepted

不同数据库在查询语言、事务、元数据、权限和导出方面无法提供完全一致的语义，因此 SQLFlow 不建立“所有数据源行为相同”的抽象。每个数据源实现统一 `Driver` 接口并显式声明 Capability；Service、前端和测试必须根据能力或数据库类型处理差异。`internal/connpool` 只保留迁移期兼容职责，新数据源能力进入 `internal/driver`，避免继续扩大双轨连接模型。
