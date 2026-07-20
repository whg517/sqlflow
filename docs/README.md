# SQLFlow 文档中心

本目录记录 SQLFlow 的产品需求、系统架构、关键技术决策和用户故事。当前文档以 `2026-07-14` 仓库实现为基线，是一次从代码、路由、数据模型和自动化测试反向重建的 **as-is 文档**。

## 阅读路径

| 读者 | 建议入口 | 目的 |
|---|---|---|
| 产品、项目负责人 | [需求文档](REQUIREMENTS.md) | 了解目标、范围、业务规则和验收边界 |
| 架构师、开发者 | [架构文档](ARCHITECTURE.md) | 了解系统边界、模块、数据、运行时和部署架构 |
| 评审者、维护者 | [ADR 索引](adr/README.md) | 理解关键决策、后果及后续演进约束 |
| 产品、测试、开发 | [User Stories 索引](user-stories/README.md) | 按角色和场景查看可验收行为 |

## 信息架构

```text
docs/
├── README.md                 # 文档入口与治理规则
├── REQUIREMENTS.md           # 产品需求基线（Why / What）
├── ARCHITECTURE.md           # 架构基线（How）
├── adr/                      # 单项架构决策及其原因
│   ├── README.md
│   └── NNNN-*.md
└── user-stories/             # 按业务域拆分的用户故事与验收标准
    ├── README.md
    └── US-*.md
```

## 文档职责与事实来源

| 内容 | 主事实来源 | 文档职责 |
|---|---|---|
| 产品范围与业务规则 | `docs/REQUIREMENTS.md` | 定义系统应提供什么能力 |
| 用户可观察行为 | `docs/user-stories/` | 定义角色、场景和可验收结果 |
| API 契约 | Handler 注解生成的 OpenAPI | 文档仅描述边界，不复制完整端点清单 |
| 工单状态与领域规则 | `internal/model/`、`internal/service/` | 架构文档解释流程和责任归属 |
| 组件和依赖方向 | `internal/app/`、`internal/api/`、`internal/service/`、`internal/driver/` | 架构文档描述约束 |
| 已作出的架构取舍 | `docs/adr/` | 一项决策一个 ADR |
| 验证证据 | Go、Vitest、Playwright 测试 | User Story 引用代表性测试范围 |

当文档与可执行契约不一致时，先确认这是实现缺陷还是需求变更，再在同一个变更中同步对应文档。不得用 README 中的摘要覆盖正式需求或 ADR。

## 标识与状态

- 功能需求：`FR-<领域>-NNN`，例如 `FR-QRY-001`。
- 非功能需求：`NFR-<领域>-NNN`，例如 `NFR-SEC-001`。
- 用户故事：`US-<领域>-NNN`，例如 `US-TKT-001`。
- 架构决策：四位顺序号，例如 `0001-modular-monolith.md`。
- ADR 状态：`proposed`、`accepted`、`superseded`、`deprecated`。
- 需求实现状态：`implemented`、`partial`、`planned`、`out-of-scope`。

## 变更规则

1. 新增或改变用户可观察行为时，更新需求和对应 User Story 的验收标准。
2. 改变系统边界、持久化方式、依赖方向、安全边界或部署拓扑时，新增 ADR；不要重写已经接受的决策历史。
3. 架构图只表达稳定关系，端点、字段和配置项应链接到代码或生成契约，避免维护重复清单。
4. 文档中的“已实现”必须能指向代码、迁移或自动化测试；尚未实现的内容必须明确标记。
5. Markdown 使用相对链接，合并前检查本地链接和 Mermaid 语法。

## 当前已知边界

- 覆盖度审计前端页面存在，但后端路由因需要独立 PostgreSQL 且当前启动时传入 `nil` 而被设计性禁用，因此不计入已交付产品范围。
- 平台元数据使用 SQLite；MySQL、PostgreSQL、MongoDB 和 Elasticsearch 是受治理的目标数据源，不是平台元数据库。
- `internal/connpool` 与统一 `internal/driver` 连接池目前并存，属于驱动迁移期兼容状态。
- Ent Schema 与 SQL migration 双轨并存，当前仍由 `golang-migrate` 执行 DDL。

