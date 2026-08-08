# SQLFlow 整改与交付路线图

本路线图把 [2026-07-26 跨角色评审](reviews/2026-07-26-cross-functional-review.md) 转换为可验收的交付顺序。优先级依据是安全暴露、核心用户旅程、数据一致性和可恢复性，不代表工期承诺。

状态列以 [2026-07-31 整改进度复核](reviews/2026-07-31-implementation-verification.md) 的代码验证结果为准。该复核确认 P0 关闭 2/6、P1 关闭 3/12，并新增 REV-P1-013 ~ REV-P1-017。

## 发布分级

| 等级 | 允许场景 | 必须满足 |
|---|---|---|
| L0 开发验证 | 本地、隔离测试数据 | 可启动、基础测试通过，明确使用示例凭据 |
| L1 内部试用 | 受控内网、非关键数据 | 阶段 0 全部完成；无未接受的 P0 |
| L2 生产候选 | 生产同构预发布环境 | 阶段 1、阶段 2 全部完成；恢复演练和安全回归通过 |
| L3 生产运行 | 受监控的单实例生产环境 | 发布审批、回滚方案、SLO/告警、值班与定期恢复演练有效 |

当前评审等级为 **L0**。

## 阶段 0：安全止血与主流程恢复（P0）

目标：消除直接数据暴露和默认配置风险，让 Developer/DBA 的只读查询主流程真正可用。

| 顺序 | 工作包 | 关联问题 | 状态 | 验收出口 |
|---|---|---|---|---|
| 0.1 | 修复密码分享协议，可选地在修复前临时禁用密码分享/公开分享 | REV-P0-001 | **已完成** | 未验证请求不返回结果；密码、过期、撤销回归通过 |
| 0.2 | 新增“当前用户可访问数据源”接口，修复页面错误态 | REV-P0-002 | 实现完成，验证方式待定 | Developer/DBA 可选择授权数据源，凭据和管理字段不可见 |
| 0.3 | 统一 Casbin 元组和通配语义，新增授权 ADR | REV-P0-003 | **已完成** | 空库种子角色矩阵与服务端授权测试通过 |
| 0.4 | 查询入口拒绝或完整验证多语句 | REV-P0-004 | **已完成**（2026-08-08） | 写语句拼接被拒绝；`WHERE name = 'a;b'` 可正常执行；含 `$$` 函数体的 PG 工单完整执行 |
| 0.5 | 分离开发与生产 Compose，移除固定 root 暴露 | REV-P0-005 | 部分完成（07-31 复核） | 生产模板无示例库外露和已知凭据 |
| 0.6 | 建立一致性备份与恢复最小闭环 | REV-P0-006 | **已完成** | 备份走 `pg_dump`；`TestBackupRestoreRoundTrip` 每次 CI 都把备份还原进独立库并核对行数、内置角色、检索函数与序列 |

0.5 已完成部分：`MYSQL_ROOT_PASSWORD` 改为必填，不再提供固定示例密码。剩余：默认发布 `3306`、示例库仍以 root 运行且是 `sqlflow` 的启动依赖、未用 Compose `profiles` 分离开发与生产模板。

0.4 已完成内容（2026-08-08）：语句边界归驱动（`StatementSplitter`），分析端与执行端消费同一个 `ticketPlan`，`strings.Split(sql, ";")` 已删除。MySQL/SQLite 用手写词法扫描器（vendored 的 pingcap/parser 拒绝 `ALTER TABLE ... RENAME COLUMN`、CTE、窗口函数，拿它当分词器会让普通变更工单提不了）；PostgreSQL 用 `pgquery.SplitWithParser`，`$$` 与 `BEGIN ATOMIC` 函数体不再被切碎。回归见 `internal/platform/sqlparser/split_test.go` 与 `internal/ticket/ticket_statement_split_test.go`。

同期补上一处相关缺口：切分器把 `/*!nnnnn ... */` 当代码扫描（服务器会执行它），而定级器的 `normalizeSQL` 曾把它连同普通注释一起删掉，于是 `/*!50000 DROP TABLE users */` 评 OTHER/medium 而 MySQL 照常执行 DROP。见 `internal/ticket/grading_test.go`。

阶段出口：

- 所有 P0 有自动化回归和代码审查记录。
- `FR-QRY-001`、`FR-QRY-007`、`FR-SEC-001` 重新达到 `implemented`。
- 隔离预发布环境完成 Developer 查询旅程和一次恢复演练（Playwright 套件已移除，需先确定替代验证方式）。

## 阶段 1：核心治理完整性（P1）

目标：让授权、工单、导出和多数据源路径符合服务端安全与状态机约束。

工作包按建议实施顺序排列。顺序依据是"功能完全不可用"优先于"行为不完整"，再到"降低攻击面"。

| 顺序 | 工作流 | 工作包 | 关联问题 | 状态 |
|---|---|---|---|---|
| 1.1 | 调度 | 单一 CAS 状态机；调度器先读完 ID 再执行 | REV-P1-003 | **已完成**（2026-07-31） |
| 1.2 | 调度 | 执行租约与崩溃恢复；排查全仓库同类嵌套读写模式 | REV-P1-003、015 | **已完成**（2026-08-08） |
| 1.3 | 工单 | 补齐 `sql_hash` 读取路径使审批后 SQL 篡改检测生效；收敛重复的工单列清单 | REV-P1-017 | **已完成**（2026-07-31） |
| 1.4 | 工单 | 服务端无条件重算风险；审批/驳回/重提事务化 + CAS；重提走完整分析链并刷新 `sql_type`/`affected_tables` | REV-P1-006、007 | **已完成**（2026-07-31） |
| 1.4b | 工单 | 审批记录与状态迁移纳入同一事务 | REV-P1-007 余项 | **已完成**（2026-08-08） |
| 1.5 | 运行时 | 独立 `data_dir` 配置与启动期可写检查；TLS/非 TLS 统一生命周期；查询超时与行数上限提为配置并在 Service 层统一施加 | REV-P1-008、011、013 | **已完成**（2026-08-08） |
| 1.6 | 授权 | 补齐剩余路由的 Token Scope；工单创建增加数据源级门禁；菜单按服务端能力矩阵渲染 | REV-P1-002、012、014 | 部分完成（数据源级门禁与分享路由 Scope 已完成；慢查询路由 Scope 与归属边界已补 2026-08-09；菜单渲染待实施） |
| 1.7 | 多数据源 | 用 Driver Capability 驱动表单与执行路径 | REV-P1-009 | **已完成**（2026-08-08） |
| 1.8 | API/UX | 统一领域错误映射 | REV-P1-010 | 部分完成 |
| — | 授权 | 统一临时权限、资源所有权与私有模板可见性 | REV-P1-001、004、005 | **已完成** |

1.1 已完成内容：调度器改为只查到期工单 ID、读尽游标后再执行，`SCHEDULED → EXECUTING` 迁移由 `TicketService.executeTicket` 单点持有。同时消除了调度器与 `scanTicket` 的列数不匹配。回归见 `internal/ticket/scheduler_test.go`（6 个用例，含真实 SQL 执行与幂等验证）。

1.3 已完成内容：工单列清单收敛为 `ticketColumns` 常量并补入 `sql_hash`，`GetTicket`/`ListTickets` 统一引用；重提时清空哈希；`ListTickets` 的扫描失败不再被静默吞掉。回归见 `internal/ticket/ticket_sql_hash_test.go`（6 个用例）。撤销修复后重跑可复现"篡改语句真实到达目标库"。

1.1 与 1.3 共享同一根因——工单列清单在多处手工重复（DEBT-07）。该模式已消除，后续新增工单字段只需改动一处。

1.2 已完成内容（2026-08-08）：`tickets` 增加 `lease_owner` / `lease_expires_at`，认领与租约在同一条语句里落库——分两步会留下"崩溃时工单在 EXECUTING 但没有可过期的租约"这一窗口，正是本机制要消除的那种卡死。`ReclaimExpiredExecutions` 在启动时与每个调度周期回收过期租约，把工单置为 FAILED 并留审计与原因。**回收到 FAILED 而不是退回 APPROVED**：没有任何一方知道语句是否已经到达目标库，重跑一条已经生效的 DDL 比让人来看一眼更糟。回归见 `internal/ticket/lease_test.go`（4 个用例，含"活着的租约不得被回收"）。

REV-P1-015（SQLite 单连接下的嵌套读写）随 ADR-0009 迁移到 PostgreSQL 后失效：`internal/db` 的连接池为 25，不再存在"读游标占住唯一连接"的前提。

1.5 已完成部分（2026-08-08）：`data_dir` 是独立配置项，导出目录在启动期创建并做可写探测，失败即 fail fast——`os.MkdirAll` 的错误此前被 `_ =` 丢弃，故障要等到用户点击导出才暴露。TLS 与非 TLS 共用同一套启动/关闭路径：`main` 只决定退出码，其余在 `run` 里，因为 `log.Fatalf` 会 `os.Exit` 而 `os.Exit` 不跑 defer——两个分支此前都以 Fatalf 收尾，于是一次干净的 SIGTERM 关闭仍会跳过 `container.Close()` 与 `database.Close()`，调度器不停、连接池不排空；而优雅关闭本身让 `StartTLS` 返回 `http.ErrServerClosed`，又撞进同一个 Fatalf，成功的关闭得到退出码 1。Echo Server 现在配了 Read/Write/Idle 超时（此前只有 HTTP→HTTPS 重定向监听器有）。超时与行数上限收进 `config.QueryConfig` → `query.Limits`，**由 Service 施加**：
此前 30 秒只罩着 `poolMgr.Get`，执行拿的是调用方的无期限 ctx，注释写着「驱动自己会限」
——五个驱动三种答案：SQLite 与 ES 有 30 秒，PostgreSQL 一点没有，MySQL 的 `Timeout`
是拨号超时，对已经开始的查询不起作用。于是慢查询一直跑到客户端放弃，连接池位子也一直占着；
而调用方那条 `DeadlineExceeded` 分支永远不可能触发，因为它检查的 ctx 根本没有截止时间。
建连与执行共用一个预算，因为用户等的是两段之和。导出路径此前连 30 秒都没有，同样补上。
全部字段可省略，省略即取原先写死的值。

1.4b 已完成内容（2026-08-08）：`ProcessApproval` 的状态迁移与审批记录写入收进同一事务。此前是两条独立语句，中间失败会留下一张已决策但没有决策记录的工单，而审批链视图正是读这些记录的。

1.7 已完成内容（2026-08-08）：`ConfigSchema() []ConfigField` 进入 `Driver` 接口（与 `QueryForm()` 同级，强制而非可选），`GET /api/datasource-types` 在任何数据源存在之前就把连接表单的字段、校验与载荷归属交给驱动声明。前端表单从 1321 行降到 916 行、0 个按类型名的分支、15 个测试；`eslint.config.js` 的 `noDatasourceTypeBranching` 是 `internal/arch` 那条规则的前端另一半。见 [ADR-0011](adr/0011-optional-interfaces-as-driver-capabilities.md)。

1.4 已完成内容：`risk_level` 与 `ai_review_result` 从创建请求与 `CreateTicket` 签名中移除，风险由 `RiskEvaluator` 无条件派生；新增基于 ent 谓词式 `Update()` 的 `casTicketStatus`，审批/驳回/重提/多阶段审批全部改为 CAS；重提在单事务内完成快照与迁移，并复用 `applyApprovalPolicy` 重跑完整分析链。补齐了 1.3 遗漏的缺口——自动审批与多阶段末阶段审批此前不写 `sql_hash`，现在所有进入 `APPROVED` 的路径都固定哈希。回归见 `internal/ticket/ticket_governance_test.go`（6 个用例）与 `TestTicketHandler_CreateTicket_IgnoresClientRiskLevel`。

**契约变更**：`POST /api/tickets` 不再接受 `risk_level` 和 `ai_review_result`。前端 `CreateTicketRequest` 已同步；第三方调用方若发送这两个字段将被静默忽略。

测试要求（REV-P1-016）贯穿全部工作包：调度执行、并发审批、多语句拒绝、Scope 矩阵、导出目录五条路径必须有带真实断言的测试，不接受烟雾级用例。其中调度执行已随 1.1 完成。

阶段出口：

- Developer、DBA、Admin 三角色的查询、提单、审批、执行、导出、分享和临时授权旅程通过。
- 所有资源按所有者、审批职责或管理角色隔离；负向越权测试通过。
- 定时工单能在真实 PostgreSQL 上从 `SCHEDULED` 走到 `DONE`；重复触发只执行一次。
- 调度任务重启后不会丢失、重复或永久卡在 `EXECUTING`。
- API Token 的每个 Scope 有明确允许/拒绝测试。
- 客户端无法通过任何请求字段影响风险等级或审批路径。

## 阶段 2：生产运行保障

目标：从“可部署”升级到“可监控、可恢复、可安全发布”。

| 主题 | 必做项 | 验收出口 |
|---|---|---|
| 密钥与网络 | 拒绝已知默认密钥；收紧 CORS；私有化指标；日志脱敏 Token/OIDC 参数 | 安全配置检查和日志泄漏测试通过 |
| 可观测性 | 统一结构化日志/request ID；修正指标更新与标签；定义 SLI/SLO、Dashboard 和告警 | 注入故障可触发可行动告警 |
| 健康与容量 | 统一 readiness；磁盘/PostgreSQL/导出/备份容量阈值；版本来自构建信息 | 探针反映真实依赖，容量告警可演练 |
| 备份恢复 | 加密、异地副本、保留策略、恢复工具和季度演练 | 达到书面 RPO/RTO 并保存演练证据 |
| 交付链 | 构建—扫描—签名—推送顺序；固定 Action 版本；依赖锁定；迁移预检和回滚 | 漏洞门禁能阻止发布，预发布升级/回滚通过 |
| 运行手册 | 启停、升级、回滚、恢复、凭据轮换、故障排查和值班升级 | 非开发人员按手册完成演练 |

## 阶段 3：规模化与合规增强

仅在真实需求出现时启动：

- 评估外部元数据库、任务租约和多副本；决策前保留模块化单体。
- 为 `Driver` 增加索引/字段浏览能力后移除 `internal/connpool` 最后一处用途；收敛原始 SQL/Ent 双轨债务。
- 建立审计不可抵赖、长期留存、归档和合规导出能力。
- 按组织要求补充 SSO 强制策略、密钥托管、数据分级和审批职责分离。

## 重构债务

[2026-07-31 复核](reviews/2026-07-31-implementation-verification.md#6-重构债务) 记录的 DEBT-01 ~ DEBT-07 不单独立项，在对应功能切片内顺带完成：

| 债务 | 建议归属 |
|---|---|
| DEBT-01 连接层双轨、`query.go` 不可达 fallback | 阶段 1.5 — **基本完成**：仅 ES 元数据浏览仍用 `connpool` |
| DEBT-02 `ExecuteQuery` 依赖变量遮蔽的控制流 | 阶段 1.5 — **已完成** |
| DEBT-05 领域错误映射不统一 | 阶段 1.8 |
| DEBT-06 超时/行数上限三处硬编码 | 阶段 1.5 |
| DEBT-07 工单 SELECT 列清单多处重复 | 阶段 1.3 — **已完成** |
| DEBT-03 原始 SQL 与 Ent 双轨 | **已完成**（2026-08-04）：141 处改写完毕，守卫见 `internal/arch` |
| DEBT-04 超大 Service 文件与扁平包结构 | 阶段 3 — **已完成**（`internal/service` 已拆为八个领域包） |

## 工作项完成定义

每个整改工作项只有同时满足以下条件才算关闭：

1. 能从评审问题追踪到需求、代码变更和测试证据。
2. 服务端对认证、授权、输入、所有权和状态迁移做最终裁决。
3. 覆盖成功路径、关键失败路径、越权路径和重试/并发路径。
4. 数据或配置变更具备升级、兼容和回滚方案。
5. 日志和审计足以定位问题，且不泄露凭据、Token、结果数据或服务器路径。
6. 需求状态只在端到端验收通过后升级为 `implemented`。

## 2026-08-09 架构评审整改

七个候选全部落地，外加评审过程中发现的十个可复现缺陷。全部按仓库纪律执行：
先写会失败的用例 → 修 → 反证（把修复改回去，确认用例失败）。反证发现过三次
「测试没有区分力」，那三个用例被重写。

**验证过的缺陷（Wave 0）**

| # | 缺陷 | 性质 |
|---|---|---|
| 1 | `EXPLAIN ANALYZE DELETE FROM t` 经 `/api/query/explain` 真的删行（复现：3 行进 0 行出）；该入口还无超时、无失败审计，handler 漏 5 个错误映射 | 越权写入 |
| 2 | PostgreSQL 执行计划在 UI 上恒显示「无执行计划数据」——服务端按列名嗅探引擎，表头硬编码 MySQL 12 列 | 功能失效 |
| 3 | 慢查询历史无归属过滤且路由无 Scope，`SQLContent` 原文可被任何登录用户读到 | 越权读取 |
| 4 | CSV 导出丢弃列选择（注释声称已修，实际只修了 Excel） | 数据外泄面 |
| 5 | 审批策略后台：编辑任何策略静默清空 `is_default`；启停开关与排序箭头必然 500 | 功能失效 |
| 6 | 提交人可以审批/驳回自己的工单（三个入口都没查） | 治理失效 |
| 7 | `RejectTicket` 完全绕过审批链并清空阶段计数 | 治理失效 |
| 8 | `applyTransition` 的 CAS 结果被两处丢弃：执行输给回收扫描后仍上报 DONE | 状态撒谎 |
| 9 | 评论、审批链、审批历史三个读取入口没有 actor（`ListComments` 的接口根本表达不了） | 越权读取 |
| 10 | 策略匹配失败让工单永久卡在 SUBMITTED（注释描述的人工复核路径不存在） | 死状态 |

**七个深化（C1–C7）**

- **C1** 目标库读取收敛为 `authorizeRead` 一个入口，十一道闸门顺序固定；
  `readPurpose` 只选行数上限、Casbin 动作与审计动作串，**不选哪些闸门运行**。
  `TestTargetDatabaseReadsGoThroughOneSeam` 把第四个入口变成构建失败。
  顺带删掉了两个 handler 恒传 `""` 的 `dbType` 参数。
- **C2** 结果释放收敛为 `releaseRows` 一个决策，替掉四个函数；每次查询的
  Casbin 扫描与规则读取从两次降到一次；`ai_review` 的第三套规则谓词（漏了
  `"*"` 通配与库作用域）改问同一模块。
- **C3** 通知与传输分离：一个事件标识、一个传输中立的值、一个 dispatcher、
  每传输一个 adapter。修好了「12 个事件里 5 个只到钉钉」和「转义按传输各写各的」；
  已实现却无事件源的 outbound webhook 成为第三个 adapter，`scripts/deadcode.allow`
  因此**清空**。
- **C4** 审批条件语言收敛为一套，删掉 224 行只验证自己语法的校验器；
  `environment` 等四个「表单提供但无人实现」的字段删除而不是修补。
- **C5** 错误映射改为数据 + `errors.Is`，14 个 switch 归一；
  `TestEveryDomainErrorHasAStatus` 解析包内声明，任何未映射的导出 `Err*` 构建失败
  ——它第一次跑就找出我漏掉的 5 个。同时堵上 `Extra` 闭包绕过状态机的洞。
- **C6** 删除 ES 专属元数据路径与 `internal/connpool`，以及 API 里**唯一**按类型
  命名的路由。它给自己写的理由（需要分页 `_cat/indices`）经核实是假的。
  `TestNoTypeNamedRoutes` 防止再出现。
- **C7** 补上 `ExecuteStatements` 三种事务语义的测试（此前**零测试**），
  SQLite 那 59 行 `sqlrows.Query` 的重复实现收敛为一行。共享 base struct 的方案
  **拒绝**：那是样板而非正确性，而这个仓库被「隐藏真实差异的共享抽象」坑过。

未做：C5 的 actor 位置参数收敛（`ListTickets` 仍有六个连续 string），
以及 `PreferenceService.ShouldNotify` 仍未接线——它是按用户的，而现有渠道都是
群机器人，接上会让一个人的偏好静音整个频道。它的白名单已改为从事件标识派生，
不会再宣传平台不发的事件。
