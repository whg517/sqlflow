# ADR-0006：统一 Casbin 授权元组与通配语义

- 状态：accepted
- 日期：2026-07-29
- 决策者：SQLFlow Team
- 关联需求：FR-QRY-001、FR-SEC-001、FR-SEC-003
- 取代：无
- 被取代：无

## 背景

查询服务按 `role + ds_<id> + table + action` 请求授权，早期种子策略却把 `*` 写入域和对象，而 Casbin matcher 使用严格相等。结果是空数据库启动后，Developer 和 DBA 的默认策略存在于数据库中却不能匹配真实数据源。临时权限还曾使用裸数据源 ID，管理 API 允许写入任意格式，形成多套不兼容语义。

## 决策

所有 Casbin 策略和请求使用同一四元组：

| 字段 | 规范格式 | 示例 |
|---|---|---|
| Subject | 内置/扩展角色使用小写 slug；个人授权使用 `user:<正整数 ID>` | `developer`、`dba`、`user:42` |
| Domain | 数据源使用 `ds_<正整数 ID>`；策略可使用 `*` | `ds_17` |
| Object | SQL parser 或驱动识别出的表、集合、索引；策略可使用 `*` | `orders` |
| Action | `select/update/delete/insert/ddl/export/desensitize:bypass`；策略可使用 `*` | `select` |

授权 matcher 对 Subject 采用直接匹配或域内角色继承；Domain、Object、Action 的 `*` 仅作为策略侧通配符。运行路径在对象已知时必须传入具体值，不得通过字符串前缀、模糊包含或客户端声明推导权限。

`internal/authz` 是元组构造和边界规范化的唯一入口。查询、导出、脱敏、工单、权限 Middleware、策略管理 API 和临时权限均复用它。管理 API 可为兼容输入接受裸正整数数据源 ID，但保存前必须转换为 `ds_<id>`；其他非规范域和未知动作被拒绝。

## 备选方案

- 为每个数据源生成完整的角色策略：策略数量随数据源增长，新增数据源时容易漏同步。
- 使用 `keyMatch` 等路径匹配函数：当前域和对象不是 URL，扩大匹配能力会增加误配和越权风险。
- 保留严格相等并把请求改成 `*`：会丢失数据源和对象边界，无法表达细粒度授权。

## 后果

- 空库种子策略可以直接覆盖真实 `ds_<id>` 和表对象。
- Developer 默认只读；DBA 获得预定义治理动作；Admin 通过策略侧通配覆盖平台动作。
- 临时策略、管理 API 和运行时消费者不再产生裸数据源域。
- API Token Scope 与个人临时权限如何和角色策略组合仍需在阶段 1 通过统一决策入口完成；本 ADR 不把客户端 Scope 当作 Casbin 授权事实。
- 既有数据库中的非规范自定义策略应在升级检查中识别并迁移，不能静默按模糊规则解释。

## 验证

- 从空 SQLite 元数据库创建 `PermissionService`，断言种子策略数量与 Admin/DBA/Developer 允许—拒绝矩阵。
- 使用真实形式 `ds_17 + orders` 验证策略侧 Domain/Object/Action 通配。
- 策略管理 API 拒绝非规范域和未知动作。
- 临时权限批准后写入 `user:<id> + ds_<id>`，撤销后立即停止匹配。
- Middleware 覆盖路径、JSON 和查询参数的数据源 ID 规范化，包括以 `0` 结尾的 ID。
