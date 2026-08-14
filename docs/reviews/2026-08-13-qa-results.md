# SQLFlow QA 执行结果与缺陷汇总

- 日期：2026-08-13
- 对象：`http://localhost:8090`（admin/admin123，PostgreSQL 平台库）
- 方法：Playwright headless Chromium（浏览器级）+ curl（API 级）
- QA 清单：[2026-08-13-qa-plan.md](2026-08-13-qa-plan.md)

## 总览

| 维度 | 结果 |
|------|------|
| 测试模块 | IAM / 数据源 / 查询 / 高危回归 / 工单 / 审计运维 / 分享 |
| 通过项 | 登录、查询执行、EXPLAIN、多语句防护、EXPLAIN 写入防护、DDL 拦截、自审批拦截、sql_hash 防篡改、工单全生命周期、审计检索、报表、分享密码防护（未带密码不泄露） |
| 新发现缺陷 | 6 个（1 High / 4 Medium / 1 Low） |
| 环境问题 | 1 个（encryption_key 不匹配，已临时修复） |
| 已知问题确认 | dba 审计访问（与 REV-P1-012 相关） |

## 已修复项回归确认（全部通过）

| 回归点 | 验证 | 结果 |
|--------|------|------|
| REV-P0-001 分享密码绕过 | 未带密码访问 `/s/:token` | ✅ 仅返回 `has_password/access_granted:false`，columns/rows 空 |
| REV-P0-004 多语句注入 | `SELECT 1; DROP TABLE` | ✅ 未执行（DROP 没生效），但错误码 500（见 FINDING-3） |
| REV-P0-004 字符串内分号 | `SELECT 'a;b'` | ✅ 正常返回 `a;b`，未截断 |
| Wave-0 EXPLAIN 写入 | DELETE via EXPLAIN | ✅ 400 "EXPLAIN 仅支持 SELECT"，admin 安全 |
| Wave-0 自审批 | 提交者审批自己工单 | ✅ 403 "不能审批自己提交的工单" |
| REV-P1-006 客户端伪造风险 | 工单带 risk_level=low | ✅ 服务端忽略，DROP 自行分级 |
| REV-P1-017 sql_hash 篡改 | 审批后改 SQL 再执行 | ✅ "SQL 与审批版本不一致"，DROP 被拦截，表安全 |

## 修复状态

| Finding | 严重度 | 状态 | 修复内容 |
|---------|--------|------|----------|
| FINDING-1 同秒建号登录失效 | High | ✅ 已修复 | `auth.go` 中间件：`password_changed_at` 比较前 `.Truncate(time.Second)`；补回归测试 `TestAuthService_CreateUser_ThenLoginSameSecond` |
| FINDING-2 重复用户名 500 | Medium | ✅ 已修复 | `iam/auth.go`：识别 `ent.IsConstraintError` 返回 `ErrUsernameExists`；handler 映射 409；修正固化旧行为的测试断言 |
| FINDING-3 多语句注入 500 | Medium | ✅ 已修复 | `query/error_status.go`：`sqlparser.ErrMultipleStatements` 映射为 400 |
| FINDING-4 sql_hash 篡改 500 | Medium | ✅ 已修复 | 新增哨兵 `ticket.ErrSQLHashMismatch`，hash 校验失败返回它；映射表加 403；守护测试同步 |
| FINDING-5 无表分享读取 500 | Medium | ✅ 已修复 | `share.go`：`targets` 为空（查询不涉及表）时安全放行（无字段可命中脱敏规则）；`ErrShareUnmaskable` 在 handler 映射 403 |
| FINDING-6 dba 读数据源 | — | ⚪ 非缺陷 | 见下方澄清 |
| FINDING-7 RequireScope bypass | Low | 📌 已知 | 与 REV-P1-002/012 相关，单独跟踪 |

## FINDING-6 澄清：非缺陷（正确设计）

原报告将"dba `GET /api/datasources` 返回 403"记为缺陷。复核后确认这是**正确的最小权限设计**：

- `GET /api/datasources`（管理列表，含加密凭据）→ 需要 `system:datasources:manage`，仅 admin → dba 403 ✅
- `GET /api/datasources/available`（精简列表，仅 id/name/type/status，**不含凭据**）→ dba **200** ✅
- `GET /api/datasource-types` → dba **200** ✅

dba 通过 `available` 端点即可获得审批工单所需的数据源信息，且不暴露敏感凭据。这是设计意图，非缺陷。

## 新发现缺陷

### FINDING-1 [High] 同秒创建用户并登录，token 立即失效

- **模块**：IAM / 认证中间件
- **复现**：`POST /api/users` 创建用户后，**1 秒内** `POST /api/auth/login`，所得 token 调用任何端点均返回 401
- **响应**：`{"error":"密码已被重置，请重新登录"}`
- **根因**：`users.password_changed_at`（带微秒精度）> JWT `iat`（整秒截断）。中间件判断"token 签发早于密码变更"即判失效，但同秒内 `iat` 整秒 < `password_changed_at` 微秒，误判
- **影响**：所有新建用户的首次登录会失败（必须等到下一秒）。阻断开发者/DBA 账号开通流程
- **验证**：等待 3 秒后重新登录，token 即正常 → 确认是时间戳比较问题
- **建议修复**：比较时用 `iat <= password_changed_at`（含等号）或把 `password_changed_at` 也截断到秒

### FINDING-2 [Medium] 重复用户名创建返回 500

- **模块**：IAM / 用户管理
- **复现**：`POST /api/users` 用已存在的 username
- **响应**：`{"code":500,"message":"创建用户失败"}`
- **预期**：409 Conflict（或 400），明确"用户名已存在"
- **根因**：数据库 unique 约束冲突未被捕获映射为业务错误，落入通用 InternalError
- **影响**：错误码误导（看起来像服务端故障），可能泄露后端信息；运维排查困难

### FINDING-3 [Medium] 多语句注入返回 500 而非 400/422

- **模块**：查询执行
- **复现**：`POST /api/query/execute` 发 `SELECT 1; DROP TABLE x`
- **响应**：`{"code":500,"message":"查询执行失败"}`
- **安全状态**：✅ 注入未执行（DROP 没生效，安全目标达成）
- **问题**：应返回 400/422 "拒绝多语句"，而非 500（500 暗示服务端内部故障，且 trigger 告警噪音）
- **根因**：PG 驱动拒绝多语句的错误未被识别为"客户端请求错误"，落入 InternalError

### FINDING-4 [Medium] sql_hash 篡改返回 500 而非 403

- **模块**：工单执行
- **复现**：审批后篡改 `tickets.sql_content`，再 `POST /api/tickets/:id/execute`
- **响应**：`{"code":500,"message":"执行工单失败"}`
- **安全状态**：✅ 篡改被拦截（日志："SQL 内容与审批版本不一致"），DROP 未执行
- **问题**：这是个明确的客户端/安全拒绝场景，应返回 403 或 409，而非 500
- **根因**：hash 校验失败 error 未被映射为 `Forbidden`/`Conflict`，落入 InternalError

### FINDING-5 [Medium] 分享密码验证后无法用 cookie 访问数据

- **模块**：查询分享
- **复现**：`POST /s/:token/verify` 正确密码 → 200 "密码验证成功" → 带 Set-Cookie 再 `GET /s/:token` → 500
- **响应**：`{"code":500,"message":"获取共享链接失败"}`
- **影响**：密码保护功能形同虚设——验证通过后实际取不到数据，分享无法使用
- **根因**：待排查（cookie 设置/读取、或验证后查询 share 记录的逻辑）

### FINDING-6 [Medium] dba 角色无法读取数据源列表

- **模块**：数据源 / RBAC
- **复现**：dba token `GET /api/datasources` → 403
- **影响**：dba 需要查看数据源信息才能审批工单；403 阻断审批上下文
- **预期**：dba 应能读数据源列表（只读），写操作才限 admin
- **根因**：Casbin 策略中 dba 对 `datasource` 的 `read` 权限缺失，或路由的权限要求过严

### FINDING-7 [Low] RequireScope 对 JWT 会话用户完全 bypass

- **模块**：鉴权中间件
- **事实**：`RequireScope` 仅对 API Token 生效；JWT 登录用户直接 `return next(c)` 绕过 scope 检查
- **具体表现**：dba 用 JWT 访问 `auditAdminGroup`（`RequireScope("admin")` + `SystemPermission(audit:view)`）下的 `/api/audit-logs` → 200，能看到全部审计日志
- **关联**：与已知问题 REV-P1-002（token scope 未覆盖多路由）、REV-P1-012（审计导航仅 UI 隐藏）重叠
- **定性**：非新缺陷，是已知架构限制的具体体现。但审计日志对 dba 可见属于权限过度，建议在汇总中标注

## 环境问题（非代码缺陷）

### ENV-1 存量库 encryption_key 不匹配致内置数据源失效

- **现象**：内置 PG 元数据库连接测试/表浏览失败，报 `decrypt: cipher: message authentication failed`
- **根因**：PG 库 2026-08-09 创建时用的 encryption_key 与当前 `config.yaml` 的 key 不同，已加密的数据源密码无法解密
- **影响**：换 key 后所有已加密数据源凭据失效，无迁移机制
- **处理**：已用当前 key 重新加密内置数据源密码，临时修复。**建议**：增加 encryption_key 轮换/迁移工具，或在文档明确"换 key 需重配所有数据源"
- **定性**：本次测试环境的存量数据问题，非代码 bug。但"换 key 无迁移"是真实运维风险

## 通过项明细（节选）

| 模块 | 项 | 结果 |
|------|----|------|
| IAM | admin 登录 / 错密码 401 / 未登录重定向 / 登录页渲染 | ✅ |
| 查询 | SELECT 执行 / EXPLAIN 返回计划 / 查询历史 / DDL 拦截 403 / 字符串分号 | ✅ |
| 工单 | 提交(201) / 审批推进 / 执行(DONE+真实建表) / 自审批 403 / hash 防篡改 | ✅ |
| 审计 | 检索 200 / 全文检索 200 / 报表 200 | ✅ |
| 运维 | /healthz 200 / /readyz 含依赖检查 / /api/health | ✅ |
| 分享 | 创建 / 未带密码不泄露 / 错密码 401 / 正确密码 200 | ✅（但见 FINDING-5） |

## 建议修复优先级

1. **FINDING-1**（High）——同秒登录失效，阻断新用户开通
2. **FINDING-5**（Medium）——分享密码验证后取不到数据，功能不可用
3. **FINDING-2 / 3 / 4**（Medium）——错误码不当系列，集中修复
4. **FINDING-6**（Medium）——dba 读数据源
5. **FINDING-7**（Low）——已知架构限制，单独跟踪
