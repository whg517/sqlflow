# Changelog

本文件记录 SQLFlow 平台所有值得注意的变更。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [1.0.0] - 2026-05-23

SQLFlow v1.0 正式发布 — 面向开发团队和 DBA 的 SQL 审批管理平台。

### 新增功能

#### 查询模块
- **在线 SQL 查询**：支持 MySQL 和 MongoDB 双数据源，CodeMirror 6 编辑器，实时执行，结果分页展示
- **慢查询检测**：30s 超时自动中断，防止长查询拖垮数据库
- **查询结果导出**：支持 CSV/JSON 流式导出，单次上限 10000 行
- **查询历史**：按用户隔离，支持分页浏览和删除
- **数据脱敏**：字段级脱敏规则，查询结果默认脱敏
  - 8 种内置规则：手机号、身份证、姓名、邮箱、银行卡、地址、全掩码、自定义正则
  - Casbin 权限控制脱敏豁免，豁免操作记录审计日志
- **敏感表管理**：按表级别标记敏感数据（low/medium/high），默认拒绝访问
- **SQL 解析**：基于 pingcap/parser AST 解析，实现 SQL 类型识别和风险分级

#### AI 评审模块
- **AI 前置评审**：提交 SQL 后自动进行风险分级 + 优化建议 + 变更影响分析
- **多 Provider 支持**：OpenAI / 智谱 GLM / Azure OpenAI / 自定义（OpenAI 兼容 API）
- **SSE 流式返回**：3-10 秒出结果，前端实时展示 AI 评审过程
- **超时降级**：AI 超时自动降级为静态规则评审（基于 AST 的确定性规则）
- **评审结果缓存**：30 秒有效期内复用评审结果，避免重复调用

#### 工单审批模块
- **变更工单流**：DDL/DML/MongoDB update 操作必须走工单
- **完整状态机**：`SUBMITTED → AI_REVIEWED → PENDING_APPROVAL → APPROVED → EXECUTING → DONE`
- **支持 REJECTED / CANCELLED 终态**
- **AI 驱动流程**：AI 评审结果决定简化/标准工单流程
- **手动执行**：审批通过后 DBA 可选择合适时机执行
- **定时执行**：支持设置 scheduled_at，Scheduler 自动执行到期工单
- **工单评论**：支持工单内讨论，支持嵌套回复（ParentID）

#### 认证与权限模块
- **JWT 认证**：HS256 签名 + Refresh Token 轮换机制
- **RBAC 权限管理**：基于 Casbin RBAC with domains
  - 三种内置角色：admin（系统管理）/ dba（审批 + 配置）/ developer（查询 + 提交工单）
  - 数据源级别隔离，权限粒度到表 + 操作类型（select/update/delete/ddl/export/desensitize:bypass）
- **钉钉 OAuth 登录**：支持通过钉钉扫码登录（可选开启）
- **用户管理**：Admin CRUD + 重置密码，密码策略（8-128 字符，必须含字母+数字，bcrypt 哈希存储）

#### 审计模块
- **全量审计日志**：记录查询、变更、导出、权限策略变更等所有操作
- **多维度筛选**：按用户/时间/数据源/操作类型/关键词搜索
- **FTS5 全文搜索**：审计日志支持 SQLite FTS5 全文搜索 + 高亮
- **不可删除**：API 层面不提供 DELETE 端点，保障审计完整性

#### 通知模块
- **钉钉 Webhook 集成**：工单状态变更 + 高风险操作实时告警
- **事件覆盖**：工单提交、审批通过、审批驳回、工单执行完成

#### 系统模块
- **Dashboard 概览**：统计卡片（查询数、工单数、活跃用户等）+ 待办事项
- **数据源管理**：MySQL/MongoDB 注册、编辑、连接测试、库表列表查询
- **数据库自动备份**：定时备份 SQLite 数据库，支持 gzip 压缩，自动轮转（默认保留 10 个）
- **连接池管理**：MySQL/MongoDB 连接池复用，按 datasourceID 缓存连接
- **健康检查**：`/api/health` 端点 + Docker HEALTHCHECK 指令

#### 前端页面
- 查询页（SQL 编辑器 + 结果表格 + AI 评审面板 + 导出）
- 工单页（提交/查看/审批 + 评审详情 + 评论）
- 审计日志页（多维度筛选 + 全文搜索）
- 数据源管理页（注册/编辑/连接测试）
- 设置页（AI 配置 + 脱敏规则 + 钉钉通知配置）
- Dashboard 概览页（统计卡片 + 待办事项）
- 用户管理页（Admin CRUD + 重置密码）
- 7 个 E2E 测试套件覆盖核心流程

#### DevOps & CI/CD
- **多阶段 Docker 构建**：Node 构建前端 → Go 编译（前端 embed 进二进制）→ Alpine 运行时
- **非 root 运行**：Docker 容器以 sqlflow 用户运行
- **GitHub Actions CI**：Lint → Test → Build → E2E → Docker Verify → Security Scan（8 个 Job）
- **GitHub Actions CD**：自动部署到测试环境
- **GitHub Actions Release**：多平台二进制（linux/darwin/windows × amd64/arm64）+ Docker 镜像 + Checksum
- **Dependabot**：自动检测 Go 和 npm 依赖更新
- **Stale Bot**：自动标记过期 Issue/PR

### 安全加固

- **密钥管理**：JWT Secret、AES 加密密钥、管理员密码、AI API Key 全部支持环境变量注入，配置文件中不存储明文
- **启动校验**：JWT Secret ≥ 16 字节、AES Key 长度校验（16/24/32）、管理员密码策略
- **CORS 安全**：修复 wildcard + credentials 冲突，支持生产环境域名白名单
- **MongoDB URI 凭据编码**：用户名/密码 URL 编码防止注入
- **SQL 注入防护**：AST 解析限制仅 SELECT 可直接执行 + 内部查询全部参数化
- **生产环境错误脱敏**：不暴露 err.Error() 给前端
- **审计日志不可删除**：API 层无 DELETE 端点
- **API Key 脱敏返回**：GetConfig() 返回时仅显示前4后4位
- **安全扫描**：CI 集成 govulncheck + Trivy 扫描
- **完整安全审计**：3 Critical + 2 High + 4 Medium + 3 Low + 3 Info，全部 Critical/High 已修复

### 性能优化

- **连接池复用**：MySQL/MongoDB 连接按 datasourceID 缓存，避免重复建立连接
- **审计日志阻塞写入**：确保审计记录不丢失
- **分页逻辑抽取**：通用 Pagination helper，减少代码重复
- **全链路 Context 传递**：handler → service → db，支持超时控制和链路追踪
- **查询历史限制**：每用户最大 200 条，自动清理历史记录
- **数据库 WAL 模式**：SQLite WAL 模式，支持并发读写
- **自动备份压缩**：gzip 压缩备份文件，减少磁盘占用

**性能基准测试（20 并发）**：

| 接口 | 吞吐量 | P50 延迟 | P95 延迟 |
|------|--------|----------|----------|
| Login | 3,494 req/s | 0.98ms | 5.49ms |
| Auth Me | 7,009 req/s | 1.71ms | 5.79ms |
| Dashboard Stats | 3,046 req/s | 5.48ms | 11.01ms |
| Query History | 2,800 req/s | 4.12ms | 27.38ms |
| Ticket List | 2,788 req/s | 5.75ms | 15.85ms |
| Ticket Create | 3,079 req/s | 1.26ms | 48.02ms |

**数据库性能**：

| 操作 | P50 延迟 | P99 延迟 |
|------|----------|----------|
| Insert | 43.9µs | 187.8µs |
| Select (COUNT) | 3.4µs | 17.8µs |
| Select (indexed) | 5.3µs | 20.6µs |
| Select (paginated) | 48.7µs | 283.5µs |
| Concurrent Write | 350.3µs | 1.63ms（528K rows/s） |

所有指标均通过基准线检查 ✅

### 已知问题

1. 前端 JS bundle 超过 500KB（建议后续 code-splitting 优化）
2. MongoDB 仅支持基本 aggregation pipeline（白名单 stage 校验）
3. 导出功能上限 10000 行
4. 查询结果默认上限 1000 行
5. 仅支持 HTTP（需反向代理层终止 TLS），建议 v1.1 添加应用层 TLS 支持
6. 查询执行端点缺少速率限制，建议 v1.1 添加令牌桶限流中间件
7. 登录接口缺少暴力破解防护，建议 v1.1 添加失败计数 + IP 限流 + CAPTCHA
8. 审计日志数据库层缺少 DELETE 保护（API 层已安全）

---

## [0.5.0] - 2026-05-08

### Added
- 前端功能补全提案（004-frontend-completion）
- Admin 用户管理前端页面（CRUD + 重置密码）
- Dashboard 概览页（统计卡片 + 待办事项）
- 查询页 AI 评审高风险→工单创建联动入口
- 侧边栏导航更新

## [0.4.0] - 2026-05-07

### Fixed
- Permission 中间件参数来源修复（query → body）
- TRUNCATE 拦截误拦 DDL 修复
- rows.Err() 全局补全（audit/mask/permission/query_history）
- AES 密钥长度启动校验
- LIKE 通配符转义

### Changed
- 审计日志改为阻塞写入
- 错误信息脱敏（生产环境不暴露 err.Error）
- MySQL/MongoDB 连接池复用（datasourceID → 连接缓存）
- 分页逻辑抽取为通用 helper
- 审计写入统一走 AuditService
- handler → service → db 全链路 Context 传递

### Removed
- 未使用的 Ent schema 目录
- 空 DesensitizeService 实现
- 前端重复 Settings/settings 目录

### Added
- SQL Parser / Mask 单元测试补充
- config.example.yaml 示例配置
- Dockerfile HEALTHCHECK 指令

## [0.3.0] - 2026-05-07

### Added
- 测试覆盖率从 78.2% 提升至 85.2%（450+ tests）
- Service 层核心函数测试（auth/datasource/query/permission/audit）
- Connpool mock 测试（MySQL Ping/GetTables + MongoDB Ping）
- Crypto + SQL Parser 边界用例测试

## [0.2.0] - 2026-05-03

### Added
- 数据导出 API（CSV/JSON 流式导出，10000 行上限，审计记录）
- 查询执行 API + 数据脱敏增强
- SQL 解析器升级为 pingcap/parser AST 解析
- CLAUDE.md + AGENTS.md（Agent 协作配置）

## [0.1.0] - 2026-05-02

### Added
- 项目脚手架搭建（Go + React + Docker Compose）
- 认证与用户管理（JWT、角色权限、用户 CRUD）
- 数据源管理（MySQL/MongoDB 注册、连接测试）
- Casbin RBAC 权限系统
- AI SQL 评审（OpenAI/智谱 GLM/Azure/自定义 Provider）
- 查询执行（SELECT 结果表格 ≤1000 行）
- 数据脱敏（手机号/身份证自动掩码）
- 工单审批流程（提交 → AI 评审 → DBA 审批 → 手动执行）
- 审计日志（SELECT/DDL/DML/EXPORT 四类操作）
- 钉钉 Webhook 通知
- 前端页面（查询、工单、审计、数据源、设置）
- 项目文档体系（PRD + 架构 + UI 设计 + OpenSpec 提案流程）
