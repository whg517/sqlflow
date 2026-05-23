# Changelog

本文件记录 SQLFlow 平台所有值得注意的变更。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [1.0.0] - 2026-05-23

SQLFlow v1.0 正式发布 — 面向开发团队和 DBA 的 SQL 审批管理平台。

### 核心功能

- **在线 SQL 查询**：支持 MySQL 和 MongoDB 双数据源，CodeMirror 6 编辑器，实时执行，结果分页展示，慢查询检测（30s 超时中断）
- **AI 前置评审**：提交 SQL 后自动风险分级 + 优化建议 + 变更影响分析，支持 OpenAI / 智谱 GLM / Azure OpenAI / 自定义 Provider，SSE 流式返回（3-10s 出结果），超时自动降级
- **数据脱敏**：字段级脱敏规则，查询结果默认脱敏，8 种内置规则（手机号、身份证、姓名、邮箱、银行卡、地址、全掩码、自定义正则），Casbin 权限控制脱敏豁免
- **变更工单审批流**：DDL/DML/MongoDB update 走工单流程（SUBMITTED → AI_REVIEWED → PENDING_APPROVAL → APPROVED → EXECUTING → DONE），AI 评审结果决定简化/标准流程
- **RBAC 权限管理**：三种内置角色（admin / dba / developer），基于 Casbin RBAC with domains，数据源级别隔离，权限粒度到表+操作类型
- **操作审计**：全量记录查询/变更/导出/权限策略变更，支持多维度筛选，审计日志不可删除
- **钉钉通知集成**：工单状态变更 + 高风险操作实时告警

### 前端页面

- 查询页（SQL 编辑器 + 结果表格 + AI 评审 + 导出）
- 工单页（提交/查看/审批 + 评审详情）
- 审计日志页（多维度筛选）
- 数据源管理页（注册/连接测试）
- 设置页（AI 配置 + 脱敏规则）
- Dashboard 概览页（统计卡片 + 待办事项）
- 用户管理页（Admin CRUD + 重置密码）

### 技术架构

- 后端：Go 1.25 + Echo v4 + SQLite（WAL 模式）
- 前端：React 19 + Vite 8 + TypeScript 6 + TailwindCSS 4 + Shadcn/ui
- SQL 编辑器：CodeMirror 6
- 权限：Casbin RBAC with domains
- 认证：JWT（HS256）+ Refresh Token
- SQL 解析：pingcap/parser AST
- 部署：Docker 单容器（前端 embed 进 Go 二进制）

### 安全

- 测试覆盖率 85.2%
- 完整安全审计（参见 SECURITY_AUDIT.md）
- 生产环境错误信息脱敏
- AES 密钥长度启动校验
- CORS 中间件配置
- SQL 注入防护（AST 解析 + 参数化查询）

### Changed
- PRD 定稿 v1.0
- .gitignore 补充 server 二进制和 coverage.out 排除规则

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
