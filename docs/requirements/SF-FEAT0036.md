# SF-FEAT0036 Elasticsearch 数据源支持

> 状态：需修改 → 修改中
> 优先级：P1🟠重要
> 负责人：待分配
> 创建日期：2026-05-27
> 需求编号：recvkMoLmMYFQR

## 需求概述

SQLFlow 新增 Elasticsearch 数据源，支持 ES DSL 查询、index 管理、查询历史和导出。

## 评审结论

### 🎨 UI/UX 评审（苏晴）：⚠️ 需修改
需补充：ES 特有配置参数、ElasticEditor 交互线框图、结果展示方案（hits 展平）、QueryPage 类型分发重构、工单字段通用化。

### 修改响应

以下根据苏晴评审意见补充技术方案。

## 技术方案

### 架构设计

- **Go 客户端**：go-elasticsearch/v8（官方维护，兼容 ES 7.x/8.x）
- **connpool 扩展**：新增 `esClients sync.Map`（key: `es:{dsID}:{host}`），缓存 `*es.TypedClient`
- **查询模式**：QueryPage 根据数据源类型分发编辑器（见前端重构方案）
- **权限控制**：index 级别隔离（Casbin policy 扩展）
- **DSL 校验**：前端 JSON Schema 校验 + 后端白名单 query 类型检查

### 前端重构：QueryPage 类型分发

当前 QueryPage 硬编码 SqlEditor + MongoEditor，需重构为**编辑器工厂模式**：

```
QueryPage
├── SqlEditor         (type === "mysql" || "postgresql")
├── MongoEditor       (type === "mongodb")
└── ElasticEditor     (type === "elasticsearch") ← 新增
```

**ElasticEditor 设计**：
- 基于 CodeMirror 6 JSON mode（已安装 `@codemirror/lang-json`）
- 预置 DSL 模板：`{"query": {"match_all": {}}, "size": 100}`
- 语法高亮 + 自动补全（index 字段名从 mapping 获取）
- 顶部工具栏：Index 选择器（下拉，API 获取 index 列表）、执行按钮、格式化按钮

### ES 特有配置参数

数据源注册时新增字段：

| 字段 | 类型 | 说明 | 示例 |
|------|------|------|------|
| auth_type | enum | 认证方式：none/basic/api_key/bearer | basic |
| api_key | text | API Key（AES 加密存储） | - |
| es_version | int | ES 版本号（7 或 8，影响 query DSL 兼容性） | 8 |
| verify_certs | bool | 是否验证 TLS 证书 | true |
| es_sniff | bool | 是否启用集群嗅探 | false |
| index_pattern | text | 索引模式（可选，限制可查询的索引范围） | logs-* |

### 数据库变更

`datasources` 表新增列：

```sql
ALTER TABLE datasources ADD COLUMN auth_type TEXT DEFAULT '';
ALTER TABLE datasources ADD COLUMN api_key TEXT DEFAULT '';
ALTER TABLE datasources ADD COLUMN es_version INTEGER DEFAULT 8;
ALTER TABLE datasources ADD COLUMN verify_certs BOOLEAN DEFAULT 1;
ALTER TABLE datasources ADD COLUMN es_sniff BOOLEAN DEFAULT 0;
ALTER TABLE datasources ADD COLUMN index_pattern TEXT DEFAULT '';
```

### 接口设计

| 接口 | 说明 |
|------|------|
| POST /api/datasources | 新增 elasticsearch 类型 + 上述字段 |
| POST /api/query/execute | 支持 elasticsearch 类型 DSL 查询 |
| GET /api/datasources/:id/indices | 返回 index 列表（支持 index_pattern 过滤） |
| GET /api/datasources/:id/indices/:name/mapping | 返回指定 index mapping |

### 查询结果展示方案（hits 展平）

ES `_search` 返回结构：
```json
{
  "took": 5,
  "hits": {
    "total": { "value": 100 },
    "hits": [
      { "_id": "1", "_index": "logs", "_source": {"message": "hello", "level": "info"} },
      ...
    ]
  },
  "aggregations": { ... }
}
```

**前端展示逻辑**：
- **纯查询结果**（无 aggregations）：展平 `_source` 字段为表格列 + 固定列 `_id`、`_index`、`_score`
- **聚合结果**（有 aggregations）：JSON 树形展示（折叠面板）
- **混合结果**：Tab 切换「文档结果」/「聚合结果」
- 分页：使用 `from` + `size` 参数实现 ES 端分页

**字段类型映射**（ES mapping type → 前端展示）：

| ES type | 展示 |
|---------|------|
| text/keyword | string |
| long/integer/short/byte | integer |
| double/float/half_float/scaled_float | number |
| date | date |
| boolean | boolean |
| nested/object | JSON 折叠 |
| geo_point | string (lat,lon) |

### 工单字段通用化

当前工单的 `sql_content` 字段语义需扩展为 `query_content`：
- 数据库列名重命名（`sql_content` → `query_content`），需 migration
- 前端工单提交：MySQL/PG 存 SQL，MongoDB 存 JSON，ES 存 DSL
- 工单详情展示时根据关联数据源类型选择对应渲染器

### ES 安全约束

- 仅允许 `_search` API（`_bulk`、`_delete_by_query`、`_update_by_query` 等写操作走工单）
- DSL 中禁止 `script` 字段（安全风险）
- index_pattern 限制可查询范围
- 单次查询 `size` 上限 10000（ES 默认 max_result_window）

## 设计规范

- 数据源表单：ES 类型时动态显示认证方式选择、ES 版本、TLS、索引模式等字段
- 非 ES 类型时隐藏这些字段
- ElasticEditor 深色/浅色主题跟随全局主题

## 验收标准

1. ES 数据源注册 + 连接测试
2. DSL 查询执行 + 结果分页展示（hits 展平 + aggregations）
3. Index 列表 + mapping 查询（支持 index_pattern 过滤）
4. 查询历史和导出正常
5. index 级权限隔离
6. 禁止危险操作（_bulk/_delete_by_query/script）
7. 工单支持 ES DSL 提交和审批

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
| 2026-05-27 | 根据苏晴 UI/UX 评审补充：ElasticEditor 设计、hits 展平方案、QueryPage 类型分发重构、工单字段通用化、ES 特有配置参数、安全约束 |
