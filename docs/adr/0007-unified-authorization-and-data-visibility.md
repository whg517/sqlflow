# ADR-0007：统一授权决策与数据可见性

- 状态：accepted
- 日期：2026-07-30
- 决策者：SQLFlow Team
- 关联需求：FR-IAM-004、FR-SEC-003、FR-SEC-005、FR-SEC-006、FR-SEC-007、FR-SEC-008
- 依赖：ADR-0005、ADR-0006

## 背景

原实现分别在路由、Handler 和 Service 中使用角色、API Token Scope、个人临时策略和资源所有权。各入口使用的主体并不一致：临时权限写给 `user:<id>`，查询只检查角色；Token 被替换为虚拟角色；元数据接口只要求登录；脱敏豁免只检查角色。这使“配置了权限”和“业务实际执行”产生差异。

## 决策

### 统一决策输入

每次授权都使用服务端构造的 Actor、Resource、Action 和 Context：

```text
Actor    = user_id + current_role + optional_token_scopes
Resource = datasource + database/schema + object_type + object + optional_field
Action   = discover | metadata:view | select | export | insert | update |
           delete | ddl | unmask | platform-resource actions
Context  = owner/reviewer + temporary_expiry + ticket_state
```

角色策略和有效的个人策略取并集。API Token Scope 与该结果取交集，Scope 永远不能扩大用户权限。Token 使用所有者的当前角色，不创建独立虚拟角色。

### 元数据可见性

- `discover` 控制是否显示数据源安全摘要。
- `metadata:view` 控制表、集合、索引及字段元数据。
- `select` 隐含相同对象的元数据可见性，以保证查询编辑器可用。
- 元数据由服务端过滤；前端隐藏菜单和过滤数组不构成安全边界。
- 字段接口在读取真实数据库元数据之前先完成对象权限校验。

### 字段脱敏

- 脱敏规则描述“哪些返回字段如何转换”，不是查询授权。
- 规则维度为数据源、数据库/Schema、表/集合/索引、字段路径和策略。
- 命中规则时默认脱敏；只有明确获得目标对象 `unmask` 的主体才返回原值。
- `select`、`export` 和 `unmask` 相互独立。导出必须同时满足 `select + export`，且不自动绕过脱敏。
- `desensitize:bypass` 仅作为迁移期兼容动作，新策略使用 `unmask`。

### 临时权限

临时策略的到期时间在每次授权决策时同步检查。定时清理负责状态和存储回收，但系统安全性不依赖调度器准时运行。

### 平台资源所有权

普通用户默认只能查看自己的工单、权限申请、查询历史、模板、分享和导出任务。DBA/Admin 是否可查看全局资源由资源类型的治理职责矩阵决定，不能由客户端 `scope=all` 扩大。

## 后果

- 权限页面配置、SQL 自动补全、查询、导出和脱敏共享同一主体语义。
- 可以只开放表结构而不开放数据，也可以允许查询但始终返回脱敏值。
- 表级策略能够控制对象是否出现在数据源导航中。
- 后续需把 Object 从单独表名迁移为包含 database/schema/object-type 的规范资源路径，避免同一数据源下同名对象冲突。
- 所有新增受保护路由必须声明 Token Scope；未声明的 Token 路由应默认拒绝。

## 验证

- 角色策略或有效个人策略任一允许时 JWT 请求可通过；过期个人策略立即拒绝。
- API Token 同时满足用户权限和所需 Scope 才通过。
- 表列表逐对象过滤，越权字段接口返回 403。
- `select` 不隐含 `export` 或 `unmask`。
- 普通用户不能按 ID 读取他人的工单、执行结果、修订和权限申请。
