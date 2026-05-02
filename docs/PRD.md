# SQL 审批管理平台 - PRD

> 项目代号：SQLFlow
> 创建日期：2026-04-21
> 最后更新：2026-05-02（评审修订 v4）
> 状态：需求确认中

## 背景

公司开发团队频繁找运维执行 SQL，原因：
- 需要运维监管，管控增删改等危险操作
- 避免开发人员查看或导出敏感数据
- 现状为口头面对面沟通，无审批流程和记录

## 目标

让大部分低风险查询自助化，高风险操作走审批流程，全程留痕可追溯。
将运维从"执行者"转变为"审批者和规则制定者"。

## 目标用户

- **开发人员**（主要）：日常查数据、改数据
- **DBA/运维**（次要）：审批、规则配置、数据源管理、审计
- **管理员**（可选）：系统配置（MVP 阶段可由 DBA 兼任）

## 业务范围

- **数据库**：MySQL + MongoDB（仅生产环境，测试环境不管控）
- **不纳入**：Elasticsearch

---

## 功能模块

### 1. 数据源管理

- DBA 在平台上注册目标数据库实例（MySQL / MongoDB）
- 配置项：实例名称、地址、端口、账号密码（加密存储）、最大连接数
- 支持多实例，每个实例可关联多个库
- MVP 只支持 DBA 添加/编辑/禁用，不支持自动发现

### 2. 在线 SQL 查询

**MySQL：**
- 标准 SELECT 查询（支持 JOIN、子查询、聚合）
- 语法校验 + 执行
- 查询结果表格展示，支持分页
- 默认限制返回 1000 行（可配置上限）
- 慢查询检测，超时自动中断（默认 30 秒，可配置）
- 结果集行数 / 执行耗时展示

**MongoDB：**
- MVP 支持 `find` 查询（含条件过滤、投影、排序、分页）
- MVP 支持 `updateOne` / `updateMany`（走工单审批）
- MVP 支持**基本的 aggregation read pipeline**（$match、$group、$project、$sort、$limit、$skip、$count、$unwind）
- MVP 不支持 mapReduce、$lookup、$facet 等复杂聚合
- **Aggregation 安全策略**：平台在提交 aggregation pipeline 时进行 stage 白名单校验，仅允许以下 read-only stage：$match、$group、$project、$sort、$limit、$skip、$count、$unwind、$addFields。包含 $set（修改原数据）、$unset、$rename、$out、$merge、$replaceRoot 等写操作 stage 的 pipeline 直接拦截并拒绝执行

### 3. AI 前置评审

用户提交 SQL 后，AI 自动评审：

**按操作类型分流评审逻辑：**

| 操作类型 | 低风险处理 | 中风险处理 | 高风险处理 |
|----------|-----------|-----------|-----------|
| SELECT | 免审执行 | 执行 + 记录 + DBA 通知 | 强制走工单 |
| DDL（建表/加字段/加索引） | — | 走简化工单（DBA 快速确认） | 走标准工单 |
| DML（UPDATE/DELETE） | — | 走标准工单 | 走标准工单 + AI 生成影响分析 |
| MongoDB update | — | 走标准工单 | 走标准工单 + AI 生成影响分析 |

> 注：SELECT 不会免审放行 DDL/DML。所有变更操作最低走简化工单。

**风险分级因素：**
- SQL 操作类型（SELECT / DDL / DML）
- 涉及表的敏感等级（普通表 / 敏感表 / 核心表）
- 预估影响范围（行数、是否全表扫描）
- 是否有 WHERE 条件（无 WHERE 的 UPDATE/DELETE 直接判定高风险）
- 查询复杂度（JOIN 数量、子查询嵌套深度）
- 用户脱敏权限：拥有 \`desensitize:bypass\` 的用户查敏感表视为高风险（可能查看原始数据），无 bypass 权限的用户查敏感表降级为中风险（结果已脱敏）

> **SELECT 风险判定补充说明：** 敏感表 + 无脱敏豁免 = 中风险（已脱敏，数据安全）；敏感表 + 有脱敏豁免 = 高风险（查看原始数据，需走工单或记录告警）。

**优化建议：**
- 缺少索引提示、全表扫描预警
- JOIN 优化、子查询改写建议
- 大结果集预估，建议加 LIMIT
- MongoDB 查询分析索引命中情况

**变更影响分析（DDL/DML 专用）：**
- DDL：预估影响范围
- DML：预估影响行数、是否锁表
- 自动生成回滚语句

**评审模型：** 外部 LLM API（具体待定）

### 4. 数据脱敏

- 按**表级别**标记哪些表包含敏感数据（用户表、CRM 系统数据）
- 按**字段级别**配置脱敏规则，管理员指定字段名 + 脱敏类型

**内置脱敏类型：**

| 类型 | 规则 | 示例 |
|------|------|------|
| 手机号 | 保留前3后4，中间星号 | 138\*\*\*\*1234 |
| 身份证 | 保留前3后4 | 310\*\*\*\*\*\*\*\*1234 |
| 姓名 | 保留姓，名用星号 | 张\*\* |
| 邮箱 | @前保留首尾字符 | z\*\*\*g@example.com |
| 银行卡 | 保留后4位 | \*\*\*\*\*\*\*\*\*\*\*\*\*1234 |
| 地址 | 保留省市，详细地址星号 | 上海市浦东新区\*\*\*\*\*\* |
| 全掩码 | 全部替换 | \*\*\*\*\* |
| 自定义正则 | 用户配置正则 + 替换模板 | 灵活配置 |

- 查询结果**默认脱敏生效**
- **脱敏行为由权限控制**：通过 Casbin RBAC 策略，拥有 `desensitize:bypass` 权限的用户可查看原始数据
- 没有 bypass 权限的用户，查询和导出均自动脱敏
- 拥有 bypass 权限的用户导出时，可选择是否包含脱敏数据（默认包含，需主动勾选"导出原始数据"）
- 所有 bypass 操作均记录审计日志
### 5. 变更工单

- DDL（建表、改表）和 DML（UPDATE/DELETE）必须提交工单
- MongoDB `updateMany` 走工单，`updateOne` 也走工单（MVP 统一管理）
- 工单流程：提交 → AI 评审 → DBA 审批（单级）→ 执行 / 驳回
- AI 评审结果决定走**简化工单**还是**标准工单**
- **支持取消**：提交人或 DBA 可在审批前取消工单
- **审批通过后需手动执行**：DBA 审批通过后，工单进入 APPROVED 状态，需提交人或 DBA 手动点击「执行」按钮才真正执行 SQL。这样 DBA 可以在审批通过后选择合适的执行时机
- **执行权限**：仅工单提交人或 dba/admin 角色可执行工单 SQL
- 工单状态透明可追踪，所有状态变更记录时间戳和操作人

**工单状态机：**
```
SUBMITTED → AI_REVIEWED → PENDING_APPROVAL → APPROVED → EXECUTING → DONE
                                            → REJECTED   → DONE
                                            → CANCELLED  → DONE
```

> MVP 不做草稿功能，提交即进入 SUBMITTED 状态。

**取消规则：**
- 工单在 `SUBMITTED`、`AI_REVIEWED`、`PENDING_APPROVAL` 状态下可被提交人或 DBA 取消
- 已执行（`EXECUTING`、`DONE`）的工单不可取消
- 取消需填写原因

**简化工单 vs 标准工单：**

| 维度 | 简化工单 | 标准工单 |
|------|---------|---------|
| AI 评审 | 附带建议，风险可控 | 附带完整影响分析 |
| DBA 操作 | 一键审批 | 需查看详情后审批 |
| 适用场景 | 加索引、加字段等低影响操作 | 删表、改字段类型、批量更新 |

### 6. 用户管理（MVP）

> MVP 采用**用户名 + 密码 + JWT**方案。
> 首次启动时通过命令行参数或环境变量配置初始管理员账号和密码。

- 初始管理员通过启动参数配置（`--admin-username`、`--admin-password` 或环境变量 `ADMIN_USERNAME`、`ADMIN_PASSWORD`）
- 首次启动时自动创建管理员账号，后续启动检测到已存在则跳过
- 管理员登录后可添加其他用户（设置用户名、密码、角色）
- 角色分为：**admin**（系统管理）、**dba**（审批 + 配置）、**developer**（查询 + 提交工单）
- 所有用户通过用户名 + 密码登录
- 密码使用 bcrypt 哈希存储
- 密码策略：长度 8-128 字符，至少包含字母和数字
- 登录成功后签发 JWT（Access Token），前端存储在 localStorage，每次请求通过 `Authorization: Bearer <token>` 携带
- JWT 有效期默认 24 小时，支持配置
- 后续版本接入钉钉 OAuth 正式认证

### 7. 权限管理（Casbin RBAC）

> MVP 使用 [Casbin](https://casbin.org/) 实现基于角色的访问控制（RBAC）。

**Casbin 模型设计（RBAC with domains）：**

```ini
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) && r.dom == p.dom && (r.obj == p.obj || p.obj == "*") && (r.act == p.act || p.act == "*")
```

**模型说明：**
- `sub`：用户或角色
- `dom`：数据源/库名（domain，多数据源隔离）
- `obj`：资源对象（表名，`*` 代表全部）
- `act`：操作类型（`select`、`update`、`delete`、`ddl`、`export`、`desensitize:bypass`）
  > 注：MVP 阶段不支持 INSERT 直连执行，INSERT 需通过工单流程，因此不纳入 Casbin 权限控制

**内置角色：**

| 角色 | 说明 | 默认权限 |
|------|------|----------|
| admin | 系统管理员 | 全数据源全操作 + 用户管理 + 数据源管理 |
| dba | 数据库管理员 | 全数据源读写 + 审批工单 + 脱敏豁免 |
| developer | 开发人员 | 被授权的数据源只读 + 提交工单 |

**关键设计：**
- 敏感表默认拒绝所有角色访问，需管理员显式授权
- `desensitize:bypass` 权限控制脱敏豁免，与角色解耦——任何拥有该权限的用户都可查看原始数据
- 权限粒度：数据源（domain）→ 表（obj）→ 操作（act），支持 `\*` 通配
- 权限变更由 admin 或 dba 操作，变更记录写入审计日志
- Casbin 策略存储在 SQLite，使用 Ent ORM Adapter

### 8. 操作审计

- 全量记录：谁、什么时间、什么 SQL、影响行数、执行耗时、评审结果
- 数据导出记录：谁导出了什么、多少条
- 审计日志应用层面不支持删除（通过 API 限制）
- 支持按用户/时间/数据源/操作类型筛选查询

### 9. 通知集成（钉钉）

- 工单提交 / 审批结果通过钉钉机器人 Webhook 通知
- 中/高风险操作实时告警到 DBA 钉钉群
- 通知内容包含：操作人、SQL 摘要、风险等级、工单链接

---

## MVP 不做的事

- ❌ 测试环境管控
- ❌ Elasticsearch 操作
- ❌ 自然语言转 SQL
- ❌ 多级审批流
- ❌ SQL 智能推荐
- ❌ 数据库性能监控
- ❌ 钉钉 OAuth（MVP 仅用户名+密码+JWT）
- ❌ 审计日志防篡改（仅应用层限制）
- ❌ 正式部署方案（MVP 仅 Docker 内网运行）

---

## 非功能性需求

> 技术架构和非功能性需求见 [ARCHITECTURE.md](./docs/ARCHITECTURE.md)
