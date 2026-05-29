# SF-FEAT0028 SQL 模板/片段库

> 状态：需修改 → 修改中
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-27
> 需求编号：recvkMokroat4N

## 需求概述

常用 SQL 模板保存、复用、团队共享，减少重复编写。

## 评审结论

### 🏗️ 架构评审（方远）：⚠️ 需修改
- 🔴 P0：SQL 注入防护，模板渲染必须输出参数化 SQL
- 需补充：模板引擎选型 ADR、数据模型设计、API 设计、数据库方言兼容性

### 🧪 测试策略评审（叶青）：⚠️ 需修改
- 需求描述为空，需补充功能规格
- 需确认：模板复杂度上限、参数占位符语法、与编辑器集成方式
- 边界：超长 SQL、特殊字符、循环引用、并发编辑

### 🎨 UI/UX 评审（苏晴）：⚠️ 需修改
- 交互设计方案已提供（模板库入口、列表、编辑器集成）
- 需补充：参数语法规范、模板权限模型、并发编辑冲突处理

### 修改响应

以下根据三方评审意见补充完整技术方案。

## 技术方案

### ADR-001：模板引擎选型

**决策：不使用模板引擎，采用简单字符串替换 + 参数化查询**

| 方案 | 优点 | 缺点 | 结论 |
|------|------|------|------|
| Go text/template | 功能强大（条件/循环） | 🔴 SQL 注入风险极高 | ❌ 排除 |
| pongo2 / fasttemplate | 模板语法 | ⚠️ 仍有注入风险 | ❌ 排除 |
| 简单占位符替换 | 安全可控，Go 实现简单 | 不支持条件/循环 | ✅ 采纳 |

**核心原则：模板渲染输出 → 参数化 SQL，不拼接用户输入到 SQL 字符串**

### 参数占位符语法

```
{{param_name}}           — 必填参数
{{param_name:value}}     — 可选参数（带默认值）
```

示例模板：
```sql
SELECT * FROM {{table_name}}
WHERE created_at >= '{{start_date}}'
  AND status = '{{status}}'
ORDER BY created_at DESC
LIMIT {{limit:100}};
```

**渲染流程**：
1. 解析模板，提取所有 `{{param_name}}` 占位符
2. 前端弹出参数填写表单（必填标红星，可选有默认值）
3. 后端接收用户填写的参数值
4. 将占位符替换为 `?` 占位符（MySQL/PG）或 `$1, $2...`（PostgreSQL）
5. 参数值通过 `database/sql` 参数化查询传入（**不拼接字符串**）

**示例**：
```
模板：SELECT * FROM {{table}} WHERE id = {{id}}
用户填写：table=users, id=42
输出 SQL：SELECT * FROM users WHERE id = ?
传入参数：[]interface{}{42}
```

### 安全设计

| 威胁 | 防护 |
|------|------|
| SQL 注入 | 参数化查询，用户输入永不拼接进 SQL |
| 模板本身恶意 SQL | 创建模板时 AST 校验（复用现有 pingcap/parser） |
| 越权访问他人模板 | RBAC：个人模板仅创建者可编辑，公共模板仅 Admin 可管理 |
| 模板名 XSS | 前端渲染转义，后端存储不限制 |

### 数据模型

主表 `sql_templates`：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| name | TEXT NOT NULL | 模板名称（max 100 字符） |
| description | TEXT | 描述（max 500 字符） |
| sql_content | TEXT NOT NULL | SQL 内容（max 10KB） |
| db_type | TEXT NOT NULL | 数据源类型：mysql/postgresql/mongodb/common |
| category | TEXT | 分类（max 50 字符） |
| tags | TEXT | 标签（JSON 数组，max 10 个） |
| is_public | BOOLEAN DEFAULT false | 是否公共模板 |
| parameters | TEXT | 参数定义（JSON 数组，从模板自动提取） |
| created_by | INTEGER NOT NULL | 创建者 user_id |
| use_count | INTEGER DEFAULT 0 | 使用次数 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

**约束**：
- `sql_content` 最大 10KB，防止超长模板
- `name` + `created_by` 联合唯一（同用户不重名）
- 公共模板数量上限：50（防止滥用）

### 数据库方言兼容性

| 方言 | 占位符 | 参数传递 |
|------|--------|----------|
| MySQL | `?` | `args...` |
| PostgreSQL | `$1, $2, ...` | `args...`（pgx 驱动原生支持） |
| MongoDB | N/A（JSON body） | 直接替换占位符为值 |

MongoDB 模板处理不同：占位符替换为实际值（JSON 序列化），不涉及 SQL 注入，但需做 JSON 值校验（防止 NoSQL 注入）。

### 接口设计

| 接口 | 方法 | 说明 |
|------|------|------|
| /api/templates | GET | 列表（分页、搜索、分类筛选、db_type 过滤） |
| /api/templates | POST | 创建模板 |
| /api/templates/:id | GET | 获取详情 |
| /api/templates/:id | PUT | 更新模板 |
| /api/templates/:id | DELETE | 删除模板（仅创建者或 Admin） |
| /api/templates/:id/toggle-public | PUT | 切换公共/私有（Admin） |
| /api/templates/:id/render | POST | 渲染模板（返回参数化 SQL + 参数列表） |
| /api/templates/:id/use | POST | 使用模板（渲染 + 执行，结果返回） |

### 模板复杂度边界

- **不支持**：条件逻辑（if/else）、循环（for）、嵌套模板引用
- **支持**：简单占位符替换 + 默认值
- 原因：避免引入模板引擎带来安全风险，简单替换覆盖 80% 使用场景

### 并发编辑冲突

- 不引入锁机制或版本冲突检测（OT/CRDT）
- 最后写入覆盖（Last Write Wins）
- 模板编辑频率低，冲突概率极低
- 如有需要后续可加乐观锁（version 字段）

### 前端交互（基于苏晴设计方案）

- **入口**：查询页面工具栏「📋 模板」按钮 + 设置页「SQL 模板」Tab
- **编辑器侧边面板**：Sheet 从右滑出，展示模板列表快速插入
- **模板编辑**：双栏布局（左侧参数表单 + 右侧 SQL 预览）
- **编辑器集成**：自动补全中增加 tpl_ 前缀触发模板补全
- **快捷键**：Cmd/Ctrl+Shift+T 打开模板面板

## 验收标准

1. 个人模板 CRUD 完整
2. 团队公共模板 Admin 可管理
3. SQL 编辑器支持一键插入模板
4. 参数占位符替换输出参数化 SQL（不拼接用户输入）
5. 按数据源类型筛选模板
6. 模板创建时 AST 校验 SQL 类型
7. 超长 SQL（>10KB）拒绝创建
8. 特殊字符参数值正确处理
9. MongoDB 模板正常工作

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
| 2026-05-27 | 根据三方评审补充：ADR（不使用模板引擎）、参数语法规范、安全设计（参数化查询）、数据模型、方言兼容、并发策略、前端交互方案 |
