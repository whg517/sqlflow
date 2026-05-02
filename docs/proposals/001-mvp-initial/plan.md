# Plan: SQLFlow MVP 初始开发

> 状态: 待启动
> 更新日期: 2026-05-02
> 关联 Spec: spec/PRD.md v4, spec/ARCHITECTURE.md v4, spec/UI-DESIGN.md v4

## 开发原则

- **后端先行**：每个 Phase 先完成 API，再做前端页面
- **纵向切分**：按功能模块切分，每个 Phase 产出可独立验证的功能
- **Claude Code 执行**：化刚角色是 PM + QA，开发交给 Claude Code
- **验收标准**：每个 Task 完成后必须能跑通对应的 API + 前端交互

---

## Phase 0：项目脚手架（1 工作日）

> 目标：项目能编译运行，空白首页可访问

### Task 0.1 后端脚手架

> Spec: `spec/ARCHITECTURE.md` 技术选型 + 项目结构

- [ ] 初始化 Go module (`go mod init`)
- [ ] 集成 Echo 框架、中间件 (CORS、Logger、Recovery)
- [ ] 集成 Ent ORM + SQLite WAL 配置
- [ ] 统一响应格式 (`response.go`)
- [ ] 配置管理 (`config.yaml` + Viper)
- [ ] 启动入口 (`cmd/server/main.go`)
- [ ] 健康检查端点 `GET /api/health`
- [ ] Dockerfile + docker-compose.yaml

**验收**：`docker-compose up` → `curl /api/health` 返回 `200 {"status":"ok"}`

### Task 0.2 前端脚手架

> Spec: `spec/UI-DESIGN.md` 第 2 节整体布局 + 第 10 节 Design Token

- [ ] Vite + React 19 + TypeScript 初始化
- [ ] TailwindCSS + Shadcn/ui 配置
- [ ] Design Token (`globals.css` + `tailwind.config.ts`)
- [ ] React Router v7 路由骨架（所有页面占位）
- [ ] API 请求层封装 (fetch + 拦截器 + 401 处理)
- [ ] 全局布局组件（顶栏 + 侧边栏 + 工作区）
- [ ] 代理配置 (Vite proxy → 后端 8080)

**验收**：`npm run dev` → 显示三栏布局（侧边栏 5 个导航项 + 顶栏 + 工作区），点击导航切换占位页，Design Token 色板符合 spec/UI-DESIGN.md 第 10 节

---

## Phase 1：认证与用户管理（1.5 工作日）

> 目标：用户能登录、管理员能增删改用户

### Task 1.1 认证系统

> Spec: `spec/ARCHITECTURE.md` 第 2.1 节 JWT + `spec/PRD.md` 第 4 节认证

- [ ] Ent Schema: User（id, username, password_hash, role, created_at）
- [ ] 初始管理员创建逻辑（启动参数/环境变量）
- [ ] 密码 bcrypt 哈希
- [ ] JWT 签发/验证（HS256, 24h）
- [ ] `POST /api/auth/login` 登录
- [ ] `GET /api/auth/me` 获取当前用户
- [ ] `PUT /api/auth/password` 修改密码
- [ ] `auth.go` 中间件（JWT 验证 + 用户信息注入上下文）
- [ ] Ent Adapter for Casbin（基础结构，策略后续填充）
- [ ] 密码策略校验（8-128 字符，至少字母+数字）

### Task 1.2 用户管理 API

> Spec: `spec/ARCHITECTURE.md` API 端点 + `spec/PRD.md` 第 3 节权限模型

- [ ] `POST /api/users` 创建用户
- [ ] `GET /api/users` 用户列表
- [ ] `GET /api/users/:id` 用户详情
- [ ] `PUT /api/users/:id` 编辑用户（角色）
- [ ] `DELETE /api/users/:id` 删除用户（不能删自己、不能删 admin）
- [ ] `PUT /api/users/:id/reset-password` 重置密码
- [ ] 权限校验：仅 admin 可操作

### Task 1.3 前端 - 登录页

> Spec: `spec/UI-DESIGN.md` 第 12 节组件选型（登录 P0）

- [ ] 登录表单（用户名 + 密码）
- [ ] 表单验证（3-32 字符 / 8-128 字符）
- [ ] 登录失败 inline 错误提示
- [ ] 登录中 loading 状态
- [ ] JWT 存 localStorage + 自动跳转
- [ ] 401 统一拦截 → 跳转登录页 + toast

### Task 1.4 前端 - 权限管理（用户 Tab）

> Spec: `spec/UI-DESIGN.md` 第 6 节权限管理页

- [ ] 用户列表表格（用户名 / 角色 / 创建时间 / 操作）
- [ ] 添加用户弹窗（用户名 + 密码 + 角色）
- [ ] 编辑角色（下拉选择）
- [ ] 重置密码（二次确认弹窗）
- [ ] 删除用户（二次确认，admin 不可删）
- [ ] 角色中文名映射（admin→管理员, dba→DBA, developer→开发人员）

### Task 1.5 前端 - 个人设置

> Spec: `spec/UI-DESIGN.md` 第 8 节个人设置

- [ ] 头像下拉菜单（个人设置 / 修改密码 / 登出）
- [ ] 个人设置弹窗（用户名+角色只读 + 修改密码表单）
- [ ] 表单验证（当前密码必填 / 新密码 8-128+字母数字 / 确认一致）
- [ ] 保存成功 toast
- [ ] 登出清除 localStorage + 跳转登录页

**验收**：admin 登录 → 添加 developer 用户 → developer 登录成功 → developer 修改密码成功 → 登出跳转登录页 → 新密码登录成功

---

## Phase 2：数据源与权限（1.5 工作日）

> 目标：管理员能注册数据源、配置权限策略，用户有正确的访问控制

### Task 2.1 数据源管理

> Spec: `spec/ARCHITECTURE.md` 第 2 节目标数据库连接 + `spec/PRD.md` 第 1 节数据源管理

- [ ] Ent Schema: DataSource（id, name, type, host, port, username, password_encrypted, max_open, max_idle, max_lifetime, max_idle_time, status, created_at）
- [ ] MySQL 连接池管理 (`connpool/mysql.go`)
- [ ] MongoDB 连接池管理 (`connpool/mongodb.go`)
- [ ] 密码 AES 加密/解密 (`pkg/crypto/`)
- [ ] 健康检查（每 30s ping，3 次失败标记不可用）
- [ ] `POST /api/datasources` 添加数据源
- [ ] `GET /api/datasources` 数据源列表
- [ ] `PUT /api/datasources/:id` 编辑数据源
- [ ] `DELETE /api/datasources/:id` 禁用数据源
- [ ] `GET /api/datasources/:id/tables` 获取库表列表
- [ ] `POST /api/datasources/:id/test` 测试连接

### Task 2.2 Casbin 权限系统

> Spec: `spec/ARCHITECTURE.md` 第 2.2 节 Casbin RBAC + `spec/PRD.md` 第 3 节权限模型

- [ ] Casbin 模型定义 (`model.conf` — RBAC with domains)
- [ ] 初始策略种子数据 (`policy.csv`)
- [ ] `permission.go` 中间件（按路由配置所需 action）
- [ ] `GET /api/roles` 角色列表
- [ ] `GET /api/roles/:id` 角色详情（含权限策略）
- [ ] `POST /api/policies` 添加权限策略
- [ ] `GET /api/policies` 权限策略列表
- [ ] `DELETE /api/policies/:id` 删除权限策略
- [ ] `POST /api/policies/sync` 同步策略到内存

### Task 2.3 前端 - 设置页（数据源）

> Spec: `spec/UI-DESIGN.md` 第 9 节设置页

- [ ] 设置页二级导航（数据源 / 脱敏规则 / AI 配置）
- [ ] 数据源列表（名称 / 类型 / 地址 / 状态 / 连接测试按钮）
- [ ] 添加/编辑数据源弹窗（表单 + 验证规则）
- [ ] 连接测试结果反馈
- [ ] 禁用数据源二次确认

### Task 2.4 前端 - 权限管理（角色 Tab + 策略 Tab）

> Spec: `spec/UI-DESIGN.md` 第 6 节权限管理页（策略卡片式布局）

- [ ] 角色管理表格（角色 / 用户数 / 描述 / 查看权限）
- [ ] 查看权限展开：显示该角色下所有策略列表
- [ ] 权限策略卡片式布局
- [ ] 添加策略表单（角色 + 数据源 + 表名 + 操作类型多选）
- [ ] `desensitize:bypass` 特殊标签样式
- [ ] 编辑/删除策略

**验收**：admin 注册 MySQL 数据源 → `POST /api/datasources/:id/test` 返回 200 → 为 developer 分配 `select` 权限 → developer 调用 `POST /api/query/execute` 对无权限表返回 403

---

## Phase 3：SQL 查询引擎（2 工作日）

> 目标：核心功能——用户能在编辑器中执行 SQL 查询并看到结果

### Task 3.1 SQL 解析与静态规则

> Spec: `spec/ARCHITECTURE.md` 第 3 节 SQL 解析

- [ ] MySQL 解析器封装 (`pkg/sqlparser/` + pingcap/parser)
- [ ] MongoDB 解析器封装（BSON filter 解析 + 自定义规则）
- [ ] 操作类型提取（SELECT / DDL / DML）
- [ ] 表名提取
- [ ] 无 WHERE 的 UPDATE/DELETE → 高风险标记
- [ ] 静态拦截（DROP DATABASE/TABLE 直接拦截）
- [ ] MongoDB aggregation stage 白名单校验
- [ ] 敏感表判定（查询脱敏规则获取敏感表列表）

### Task 3.2 查询执行

> Spec: `spec/ARCHITECTURE.md` 第 7 节查询执行 + `spec/PRD.md` 第 2 节在线查询

- [ ] `POST /api/query/execute` — 执行 SQL
- [ ] SELECT 查询执行 + 结果集返回（分页，默认 1000 行上限）
- [ ] DDL/DML 拦截 → 返回"请提交工单"提示
- [ ] 查询超时控制（context timeout 30s）
- [ ] 执行后写入查询历史
- [ ] 执行后写入审计日志

### Task 3.3 查询历史

> Spec: `spec/ARCHITECTURE.md` 第 7 节查询历史

- [ ] Ent Schema: QueryHistory（id, user_id, datasource_id, database, sql_content, sql_summary, db_type, execution_time, result_rows, affected_rows, created_at）
- [ ] `GET /api/query/history` 查询历史列表（分页，按用户隔离）
- [ ] `DELETE /api/query/history/:id` 删除单条
- [ ] `DELETE /api/query/history` 清空历史
- [ ] 超出 200 条自动清理最旧记录

### Task 3.4 数据导出

> Spec: `spec/UI-DESIGN.md` 第 3.7 节导出交互 + `spec/PRD.md` 第 2 节导出

- [ ] `POST /api/query/export` 导出查询结果
- [ ] CSV 格式导出（后端流式返回）
- [ ] JSON 格式导出
- [ ] 导出上限 10000 行
- [ ] 导出记录写入审计日志

### Task 3.5 前端 - SQL 查询页

> Spec: `spec/UI-DESIGN.md` 第 3 节 SQL 查询页（核心页面）

- [ ] 数据源选择区（类型下拉 + 库名下拉）
- [ ] CodeMirror 6 编辑器集成（MySQL 语法高亮 + 自动补全）
- [ ] 可拖拽分割线（编辑器/结果表格 50:50，localStorage 记忆）
- [ ] 结果表格（TanStack Table + 分页 + 列排序 + 列搜索）
- [ ] 执行状态栏（耗时 / 行数 / 脱敏提示）
- [ ] 快捷键（Ctrl+Enter 执行 / Ctrl+Shift+F 格式化 / Ctrl+E 导出）
- [ ] 多 Tab 查询（独立上下文 + 未保存标记 + 标题自动截取）
- [ ] 查询历史面板（下拉 + 点击恢复 + 删除 + 清空）
- [ ] 导出交互（格式 Dropdown + 超量确认 + 流式下载）
- [ ] 空状态提示（无结果时"执行查询以查看结果"）

### Task 3.6 前端 - MongoDB 查询模式

> Spec: `spec/UI-DESIGN.md` 第 3.1.2 节 MongoDB 查询

- [ ] 编辑器切换 JSON 模式
- [ ] 操作类型下拉（find / aggregation / update）
- [ ] Filter/Pipeline JSON 编辑器
- [ ] Options JSON 编辑器（可折叠）
- [ ] 预设常用模板
- [ ] update 操作提示"提交工单"

**验收**：developer 登录 → 选择 MySQL 数据源 → 执行 `SELECT * FROM users LIMIT 10` → 结果表格显示 ≤10 行 → 新建 Tab → 从历史面板点击恢复查询 → 导出 CSV 文件可下载

---

## Phase 4：AI 评审 + 工单系统（2 工作日）

> 目标：变更操作走 AI 评审 + 工单审批流程

### Task 4.1 AI 评审服务

> Spec: `spec/ARCHITECTURE.md` 第 4 节 AI 评审流程 + `spec/PRD.md` 第 6 节 AI 评审

- [ ] 外部 LLM API 调用封装（SSE 流式）
- [ ] 评审 prompt 工程（风险分级 + 优化建议 + 影响分析）
- [ ] 风险判定逻辑（操作类型 + 敏感等级 + WHERE 条件 + 影响范围）
- [ ] 评审结果有效期 30s
- [ ] 降级策略（超时→中风险 / 离线→静态规则兜底 / 不可达→提示重试）

### Task 4.2 工单系统

> Spec: `spec/ARCHITECTURE.md` 第 6 节工单状态机 + `spec/PRD.md` 第 5 节变更审批

- [ ] Ent Schema: Ticket（id, submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, ai_review_result JSON, reviewer_id, reviewed_at, executed_at, created_at, updated_at）
- [ ] `POST /api/tickets` 提交工单
- [ ] `GET /api/tickets` 工单列表（分页 + 多维筛选）
- [ ] `GET /api/tickets/:id` 工单详情
- [ ] `POST /api/tickets/:id/approve` 审批通过
- [ ] `POST /api/tickets/:id/reject` 审批驳回（必填原因）
- [ ] `POST /api/tickets/:id/cancel` 取消工单（必填原因）
- [ ] `POST /api/tickets/:id/execute` 手动执行（仅提交人或 dba/admin）
- [ ] 工单状态机（SUBMITTED→AI_REVIEWED→PENDING_APPROVAL→APPROVED→EXECUTING→DONE / REJECTED / CANCELLED）
- [ ] 所有状态变更记录审计日志

### Task 4.3 脱敏引擎

> Spec: `spec/ARCHITECTURE.md` 第 5 节数据脱敏 + `spec/PRD.md` 第 7 节数据脱敏

- [ ] Ent Schema: MaskRule（id, datasource_id, database, table, field, mask_type, custom_regex, custom_template）
- [ ] Ent Schema: SensitiveTable（id, datasource_id, database, table, sensitivity_level）
- [ ] 脱敏规则引擎 (`pkg/mask/`) — 库.表.字段三级匹配
- [ ] 内置脱敏类型（手机号/身份证/姓名/邮箱/银行卡/地址/全掩码/自定义正则）
- [ ] Casbin `desensitize:bypass` 权限控制
- [ ] `POST/GET/PUT/DELETE /api/mask-rules` 脱敏规则 CRUD
- [ ] 脱敏结果写入审计日志

### Task 4.4 前端 - AI 评审交互

> Spec: `spec/UI-DESIGN.md` 第 4 节 AI 评审交互设计

- [ ] SSE 连接（fetch + ReadableStream + POST + JWT）
- [ ] 评审中动画（脉冲 + 实时建议追加）
- [ ] 评审完成卡片（风险等级 + 建议 + 影响分析）
- [ ] 按风险分级操作按钮（低自动执行 / 中确认 / 高提交工单）
- [ ] 高风险工单提交抽屉（填变更原因，不跳转页面）

### Task 4.5 前端 - 变更工单页

> Spec: `spec/UI-DESIGN.md` 第 5 节变更工单页

- [ ] 工单列表（状态 Tab + 未读角标 + 快捷筛选 + 高级筛选 + SQL 搜索）
- [ ] 工单详情抽屉（60% 宽度 + 列表不消失）
- [ ] 审批操作（通过/拒绝/取消）+ 驳回原因必填
- [ ] 评论输入（纯文本，Enter 发送）
- [ ] 新建工单独立页面 `/tickets/new`
- [ ] 工单手动执行按钮 + 执行状态展示

### Task 4.6 前端 - 脱敏规则配置

> Spec: `spec/UI-DESIGN.md` 第 9 节设置页（脱敏规则子页面）

- [ ] 敏感表标记列表（敏感等级 badge + 筛选 + 标记/取消）
- [ ] 脱敏字段规则列表（按表筛选 + 卡片展示）
- [ ] 添加敏感表表单（数据源+库+表+敏感等级）
- [ ] 添加脱敏规则表单（数据源+库+表+字段+脱敏类型）
- [ ] 自定义正则配置

**验收**：developer 写 `ALTER TABLE users ADD COLUMN phone VARCHAR(20)` → AI 评审返回高风险卡片 → 点击提交工单填变更原因 → dba 登录看到待审批工单 → 审批通过 → 手动执行 → 审计日志含该 DDL 记录

---

## Phase 5：审计 + 通知 + 设置（1.5 工作日）

> 目标：完整的审计追踪、钉钉通知、设置页面收尾

### Task 5.1 审计日志

> Spec: `spec/ARCHITECTURE.md` 审计日志 + `spec/PRD.md` 第 8 节审计日志

- [ ] Ent Schema: AuditLog（id, user_id, action, datasource_id, database, sql_content, sql_summary, result_rows, affected_rows, execution_time_ms, error_message, ai_review_result JSON, ticket_id, desensitized_fields, ip_address, created_at）
- [ ] 异步写入（内存队列 + 批量刷盘：1s 或 100 条）
- [ ] `GET /api/audit-logs` 审计日志（分页 + 多维筛选）
- [ ] 应用层禁止删除审计日志

### Task 5.2 钉钉通知

> Spec: `spec/PRD.md` 第 9 节通知

- [ ] `notify.go` 钉钉 Webhook 集成（签名验证）
- [ ] 工单提交/审批结果通知
- [ ] 中高风险操作实时告警
- [ ] 通知内容模板（操作人 + SQL 摘要 + 风险等级 + 工单链接）
- [ ] 设置页 AI 配置保存（provider/model/api_key/timeout）

### Task 5.3 前端 - 审计日志页

> Spec: `spec/UI-DESIGN.md` 第 7 节审计日志页

- [ ] 审计日志表格（时间 / 用户 / 操作类型 / 数据库 / SQL 摘要）
- [ ] 多维筛选（用户 / 操作类型 / 日期范围）
- [ ] SQL 模糊搜索
- [ ] 行展开详情（完整 SQL / 耗时 / 行数 / 脱敏信息 / AI 评审 / 关联工单）
- [ ] 导出 CSV

### Task 5.4 前端 - 设置页收尾

> Spec: `spec/UI-DESIGN.md` 第 9 节设置页（AI 配置子页面）

- [ ] AI 配置页面（API 地址 / 模型 / 超时 / 风险阈值 / 评审开关）
- [ ] 脱敏规则页（已在 Task 4.6 完成）
- [ ] 数据源管理优化（状态指示灯 + 健康检查结果显示）

### Task 5.5 前端 - 全局交互

> Spec: `spec/UI-DESIGN.md` 第 11 节全局交互规范

- [ ] Cmd+K 全局搜索弹窗（搜索工单/审计日志/页面导航）
- [ ] 403 / 404 错误页面
- [ ] 网络断开 banner
- [ ] 二次确认弹窗组件（统一 AlertDialog）
- [ ] 侧边栏折叠/展开

**验收**：执行 3 次不同类型查询（SELECT/DDL/DML）→ 审计日志页显示 3 条记录，筛选操作类型可精确过滤 → 钉钉 Webhook URL 配置后触发通知 → Cmd+K 搜索 SQL 摘要可定位到对应审计记录

---

## Phase 6：集成测试 + Docker 部署（1 工作日）

> 目标：MVP 可交付，Docker 一键部署

### Task 6.1 后端集成测试

> Spec: `spec/ARCHITECTURE.md` 第 8 节错误处理 + 各 API 端点

- [ ] 认证流程测试（登录/JWT/过期）
- [ ] 权限测试（RBAC 路由校验）
- [ ] 工单状态机测试
- [ ] 脱敏引擎单元测试
- [ ] SQL 解析器单元测试
- [ ] 审计日志写入测试

### Task 6.2 前端 E2E

> Spec: `spec/UI-DESIGN.md` 第 11 节全局交互规范

- [ ] 登录→查询→工单完整流程
- [ ] 权限隔离验证（developer 不能访问管理功能）
- [ ] 全局交互验证（401 跳转 / 404 页面 / 网络断开）

### Task 6.3 Docker 部署

> Spec: `spec/ARCHITECTURE.md` 项目结构 + `spec/PRD.md` MVP 约束

- [ ] 多阶段 Dockerfile（Go build + 前端 build → 单二进制 + 静态文件）
- [ ] docker-compose.yaml（环境变量注入敏感配置）
- [ ] 健康检查（`/api/health`）
- [ ] 启动脚本（自动创建初始管理员）

### Task 6.4 文档收尾

- [ ] README.md（项目介绍 + 快速启动 + 配置说明）
- [ ] API 文档（OpenAPI/Swagger 或 Markdown）
- [ ] 部署文档

**验收**：全新环境 `docker-compose up -d` → 浏览器打开 `http://localhost:3000` → 管理员登录 → 完整走一遍查询+工单+审计流程 → 无报错

---

## 工时估算

| Phase | 内容 | 后端 | 前端 | 总工时 |
|-------|------|------|------|--------|
| Phase 0 | 项目脚手架 | 0.5d | 0.5d | **1d** |
| Phase 1 | 认证与用户管理 | 0.5d | 1d | **1.5d** |
| Phase 2 | 数据源与权限 | 1d | 0.5d | **1.5d** |
| Phase 3 | SQL 查询引擎 | 1d | 1d | **2d** |
| Phase 4 | AI 评审 + 工单 | 1d | 1d | **2d** |
| Phase 5 | 审计 + 通知 + 设置 | 0.5d | 1d | **1.5d** |
| Phase 6 | 测试 + 部署 | 0.5d | 0.5d | **1d** |
| **合计** | | **5d** | **5.5d** | **10.5d** |

> 注：以上为 Claude Code 执行的估算工时，实际可能因 AI 效率波动 ±30%。建议每个 Phase 完成后做一次验收再进入下一 Phase。

## 依赖关系

```
Phase 0 (脚手架)
  ├── Phase 1 (认证 + 用户)
  │     ├── Phase 2 (数据源 + 权限)
  │     │     └── Phase 3 (SQL 查询引擎) ← Phase 2 的数据源和权限是前置
  │     │           └── Phase 4 (AI 评审 + 工单) ← Phase 3 的查询执行是前置
  │     │                 └── Phase 5 (审计 + 通知) ← Phase 4 的工单是前置
  │     │                       └── Phase 6 (测试 + 部署) ← 全部完成
```

## 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| Claude Code 单次任务超时 | 复杂 Task 可能中断 | 拆分为更小的子任务，用 `--resume` 继续 |
| pingcap/parser Go 版本兼容 | SQL 解析器集成失败 | 预先验证版本兼容性，备选手写正则解析 |
| SSE 流式前后端对接 | AI 评审卡片交互不流畅 | Phase 4 优先做后端 SSE 端点，用 curl 验证后再做前端 |
| CodeMirror 6 MongoDB 模式 | JSON 编辑器体验差 | 预设模板降低门槛，必要时自定义 extension |
| Casbin Ent Adapter 成熟度 | 权限系统 bug | Phase 2 重点测试，预留调试时间 |
