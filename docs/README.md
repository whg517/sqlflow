# SQLFlow 文档索引

Status: Active
Owner: project
Last reviewed: 2026-07-07
Source of truth: no

本目录只保存人工维护的产品、架构、设计和交付文档。API 契约由后端代码注解生成，生成包位于 `internal/api/openapi`，运行时通过 `/swagger/index.html` 查看。

## 阅读路径

| 目标 | 文档 |
|------|------|
| 理解产品边界和优先级 | [需求文档](./spec/REQUIREMENTS.md) |
| 理解系统结构和技术约束 | [架构文档](./spec/ARCHITECTURE.md) |
| 理解前端交互与视觉规范 | [UI 设计文档](./spec/UI-DESIGN.md) |
| 理解端到端用户路径 | [用户旅程](./user-journeys.md) |
| 部署、备份、健康检查和故障排查 | [部署文档](./deployment.md) |

## 文档治理规则

- `docs/spec` 中的文档只描述当前有效规范，不记录版本流水账。
- 历史决策应进入 ADR 或 issue，不附着在现行规范尾部。
- API 端点、参数和响应结构以 handler 注解生成的 OpenAPI 为准。
- 涉及架构、需求、UI、部署行为的代码变更，应同步更新对应文档。
- 文档中可以引用代码路径，但不粘贴大段实现代码。
