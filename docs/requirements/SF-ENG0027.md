# SF-ENG0027 导出上限提升

> 状态：待评审
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-27
> 需求编号：recvkMoLmJczFo

## 需求概述

将查询结果导出上限从 10000 行提升至 100000 行，大结果集走异步导出任务。

## 评审结论

待评审。

## 技术方案

### 方案

- **≤ 10000 行**：保持同步导出（当前逻辑不变）
- **> 10000 行**：创建异步导出任务，后台处理

### 数据库变更

新增 `export_tasks` 表：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| user_id | INTEGER NOT NULL | 发起人 |
| datasource_id | INTEGER NOT NULL | 数据源 |
| query | TEXT NOT NULL | 查询 SQL |
| format | TEXT NOT NULL | csv/json |
| status | TEXT NOT NULL | pending/processing/completed/failed |
| file_path | TEXT | 导出文件路径 |
| file_size | INTEGER | 文件大小（字节） |
| row_count | INTEGER | 导出行数 |
| error_message | TEXT | 失败原因 |
| created_at | DATETIME | 创建时间 |
| completed_at | DATETIME | 完成时间 |

### 接口设计

| 接口 | 说明 |
|------|------|
| POST /api/exports | 创建导出任务（同步/异步自动判断） |
| GET /api/exports | 导出任务列表 |
| GET /api/exports/:id | 导出任务状态 |
| GET /api/exports/:id/download | 下载导出文件 |

### 异步处理

- 后台 Worker 处理异步导出（复用 Scheduler goroutine）
- 大文件写入临时目录 `state/exports/`
- 完成后通过钉钉通知用户 + 前端轮询状态

### 文件清理

- 导出文件 24h 后自动删除（Scheduler 清理任务）
- 对应 export_tasks 记录标记为 expired

## 验收标准

1. ≤ 10000 行同步导出，体验不变
2. > 10000 行自动异步，前端显示进度
3. 导出上限提升至 100000 行
4. 下载链接有效期内可用
5. 过期文件自动清理
6. 导出失败有明确错误提示

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
