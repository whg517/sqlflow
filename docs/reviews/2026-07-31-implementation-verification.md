# 2026-07-31 整改进度复核

> **2026-08-08 校准**：本文写于 `internal/service/` 还存在的时候，所有指向该目录的
> 行号已失效。逐条对当前代码复核后，以下条目已关闭：REV-P0-004（语句切分，见
> ROADMAP 0.4）、REV-P1-003 的租约与恢复余项（ROADMAP 1.2）、REV-P1-007 的事务余项
> （1.4b）、REV-P1-008 的导出目录、REV-P1-011 的进程生命周期、REV-P1-014（工单数据源
> 级门禁）。REV-P1-015 随 ADR-0009 迁移到 PostgreSQL 后**失效**——`MaxOpenConns(1)`
> 这个前提不复存在。仍然开着的是 REV-P1-013（查询超时与行数上限仍是包内常量）与
> REV-P0-005 的 Compose 分离。
>
> 校准的意义不只是记账：一份和代码不符的复核，会让下一个人按它去修已经修好的东西，
> 或者相信一条早已不成立的约束。

| 属性 | 值 |
|---|---|
| 评审范围 | 对 [2026-07-26 跨角色评审](2026-07-26-cross-functional-review.md) 全部 P0/P1 问题的实现复核 |
| 评审方法 | 逐项静态代码追踪；`go build ./...` 与 `go vet ./internal/...` 通过 |
| 总体结论 | **维持有条件不通过 / L0** |
| 整改进度 | P0 关闭 2/6；P1 关闭 3/12、部分修复 1 项；复核期间新修复 REV-P1-003 主体、006、007 与 017 |
| 适用版本 | `main` @ `5efff79` |
| 新增问题 | REV-P1-013 ~ REV-P1-017 |
| 整改入口 | [ROADMAP.md](../ROADMAP.md) |

## 1. 执行摘要

本次复核只做一件事：把 2026-07-26 评审列出的每个问题拿到当前代码里重新验证，确认哪些真的关闭了、哪些只是被记为关闭、哪些仍然原样存在。

结论：

- **已关闭的问题确实关闭了**。分享密码证明、Casbin 规范元组、临时权限授权元组、私有模板所有权、工单资源隔离五项，均能在代码中找到对应实现，不存在"文档先行"的情况。
- **复核开始时，REV-P0-004（多语句）、REV-P1-003（定时工单）、REV-P1-006（客户端风险等级）、REV-P1-007（审批原子性）、REV-P1-008（导出目录）、REV-P1-011（优雅关闭）原样存在**，无任何代码变更。其中 003、006、007 已在复核期间修复；004、008、011 仍未开始。
- **REV-P1-003 的实际严重度高于原评审记录**。原记录认为是"字段不一致 + 前置状态冲突"，本次确认是**三个**独立缺陷叠加（列数不匹配、双重 CAS、单连接下的游标内写入），结果是定时工单在任何情况下都不可能执行成功，且现有测试结构上无法发现。该项主体已在本次复核期间修复并补齐回归，详见 4.2。
- **REV-P1-002（Token Scope）为部分修复**：中间件已挂载到多数路由，但分享、性能、看板、评论等路由仍未覆盖，只读 Token 依然可以创建公开分享链接。
- 新增 5 个问题，其中 REV-P1-013（查询超时未生效）、REV-P1-015（SQLite 单连接下的嵌套读写）和 REV-P1-017（工单 SQL 一致性校验从未生效）属于会静默失效的类型——不报错、不告警，只是不起作用。REV-P1-017 已在复核期间修复。
- 三处已确认的缺陷共享同一个根因：**工单列清单在多处手工重复**（DEBT-07）。已收敛为单一常量。

## 2. 复核结论汇总

| ID | 问题 | 2026-07-26 状态 | 本次复核 | 关键证据 |
|---|---|---|---|---|
| REV-P0-001 | 分享密码可绕过 | 已修复 | **确认关闭** | `internal/service/share.go:164` 未验证时 `AccessGranted=false` 且不返回列/行；`:204` 签发绑定分享的短期证明 |
| REV-P0-002 | 普通用户无法发现数据源 | 待隔离 E2E | **未复核** | 接口与前端改造已在，E2E 执行证据本次未取得 |
| REV-P0-003 | 默认 Casbin 策略无效 | 已修复 | **确认关闭** | `internal/authz` 统一构造；`internal/service/permission.go:355` 角色与个人授权取并集 |
| REV-P0-004 | 分析对象 ≠ 执行对象 | 待实施 | **仍然存在** | 见 4.1 |
| REV-P0-005 | 生产示例暴露 root 凭据 | 待实施 | **部分修复** | 见 4.5 |
| REV-P0-006 | 备份不可证明可恢复 | 待实施 | **仍然存在** | 见 4.6 |
| REV-P1-001 | 临时权限不参与查询 | 待实施 | **确认关闭** | `internal/service/permission_request.go:156` 使用 `authz.UserSubject`/`DatasourceDomain`；`permission.go:372-388` 每次决策校验到期 |
| REV-P1-002 | API Token Scope 未执行 | 待实施 | **部分修复** | 见 4.7 |
| REV-P1-003 | 定时工单无法执行 | 待实施 | **主体已修复**，租约与恢复待做 | 见 4.2 |
| REV-P1-004 | 工单缺少资源隔离 | 待实施 | **确认关闭** | `internal/api/handler/ticket.go:100` 使用 `GetTicketForActor` 并映射 403/404 |
| REV-P1-005 | 私有模板可被他人读取 | 待实施 | **确认关闭** | `internal/api/handler/sql_template.go:109,275` 使用 `GetTemplateForUser`/`RenderTemplateForUser` |
| REV-P1-006 | 风险等级受客户端影响 | 待实施 | **已修复** | 见 4.3 |
| REV-P1-007 | 审批与重提缺少原子性 | 待实施 | **已修复**，审批记录事务化待做 | 见 4.4 |
| REV-P1-008 | 异步导出目录错误 | 待实施 | **仍然存在** | 见 4.8 |
| REV-P1-009 | 多数据源工单前端路径不完整 | 待实施 | **未复核** | 本次未审前端工单表单 |
| REV-P1-010 | 权限拒绝返回 500 | 待实施 | **部分修复** | 工单详情已映射领域错误，多数 Handler 仍以 `InternalError` 兜底 |
| REV-P1-011 | 非 TLS 缺少优雅关闭 | 待实施 | **仍然存在（新增证据）** | 见 4.9 |
| REV-P1-012 | 菜单与 API 边界不一致 | 待实施 | **仍然存在** | `web/src/components/Layout.tsx:261` 仅"用户管理"按角色隐藏 |

## 3. 阶段 0 出口条件复核

阶段 0 的三项出口条件目前均不成立：

- `FR-QRY-001`、`FR-QRY-007`、`FR-SEC-001` 依赖 REV-P0-004 关闭，该问题未动。
- 阶段 0 的 6 个工作包中 0.4、0.6 完全未开始，0.5 部分完成。
- 恢复演练无法进行——恢复工具尚不存在。

## 4. 仍然阻断的问题

### 4.1 REV-P0-004 分析对象 ≠ 执行对象（仍然存在）

- **证据**：
  - `internal/pkg/sqlparser/mysql.go:42` 与 `internal/pkg/sqlparser/postgresql.go:31` 用 `strings.IndexByte(sql, ';')` 把输入截断到第一个分号，随后只解析截断结果。
  - `internal/service/query.go:230` 把**完整未截断**的 `sqlContent` 交给 Driver 执行。
  - `internal/service/ticket_executor.go:398` 的 `splitStatements` 是 `strings.Split(sqlContent, ";")`，与解析器同源缺陷。
- **影响**：
  - 门禁面：operation 判定、表级 Casbin 校验（`query.go:174`）、风险评估和脱敏表清单全部只覆盖第一条语句。
  - 可用性面：`strings.IndexByte` 不识别字符串字面量，`SELECT * FROM t WHERE name = 'a;b'` 会被截成 `SELECT * FROM t WHERE name = 'a` 并以语法错误被拒——合法查询被误杀。
  - 执行面：工单路径的 `splitStatements` 会切碎 PostgreSQL 的 `$$ ... $$` 函数体、触发器定义以及字符串/注释中的分号，导致 **DDL 部分执行**且不可回滚。
- **当前缓解（不构成关闭理由）**：MySQL DSN 未启用 `multiStatements`（`internal/driver/mysql/mysql.go:49`），pgx 走扩展协议，因此拼接多语句目前不会被目标库复合执行。缓解来自驱动默认值，而非平台门禁。
- **关闭标准**（在原标准上补充）：
  - 引入可识别字符串字面量、注释和美元引用的语句分词器，解析、门禁、审计、执行共用同一份规范化语句集合。
  - 查询入口对多语句显式拒绝并返回稳定领域错误。
  - 回归用例必须包含：`WHERE name = 'a;b'` 可正常执行；含 `$$` 函数体的 PostgreSQL 工单可完整执行。

### 4.2 REV-P1-003 定时工单无法执行（主体已修复，租约与恢复待做）

原记录的根因描述不完整。当前代码中存在**三个各自独立、都足以致命**的缺陷。三者叠加的结果是定时执行能力 100% 不可用，且工单一旦进入 `EXECUTING` 就无法取消或重试。

- **缺陷 A：SELECT 与 `scanTicket` 列数不匹配**（原评审已记录）
  - `internal/service/scheduler.go:76` 的 SELECT 列出 17 列，`internal/service/ticket.go:127` 的 `scanTicket` 期望 20 列（缺 `sql_type`、`affected_tables`、`revision`）。
  - 每条到期工单在扫描阶段即失败：`sql: expected 17 destination arguments in Scan, not 20`，随后被 `continue` 跳过，`RunOnce` 仍返回 `nil`。
  - 这是最先触发的缺陷，它掩盖了后面两个。
- **缺陷 B：双重 CAS 互相否定**
  - `internal/service/scheduler.go:110` 先执行 `UPDATE tickets SET status='EXECUTING' WHERE id=? AND status='SCHEDULED'`。
  - 随后调用的 `internal/service/ticket.go:600` `executeTicket` 自带 CAS，条件为 `WHERE id=? AND status IN ('APPROVED','SCHEDULED')`。此时状态已是 `EXECUTING` → 影响行数 0 → 返回 `ErrTicketNotExecutable`，工单被永久留在 `EXECUTING`。
- **缺陷 C：SQLite 单连接下的游标内写入**
  - `internal/db/db.go:49` 设置 `SetMaxOpenConns(1)`。
  - `internal/service/scheduler.go:73` 打开 `rows` 游标后，在 `:85` 的迭代体内发起写操作。读游标未释放唯一连接，写请求排队等待，直到 `:60` 的 30 秒 ctx 超时。
  - 每条到期工单消耗 30 秒并失败。
- **测试盲区**：`internal/service/scheduler_test.go:50` 的注释明确写着 `Full execution would require a more complex setup`，实际断言仅覆盖"空库调用不 panic"。三个缺陷全部在测试可达范围之外。

**修复证据（2026-07-31）**：

- 调度器改为只查询到期工单 ID（`dueTicketIDs`），游标完整读尽并关闭后再执行，同时消除缺陷 A（不再重复维护列清单）与缺陷 C。
- 删除调度器自带的 CAS，`SCHEDULED → EXECUTING` 的迁移交由 `TicketService.executeTicket` 单点持有；`ErrTicketNotExecutable` 视为"其他执行者已抢到"，不再记为失败。
- `scheduler_test.go` 重写：在 `MaxOpenConns=1` 的真实 SQLite 平台库上，用注入的目标连接验证真实 SQL 执行。新增 6 个用例——单条到期工单 `SCHEDULED → DONE` 且目标表被真实修改、3 条到期工单在 5 秒 ctx 内全部完成（缺陷 C 回归会因超时失败）、重复 `RunOnce` 只执行一次、未到期工单不动、非 `SCHEDULED` 工单不动。
- `go test ./internal/service/ -race` 与 `golangci-lint run ./internal/service/...` 通过。

**仍未关闭的部分**：

- 无租约与崩溃恢复。进程在 `EXECUTING` 期间崩溃，工单仍会永久卡住——只是不再必然发生。
- 缺陷 C 的同类模式尚未在全仓库排查，见 REV-P1-015。

### 4.3 REV-P1-006 风险等级受客户端影响（已修复）

- **证据**：`internal/api/handler/ticket.go:30` 的请求体包含 `risk_level` 与 `ai_review_result`，于 `:65` 原样传入 Service；`internal/service/ticket.go:202` 仅在 `riskLevel == ""` 时才由 `RiskEvaluator` 计算。
- **影响**：风险等级经 `approval_engine.go:195` 的 `policyMatches` 参与策略匹配（含自动审批）。提交者自报 `low` 可命中自动通过策略，绕过人工审批。`ai_review_result` 会在 `TicketDetailDrawer.tsx:215` 呈现给审批人，伪造文本等于向审批人展示假的平台 AI 结论。

**修复证据（2026-07-31）**：

- `risk_level` 与 `ai_review_result` 从创建请求和 `CreateTicket` 签名中移除。风险等级由 `RiskEvaluator` 无条件计算，不再是入参——签名层面杜绝了"忘了覆盖"。
- 前端 `CreateTicketRequest` 同步删除这两个可选字段（首方前端本就未发送，无功能损失）。
- 新增 API 级负向测试 `TestTicketHandler_CreateTicket_IgnoresClientRiskLevel`：提交 `DROP TABLE users` 并声明 `risk_level: "low"` + 伪造 AI 结论，断言落库风险等级非 `low` 且 `ai_review_result` 为空。
- 服务层新增 `TestCreateTicket_RiskIsServerDerived`、`TestCreateTicket_DerivesSQLTypeAndTables`。

**契约变更**：`POST /api/tickets` 不再接受 `risk_level` 和 `ai_review_result`。若后续需要在工单上附加 AI 结论，必须由服务端调用 AI 后写入，不能由提交者提供。

### 4.4 REV-P1-007 审批与重提缺少原子性（已修复）

- **审批**：`internal/service/ticket.go:404-429` 是"先 `GetTicket` 判状态 → 再 `UpdateOneID`"，无事务、无 CAS。并发两次审批都会通过状态检查并各写一次。同一状态机内 `executeTicket` 已使用 CAS，`ApproveTicket`/`RejectTicket` 未使用，写法不一致本身即是隐患。
- **多阶段审批**：`approval_engine.go` 的 `ProcessApproval` 同样是先读 `current_stage` 再更新，且审批记录先于状态迁移写入——并发审批会产生两条记录并把审批链推进两级。
- **重提**：`internal/service/ticket_resubmit.go:14`
  - 写 revision 快照与更新工单为两次独立写入，无事务，中途失败会留下孤立快照。
  - 将 `risk_level` 和 `ai_review_result` 清空，但**不重新执行 SQL 分析、不重新评估风险、不重新匹配审批策略**。
  - `sql_type` 与 `affected_tables` 未更新，仍是被驳回版本的分析结果。审批人在新版本上看到的是旧 SQL 的影响面。

**修复证据（2026-07-31）**：

- 新增 `casTicketStatus`：基于 ent 的谓词式 `Update().Where(...)`，返回受影响行数，比较与写入是同一条语句。`ApproveTicket`/`RejectTicket` 改用它，未命中即返回 `ErrInvalidStatusTransition`。
  - 顺带修正一个既有认知：代码中多处 `RAW_SQL: ent 不支持条件 WHERE` 的注释并不准确——ent 的谓词式 `Update()` 可以表达 CAS，只有 `UpdateOneID` 不行。新代码不再为此引入裸 SQL。
- `ProcessApproval` 改为：先以 `current_stage` 为条件做 CAS 迁移，成功后再写审批记录；未命中返回新的 `ErrApprovalStageConflict`。
- `ResubmitTicket` 改为单个 ent 事务：快照与工单更新同进同出，工单更新本身是 `REJECTED → SUBMITTED` 的 CAS。SQL 分析、风险评估、策略匹配抽为 `applyApprovalPolicy`，与创建路径共用；分析在事务外完成（单连接池不允许事务期间再查询），策略应用在提交后执行。
- 补齐一个此前遗漏的缺口：`ApplyPolicy` 自动审批与 `ProcessApproval` 末阶段审批此前都不写 `sql_hash`，意味着 REV-P1-017 修好的完整性校验对这两条路径仍然失效。现在**所有进入 APPROVED 的路径都固定 SQL 哈希**。
- 新增 `internal/service/ticket_governance_test.go`（6 个用例）：4 个并发审批只有 1 个成功、审批后再驳回被拒、4 个并发重提只产生 1 条 revision、重提刷新风险/类型/影响表、创建时风险与影响面由服务端派生。
- **回归有效性已验证**：撤销审批 CAS 后重跑，4 个并发审批**全部成功**；撤销重提 CAS 后重跑，4 次重提全部成功且产生 4 条 revision 快照。

**仍未关闭的部分**：`ProcessApproval` 的 CAS 与审批记录写入不在同一事务内。CAS 先行保证了并发安全，但若记录写入失败，会留下"状态已迁移、无审批记录"的审计空档。彻底关闭需要把审批记录与状态迁移纳入同一事务。

### 4.5 REV-P0-005 生产示例凭据（部分修复）

- **已改进**：`docker-compose.yml:112` 的 `MYSQL_ROOT_PASSWORD` 改为必填（`${VAR:?}`），不再提供固定示例密码。
- **仍未关闭**：
  - `:106` 默认对外发布 `3306`。
  - 示例 MySQL 仍以 root 运行，未提供最小权限账号。
  - `:80` `sqlflow` 服务 `depends_on: mysql (service_healthy)`，示例目标库仍是平台启动依赖。
  - 未使用 Compose `profiles` 分离开发与生产模板。

### 4.6 REV-P0-006 备份不可证明可恢复（仍然存在）

- **证据**：`internal/service/backup.go:152` 的注释自称使用 `VACUUM INTO`，实际实现是 `:162` 的 `PRAGMA wal_checkpoint(TRUNCATE)`（失败仅 WARN）后直接 `io.Copy` 复制主库文件。
- **缺口**：
  - 仅比对字节数（`:189`），无 `PRAGMA integrity_check` 等完整性校验。
  - 只复制主库文件，不处理 `-wal`/`-shm`；checkpoint 失败时备份可能缺少未落盘事务。
  - 无恢复命令、无加密、无异地副本、无演练记录。
- **关闭标准**：维持原标准不变。注释与实现不一致本身也需修正。

### 4.7 REV-P1-002 API Token Scope（部分修复）

- **已改进**：`internal/api/middleware/auth.go:176` 的 `RequireScope` 已挂载到数据源发现、查询执行/历史、工单读写等路由（`internal/api/router.go:97-141`）。
- **仍未关闭**：以下认证路由无 Scope 校验——
  - `POST /api/query/share`、`GET /api/query/share`、`DELETE /api/query/share/:id`
  - `GET /api/query/performance/slow`、`/stats`
  - `GET /api/dashboard/stats`、`/overview`
  - 工单评论相关路由
- **影响**：仅持有只读 Scope 的 API Token 仍可创建对外公开的分享链接。
- **关闭标准**：为每个 Scope 建立允许—拒绝对照测试；新增认证路由默认要求显式声明 Scope。

### 4.8 REV-P1-008 异步导出目录错误（仍然存在）

- **证据**：`internal/app/container.go:111` 将 `cfg.DB.Path`（SQLite **文件**路径）作为 dataDir 传入；`internal/service/export_async.go:56` 执行 `filepath.Join(dataDir, "exports")`，得到 `./data/sqlflow.db/exports`。
- **附加问题**：`:57` 的 `os.MkdirAll` 错误被 `_ =` 丢弃，故障延迟到用户点击导出时才暴露。
- **关闭标准**：新增独立的 `data_dir` 配置项；启动期创建并校验可写，失败即 fail fast。

### 4.9 REV-P1-011 进程生命周期（仍然存在，新增证据）

- **原问题**：`cmd/server/main.go:88-92` 非 TLS 分支没有 signal 监听与 `Shutdown`。
- **新增证据**：**两个分支都以 `log.Fatalf` 收尾**，`os.Exit` 会跳过 `defer container.Close()`（`:57`）与 `defer database.Close()`（`:43`）。即使 TLS 模式优雅关闭成功，`e.StartTLS` 返回 `http.ErrServerClosed` 也会撞进 `:85` 的 `Fatalf`——退出码 1，调度器不停止，SQLite 不做干净关闭。
- **附加缺口**：Echo Server 未配置 `ReadTimeout`/`WriteTimeout`/`IdleTimeout`（仅 `:105` 的重定向 Server 配置了）。
- **关闭标准**：TLS 与非 TLS 共用同一套启动/关闭路径；`http.ErrServerClosed` 不视为错误；`SIGTERM` 后退出码 0 且资源全部释放。

## 5. 新增问题

### REV-P1-013 查询超时在 Driver 路径未生效

- **证据**：`internal/service/query.go:211` 构造了带 30 秒超时的 `drvCtx`，但只用于 `:213` 的 `poolMgr.Get`；`:230` 实际执行查询时传入的是未加超时的原始 `ctx`。
- **影响**：架构文档 §5.2 声明的"默认查询超时 30 秒"在主路径上不由平台保证，实际依赖各 Driver 内部硬编码的 30 秒（`internal/driver/mysql/mysql.go:195`、`internal/driver/postgresql/postgresql.go:214`）。Elasticsearch 与 fallback 路径行为不一致，超时值也无法配置。
- **关闭标准**：超时在 Service 层统一施加并成为配置项；`defaultRowLimit`（`query.go:33`）同步提为配置；Driver 内部不再硬编码。

### REV-P1-014 工单创建缺少数据源级授权

- **证据**：`internal/service/ticket.go:189` 只对 `dbType == "mongodb"` 调用 `checkMongoPermission`，SQL 类数据源在创建工单时不做任何数据源或表级授权校验。
- **影响**：任意认证用户可对任意数据源提交工单。执行前有审批把关，因此不是直接越权，但违反"服务端在每个入口独立裁决"的架构约束，且会污染审批队列。
- **关闭标准**：工单创建复用查询链路的 Casbin 元组做数据源级门禁；负向测试覆盖无权限用户提单被拒。

### REV-P1-015 SQLite 单连接下的嵌套读写未被约束

- **证据**：`internal/db/db.go:49` 全局 `SetMaxOpenConns(1)`。在此前提下，任何"持有 `rows` 游标期间发起写操作"的代码都会阻塞至 ctx 超时。已确认的实例见 4.2 缺陷 B。
- **影响**：这是一类会静默失效的模式——表现为超时而非报错，且只在有数据时触发，空库测试永远发现不了。
- **关闭标准**：排查全仓库同类模式（游标未关闭即发起写）；建立编码约定并在评审清单中固化；考虑用 lint 或封装的仓储方法约束。

### REV-P1-016 关键路径测试为烟雾级

- **证据**：改造前的 `internal/service/scheduler_test.go:50` 仅在空库上调用 `RunOnce` 并断言不返回错误，注释直言未覆盖完整执行。
- **影响**：REV-P1-003 的三个致命缺陷长期未被发现。测试通过率无法反映核心能力可用性。
- **进度（2026-07-31）**：调度执行一条已补齐（见 4.2 修复证据）。并发审批、多语句拒绝、Scope 矩阵、导出目录四条仍缺。
- **关闭标准**：上述五条路径全部具备带真实断言的测试。

### REV-P1-017 工单 SQL 一致性校验从未生效（已修复）

- **证据**：
  - `internal/service/ticket.go:419` 在审批时计算并写入 `sql_hash`。
  - `internal/service/ticket.go:615` 在执行前校验 `t.SQLHash`，用于拦截"审批后被改动的 SQL"。
  - 但 `t` 的唯一来源 `GetTicket`（`:290`）的 SELECT 列表**不含 `sql_hash`**，`scanTicket`（`:127`）也不扫描该字段。`model.Ticket.SQLHash` 因此恒为空字符串，`:615` 的 `if t.SQLHash != ""` 永远不成立。
- **影响**：`sql_hash` 是只写字段，审批与执行之间的 SQL 篡改检测完全不生效。工单在审批通过后被改写 `sql_content`，执行阶段不会有任何拦截。REV-P1-003 修复后定时执行真正开始工作，该缺口的暴露面随之扩大。
- **发现时机**：修复 REV-P1-003 时追踪执行路径发现，非本次静态复核的原始结论。

**修复证据（2026-07-31）**：

- 新增 `ticketColumns` 常量并补入 `sql_hash`，`scanTicket` 同步扫描该字段；`GetTicket` 与 `ListTickets` 改为引用该常量（同时关闭 DEBT-07）。
- `ResubmitTicket` 清空 `sql_hash`：新版本尚未审批，沿用旧哈希会让完整性校验对着过期的审批结论通过。
- `ListTickets` 的行扫描失败不再被静默 `continue` 吞掉，改为记录日志——这正是调度器列数不匹配长期隐蔽的原因。
- 新增 `internal/service/ticket_sql_hash_test.go`：`GetTicket`/`ListTickets` 返回哈希、篡改后立即执行被拒且工单转 `FAILED`、篡改后经调度执行同样被拒、未篡改工单正常执行（防误报）、重提清空哈希。
- **回归有效性已验证**：临时撤销修复后重跑，`ExecuteTicket` 对被篡改工单执行成功，目标表被写入 `n = 666`，即篡改语句真实到达目标库；恢复修复后全部通过。
- `go test ./internal/service/ -race`、`./internal/api/...`、`go vet`、`golangci-lint`（0 issues）通过。

## 6. 重构债务

以下问题不改变外部行为，但会持续放大修复成本。

| ID | 债务 | 证据 |
|---|---|---|
| DEBT-01 | `internal/connpool` 与 `driver.PoolManager` 双轨。`internal/service/query.go:248-269` 中 MySQL/PostgreSQL/MongoDB 的 fallback 分支在 `poolMgr != nil` 时**永不可达**，是死代码；且 `sqlite` 未列入 fallback switch，会落到 `default` 返回"不支持的数据源类型" | `query.go:204`、`:251` |
| DEBT-02 | `ExecuteQuery` 的 `:248` `if result == nil && err == nil` 依赖内层 `:=` 对 `err` 的变量遮蔽才成立。任何一处改成 `=` 都会静默改变控制流 | `query.go:206`、`:248` |
| DEBT-03 | 原始 SQL 与 Ent 双轨，同一 Service 内两种写法交替（如 `ticket.go:601` 用原始 SQL CAS，`:643` 用 Ent 更新） | ADR-0002 |
| DEBT-04 | 超大文件：`service/query.go` 1399 行、`service/datasource.go` 1396 行、`service/ticket.go` 977 行、`service/notify.go` 971 行；`internal/service` 为扁平包，47 个业务文件无子域划分 | — |
| DEBT-05 | 领域错误映射不统一：`handler/ticket.go:104` 已按错误类型映射 403/404，多数 Handler 仍以 `InternalError` 兜底（对应 REV-P1-010 未关闭部分） | — |
| DEBT-06 | 超时与行数上限在三处各自硬编码，无单一来源 | `query.go:33`、`mysql/mysql.go:195`、`postgresql/postgresql.go:214` |
| DEBT-07 | 工单 SELECT 列清单在多处手工重复，与 `scanTicket` 的字段顺序靠约定维持。已导致 REV-P1-003 缺陷 A 和 REV-P1-017。**已修复**：收敛为 `ticketColumns` 常量，与 `scanTicket` 相邻定义 | `ticket.go` |

## 7. 建议顺序

按"阻断核心功能"优先于"降低攻击面"排列，映射到 [ROADMAP.md](../ROADMAP.md)：

1. ~~REV-P1-003 主体~~（**已完成**）→ 余下 REV-P1-015（同类嵌套读写排查）与执行租约/崩溃恢复。
2. ~~REV-P1-017 + DEBT-07~~（**已完成**）。
3. ~~REV-P1-006 + REV-P1-007~~（**已完成**）→ 余下审批记录与状态迁移的事务化。
4. REV-P0-004（语句分词器）——同时修复门禁不完整与合法查询误杀，影响查询和工单两条链路。
5. REV-P1-008 + REV-P1-011 + REV-P1-013（数据目录、生命周期、超时）——运行时基础，改动小。
6. REV-P1-002 剩余部分 + REV-P1-014 + REV-P1-012（授权边界收口）。
7. REV-P0-005 + REV-P0-006（部署模板与备份恢复）。
8. DEBT-01 ~ DEBT-06（在对应功能切片内顺带完成，不单独立项重写）。

## 8. 后续清理（2026-07-31）

本次复核之后，仓库执行了一轮范围收缩。以下问题因承载功能被删除而不再适用，不计为"修复"：

| 原问题 | 处理 |
|---|---|
| 覆盖度审计（`FR-OPS-007`、`US-OPS-006`） | 前后端全栈删除。后端路由本就以 `nil` PG 连接注册（设计性禁用），前端页面、hooks、API 与类型一并移除，共约 3000 行。 |
| REV-P1-012 菜单与 API 边界不一致 | 覆盖度导航随功能删除，该项范围缩小；审计、报表、设置三处导航的角色门禁仍未收口。 |
| REV-P1-016 关键路径测试为烟雾级 | 4 个长期 `t.Skip` 的解析器用例与 1 个 handler 占位用例已删除，不再伪装成覆盖。前端一个约 50% 失败率的 flaky 用例（approval-stepper）同样删除。 |

同时删除的还有 Playwright E2E 套件（`e2e/`、`.github/workflows/e2e.yml`、`docker-compose.test.yml`、`scripts/ci-e2e.sh`）。

**由此产生的新缺口**：浏览器级端到端验证目前完全没有。REV-P0-002 的关闭标准原本依赖 Playwright 场景，现需重新约定验证方式。这是一项主动接受的取舍，不是遗漏。

另有一处保留决定：`internal/service/query.go` 的 connpool fallback 分支在生产中不可达（`app.Container` 恒构造 `PoolManager`），但它是当前所有服务层查询测试的唯一执行路径——全部测试都以 `poolMgr = nil` 构造 `QueryService`。删除它需要先把约 10 处测试改用 `driver.PoolManager.InjectForTest` 并实现完整的 Driver mock，属于净增工作量，故本轮只消除了其中的重言条件并标注了可达性。

## 9. 评审限制

- 除 REV-P1-003 外，本次为静态代码复核，未运行 Playwright E2E、未连接真实目标数据库。REV-P1-003 的三个缺陷已用先行失败的用例复现并在修复后回归通过（`go test ./internal/service/ -race`、`./internal/api/...`、`golangci-lint`）；其余问题的结论仍来自代码路径推导，建议修复时同样先复现再改。
- REV-P1-003 与 REV-P1-017 的回归测试用另一个 SQLite 库经 `InjectMySQLForTest` 充当目标数据源，验证了状态机、完整性校验与执行链路，但未验证真实 MySQL/PostgreSQL 的驱动语义（事务、DDL 自动提交等）。
- REV-P1-017 的修复覆盖 `sql_hash` 的读取与重提清理，未覆盖"谁能改写已审批工单的 `sql_content`"这一入口层面的问题——当前依赖没有此类 API，而非服务端明确拒绝。
- REV-P0-002（数据源发现 E2E）与 REV-P1-009（多数据源前端路径）本次未复核，状态沿用 2026-07-26 记录。
- 未评估 AI 评审、通知集成、审计报表和覆盖度页面的实现质量。
- 未做渗透测试、负载测试或恢复演练。

这些限制不改变上述问题的优先级，但意味着关闭时仍需补充动态证据。
