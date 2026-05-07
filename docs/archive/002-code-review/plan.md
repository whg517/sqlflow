# 002-code-review — 代码评审修复计划

> 基于 2026-05-07 全面代码评审发现的 22 个问题，按优先级分批修复。

---

## 配置规范（前置）

### config.yaml 不入 Git（已确认 ✅）
`.gitignore` 已包含 `config.yaml`。

### 提供 config.example.yaml
创建 `config/config.example.yaml`，包含所有配置项及示例值和注释说明。

### JWT Secret 示例值
```
jwt:
  secret: "your-32-char-secret-change-me-in-production"
  expiry: "24h"
```

### Encryption Key 示例值
```
encryption_key: "change-me-to-32-byte-hex-string"
```

### 启动时校验
- JWT Secret 长度 < 16 时 warn，< 32 时建议提示
- Encryption Key 长度必须是 16 或 32（对应 AES-128/AES-256）
- Admin 默认密码检测：如果 admin 密码仍是默认值，启动时 warn

---

## Phase A: 安全关键修复（P0）

| Task ID | 名称 | 问题编号 | 描述 |
|---------|------|---------|------|
| A.1 | Permission 中间件参数来源修复 | #10 | Permission 中间件从 query param 读取 datasource/table，但客户端发 JSON body，导致权限检查可能完全未生效。改为从 path/body context 读取 |
| A.2 | TRUNCATE 拦截误拦修复 | #8 | `applyMySQLRules` 中 `OpDDL && Tables > 0` 误拦所有 DDL（CREATE/ALTER），需精确识别 TRUNCATE 语句 |
| A.3 | rows.Err() 全局修复 | #5 | audit.go、mask_rule.go、query_history.go、permission.go 中 `for rows.Next()` 后未检查 `rows.Err()` |
| A.4 | AES 密钥长度校验 | #2 | crypto 包增加 `ValidateKey()` 函数，启动时校验 encryption_key 长度 |
| A.5 | LIKE 通配符转义 | #3 | audit.go 中 keyword 的 `%` 和 `_` 需要转义 |

## Phase B: 安全与稳定性（P1）

| Task ID | 名称 | 问题编号 | 描述 |
|---------|------|---------|------|
| B.1 | 审计日志不丢弃 | #4 | 改为阻塞写入（带超时）或直接同步写 SQLite（WAL 本身足够快），丢弃时增加 metrics |
| B.2 | 错误信息脱敏 | #13 | handler 层 default 分支不返回 `err.Error()` 给前端，生产环境只返回通用错误，详情写日志 |
| B.3 | 连接池复用 — MySQL | #11 | 实现 datasourceID → *sql.DB 缓存，避免每次查询新建连接 |
| B.4 | 连接池复用 — MongoDB | #12 | 同上，实现 MongoDB client 缓存 |
| B.5 | SQL Parser 单元测试 | #20 | 为 parser.go、mysql.go、mongodb.go 补充单元测试 |
| B.6 | Mask 单元测试 | #20 | 为 mask.go 补充各脱敏类型的测试 |

## Phase C: 代码质量（P2）

| Task ID | 名称 | 问题编号 | 描述 |
|---------|------|---------|------|
| C.1 | 示例配置文件 | 配置 | 创建 `config/config.example.yaml`，启动校验 jwt secret 和 encryption key |
| C.2 | 分页逻辑抽取 | #14 | 提取 Pagination helper，消除 4+ 处重复代码 |
| C.3 | 审计写入统一 | #15 | 所有审计写入统一走 AuditService，删除 MaskRuleService.WriteAuditLog |
| C.4 | Context 传递 | #16 | handler → service → db 全链路传递 context.Context |
| C.5 | Ent Schema 清理 | #7 | 删除未使用的 ent schema 目录（如果确认不用 Ent） |
| C.6 | DesensitizeService 实现或删除 | #6 | 要么实现并迁移脱敏逻辑，要么删除空文件 |
| C.7 | 前端目录整理 | #19 | 合并 Settings/settings 重复目录 |
| C.8 | Dockerfile 健康检查 | #21 | 添加 HEALTHCHECK 指令 |

---

## 依赖关系

```
A.1-A.5 (可并行) → B.1-B.6 (可并行) → C.1-C.8 (可并行)
```

所有 Phase A 任务优先于 B，B 优先于 C。同 Phase 内任务可并行。

---

## 验收标准

每个 Task 完成后：
1. `go build ./...` 通过
2. `go test ./...` 通过
3. 不引入新的 lint 警告
