# 架构决策记录

ADR 记录 SQLFlow 中难以逆转、未来读者可能不理解、且曾在多个可行方案之间做出取舍的决定。架构文档说明系统“现在是什么样”，ADR 说明“为什么选择这样做”。

## 决策索引

| ADR | 状态 | 决策 |
|-----|------|------|
| [0001](0001-modular-monolith.md) | Accepted | 使用模块化单体和单一部署单元 |
| [0002](0002-sqlite-platform-metadata.md) | Accepted | 使用 SQLite 保存平台元数据 |
| [0003](0003-capability-based-datasource-drivers.md) | Accepted | 使用基于能力声明的数据源 Driver |
| [0004](0004-governed-change-workflow.md) | Accepted | 将高风险变更纳入可审计工单流程 |
| [0005](0005-generated-openapi-contract.md) | Accepted | 从 Handler 注解生成 OpenAPI 契约 |
| [0006](0006-ent-migration-transition.md) | Accepted, transitional | 分阶段从 raw SQL 迁移到 Ent |

## 新增和修改规则

- 文件名使用四位顺序号和短横线 slug，例如 `0007-example.md`。
- 已被采用的 ADR 不通过重写历史来改变结论；新建 ADR 并把旧 ADR 标记为 `Superseded`。
- 只记录决定、背景和必要后果，不复制需求或架构正文。
- ADR 进入 `Accepted` 前，应同步检查需求、架构、部署和迁移影响。
