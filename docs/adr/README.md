# 架构决策记录（ADR）

ADR 用来保存 SQLFlow 中对架构有长期影响的决策背景、选择和后果。ADR 记录“为什么”，[架构文档](../ARCHITECTURE.md)描述当前“是什么”。

## 决策索引

| ADR | 标题 | 状态 | 日期 |
|---|---|---|---|
| [0001](0001-modular-monolith.md) | 采用模块化单体和单一部署单元 | accepted | 2026-07-14（补录） |
| [0002](0002-sqlite-metadata-and-migrations.md) | 使用 SQLite 保存平台元数据并保留显式 SQL Migration | accepted | 2026-07-14（补录） |
| [0003](0003-capability-based-datasource-drivers.md) | 使用能力声明的数据源驱动抽象 | accepted | 2026-07-14（补录） |
| [0004](0004-ai-is-advisory.md) | AI 评审只作辅助并提供确定性规则降级 | accepted | 2026-07-14（补录） |
| [0005](0005-server-side-security-enforcement.md) | 在服务端集中执行权限、状态机与脱敏门禁 | accepted | 2026-07-14（补录） |

这些 ADR 是依据既有代码重建的历史决策，因此日期标注为“补录”，不代表设计首次发生在该日。

## 新 ADR 模板

文件名使用 `NNNN-short-title.md`：

```markdown
# ADR-NNNN：标题

- 状态：proposed
- 日期：YYYY-MM-DD
- 决策者：Team
- 关联需求：FR-XXX-NNN
- 取代：无
- 被取代：无

## 背景

问题、约束和需要作出决策的原因。

## 决策

明确、可执行的选择。

## 备选方案

考虑过的方案及未采用原因。

## 后果

正面、负面和后续约束。

## 验证

如何从代码、测试、指标或运行结果确认决策被遵守。
```

## 治理规则

1. 已接受 ADR 不应通过重写来掩盖历史；新决策用新的 ADR 取代旧 ADR。
2. 影响系统边界、数据所有权、依赖方向、安全模型、部署拓扑或不可逆技术选型的变更必须有 ADR。
3. 单纯实现细节、短期任务列表和 API 字段调整通常不需要 ADR。
4. ADR 合并后同步更新本索引及架构文档中的当前状态。

