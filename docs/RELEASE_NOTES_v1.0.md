# SQLFlow v1.0 Release Notes

**发布日期**：2026-05-23  
**版本**：1.0.0  
**状态**：正式发布 🎉

---

## 概述

SQLFlow 是一个面向开发团队和 DBA 的 **SQL 审批管理平台**。v1.0 是首个正式发布版本，涵盖了从 SQL 在线查询、AI 前置评审、变更工单审批到数据脱敏、权限管理和操作审计的完整功能闭环。

**核心价值**：将低风险查询自助化、高风险操作走审批流程，所有操作全程留痕可追溯，帮助运维从「执行者」转变为「审批者和规则制定者」。

---

## 功能清单

### 🔍 在线 SQL 查询
- 支持 **MySQL** 和 **MongoDB** 双数据源
- SQL 编辑器（CodeMirror 6）+ 语法校验 + 实时执行
- 结果表格展示，支持分页、导出（CSV/JSON，≤10000 行）
- 慢查询检测，超时自动中断（默认 30s）
- MongoDB 支持 aggregation pipeline（白名单 stage 校验）

### 🤖 AI 前置评审
- 提交 SQL 后自动进行风险分级 + 优化建议 + 变更影响分析
- 支持多种 AI Provider：OpenAI / 智谱 GLM / Azure OpenAI / 自定义（OpenAI 兼容 API）
- SSE 流式返回，3-10 秒出结果
- 超时自动降级为静态规则评审

### 🔒 数据脱敏
- 字段级脱敏规则，查询结果**默认脱敏**
- 8 种内置规则：手机号、身份证、姓名、邮箱、银行卡、地址、全掩码、自定义正则
- 按表级别标记敏感数据
- Casbin 权限控制脱敏豁免，豁免操作记录审计日志

### 📋 变更工单审批流
- DDL/DML/MongoDB update 操作必须走工单
- 工单状态机：`SUBMITTED → AI_REVIEWED → PENDING_APPROVAL → APPROVED → EXECUTING → DONE`
- AI 评审结果决定简化/标准工单流程
- 审批通过后需手动执行，DBA 可选择合适时机

### 👥 RBAC 权限管理
- 三种内置角色：admin / dba / developer
- 基于 Casbin RBAC with domains，支持数据源级别隔离
- 权限粒度：数据源 → 表 → 操作类型（select/update/delete/ddl/export）
- 敏感表默认拒绝访问，需管理员显式授权

### 📊 操作审计
- 全量记录：查询、变更、导出、权限策略变更
- 记录字段：操作人、时间、SQL、影响行数、执行耗时、评审结果
- 支持按用户/时间/数据源/操作类型筛选
- 审计日志 API 层面不可删除

### 🔔 通知集成
- 钉钉 Webhook 机器人通知
- 工单提交 / 审批结果实时推送
- 中高风险操作实时告警到 DBA 群

### 🖥 前端页面
- 查询页（SQL 编辑器 + 结果表格 + AI 评审 + 导出）
- 工单页（提交/查看/审批 + 评审详情 + 评论）
- 审计日志页（多维度筛选）
- 数据源管理页（注册/连接测试）
- 设置页（AI 配置 + 脱敏规则）
- Dashboard 概览页（统计卡片 + 待办事项）
- 用户管理页（Admin CRUD + 重置密码）

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.25 + Echo v4 |
| 前端 | React 19 + Vite 8 + TypeScript 6 + TailwindCSS 4 + Shadcn/ui |
| SQL 编辑器 | CodeMirror 6 |
| 状态管理 | Zustand + TanStack Query |
| 平台数据库 | SQLite（WAL 模式，零运维） |
| 目标数据库 | MySQL + MongoDB |
| SQL 解析 | pingcap/parser（AST） |
| 权限 | Casbin RBAC with domains |
| 认证 | JWT（HS256）+ Refresh Token |
| AI 评审 | OpenAI / 智谱 GLM / Azure / 自定义（SSE 流式） |
| 部署 | Docker（单容器，前端 embed 进 Go 二进制） |

---

## 质量指标

| 指标 | 值 |
|------|------|
| Go 测试覆盖率 | 85.2% |
| 安全审计 | 通过（详见 SECURITY_AUDIT.md） |
| Go 编译 | ✅ 通过 |
| React 构建 | ✅ 通过（产物 310KB gzipped） |

---

## 快速开始

### Docker 部署（推荐）

```bash
# 1. 配置环境
cp config/config.example.yaml config/config.yaml
# 编辑 config.yaml，填入 MySQL/MongoDB 连接信息、AI Provider 配置等

# 2. 启动
docker compose up -d

# 3. 访问
# http://localhost:8080
# 默认管理员：admin / admin123（请立即修改密码）
```

### 从源码构建

```bash
# 后端
export PATH="/usr/local/go/bin:$PATH"
go build -o sqlflow ./cmd/server

# 前端（开发模式）
cd web && npm install && npm run dev

# 前端（生产构建，embed 进 Go 二进制）
cd web && npm run build
cd .. && go build -o sqlflow ./cmd/server
```

---

## 已知限制

1. 前端 JS bundle 超过 500KB（建议后续做 code-splitting 优化）
2. MongoDB 仅支持基本 aggregation pipeline（白名单 stage 校验）
3. 导出功能上限 10000 行
4. 查询结果默认上限 1000 行
5. 钉钉通知仅支持 Webhook 方式，暂不支持钉钉 OAuth 登录

---

## 升级路径

此为 v1.0 首个正式版本，无升级路径。后续版本将遵循语义化版本规范。

---

## 贡献者

- **棱镜 Prism 团队** — 架构设计、全栈开发、测试、文档
- **Marcus** — 技术负责人，核心架构与代码审查
- **钱进** — 项目经理，流程管理与发布协调

---

## 许可证

内部项目，未经授权不得对外分发。
