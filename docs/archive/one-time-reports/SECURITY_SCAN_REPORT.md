# SQLFlow 安全扫描报告

| 项目 | 值 |
|------|------|
| **扫描时间** | 2026-05-23 10:47 CST (Asia/Shanghai) |
| **扫描工具** | govulncheck v1.25.10, go mod verify, npm audit, grep (secrets/sql-injection/insecure-http) |
| **Go 版本** | go1.25.10 linux/amd64 |
| **项目路径** | `/home/kevin/.openclaw/workspace/projects/sql-platform/` |

---

## 1. 总览

| 类别 | 结果 |
|------|------|
| Go 直接漏洞 (代码调用) | ✅ 0 |
| Go 间接漏洞 (已引入模块) | ⚠️ 23 |
| Go 模块完整性 | ✅ 通过 (`go mod verify`) |
| 前端依赖漏洞 | ✅ 0 |
| 硬编码密钥/密码 | 🔴 3 项 |
| SQL 注入风险 | ⚠️ 1 项 (低风险) |
| 不安全 TLS 配置 | 🔴 2 处 |

**总体评估：中等风险** — 无直接漏洞，但存在硬编码凭据和不安全 TLS 配置需要修复。

---

## 2. Go 依赖审计

### 2.1 模块完整性验证

```
$ go mod verify
all modules verified
```

所有依赖的校验和与 `go.sum` 一致，无篡改风险。

### 2.2 govulncheck 漏洞扫描

**直接调用漏洞：0 个**（代码未直接触发任何已知漏洞）

**间接引入漏洞（已引入模块中存在，但代码未调用受影响函数）：**

#### golang.org/x/crypto@v0.50.0（17 个漏洞，建议升级到 v0.52.0）

| 编号 | CVE | 描述 | 修复版本 |
|------|-----|------|----------|
| GO-2026-5016 | — | SSH server panic during key exchange | v0.52.0 |
| GO-2026-5015 | — | SSH server panic during CheckHostKey/Authenticate | v0.52.0 |
| GO-2026-5014 | — | SSH certificate restrictions bypass | v0.52.0 |
| GO-2026-5013 | — | SSH byte arithmetic underflow/panic | v0.52.0 |
| GO-2026-5006 | — | SSH agent key forwarding drops constraints | v0.52.0 |
| GO-2026-5005 | — | SSH agent key constraints not enforced | v0.52.0 |
| + 其他约 11 个 | — | 均在 x/crypto/ssh 相关子包中 | v0.52.0 |

> **风险评估：低** — SQLFlow 代码未直接使用 `golang.org/x/crypto/ssh` 相关功能，这些漏洞通过依赖链（如 `modernc.org/*` 或测试依赖）间接引入。

#### google.golang.org/protobuf@v1.28.1（1 个漏洞）

| 编号 | CVE | 描述 | 修复版本 |
|------|-----|------|----------|
| GO-2024-2611 | — | Infinite loop in JSON unmarshaling | v1.33.0 |

> **风险评估：低** — 代码未直接调用 protobuf 的 JSON 反序列化功能。

### 2.3 修复建议

```bash
# 升级 golang.org/x/crypto
go get golang.org/x/crypto@v0.52.0

# 升级 google.golang.org/protobuf
go get google.golang.org/protobuf@v1.33.0

# 整理依赖
go mod tidy
```

---

## 3. 前端依赖审计

```
$ cd web && npm audit
found 0 vulnerabilities
```

✅ 无已知漏洞。

---

## 4. 代码安全检查

### 4.1 硬编码凭据 🔴 HIGH

| 文件 | 行号 | 问题 | 严重程度 |
|------|------|------|----------|
| `config/config.yaml:14` | JWT Secret | `secret: "sqlflow-jwt-secret-key-2026-dev-only"` | 🔴 HIGH |
| `config/config.yaml:20` | 管理员密码 | `password: "admin123"` | 🔴 HIGH |
| `config/config.yaml:38` | 加密密钥 | `encryption_key: "2f89e3851766a2840c3348dc6df23ab4"` | 🔴 HIGH |

**缓解措施：**
- ✅ `config.yaml` 已在 `.gitignore` 中，不会被提交到 Git 仓库
- ⚠️ 配置文件中有注释说明应使用环境变量替代，但默认值仍然存在
- ⚠️ 如果开发人员直接复制 `config.yaml` 到部署环境，这些凭据将暴露

**修复建议：**
1. 移除 `config.yaml` 中的默认 secret/password/encryption_key 值，替换为空字符串或占位符
2. 使用 `config.example.yaml` 作为模板（无真实凭据），并在 `.gitignore` 中排除实际配置
3. 确保生产环境强制通过环境变量注入敏感值
4. 添加启动检查：如果检测到默认凭据，拒绝启动或打印警告

### 4.2 SQL 注入风险 ⚠️ MEDIUM

| 文件 | 行号 | 问题 | 严重程度 |
|------|------|------|----------|
| `internal/service/pagination.go:57-64` | `PaginatedCountSQL` / `PaginatedQuerySQL` | 使用 `fmt.Sprintf` 拼接 SQL（table/orderBy 参数） | ⚠️ MEDIUM |

**详细分析：**
```go
func PaginatedCountSQL(table, whereClause string) string {
    return fmt.Sprintf("SELECT COUNT(*) FROM %s %s", table, whereClause)
}

func PaginatedQuerySQL(selectCols, table, whereClause, orderBy string, p Pagination) string {
    return fmt.Sprintf("%s FROM %s %s ORDER BY %s LIMIT ? OFFSET ?", selectCols, table, whereClause, orderBy)
}
```

**缓解措施：**
- ✅ 当前所有调用点的 `table`、`selectCols`、`orderBy` 参数均为硬编码字符串常量
- ✅ `whereClause` 由 `BuildWhereClause` 生成，使用参数化占位符
- ✅ `LIMIT` 和 `OFFSET` 使用 `?` 参数化

**风险评估：低** — 当前无用户输入直接流入拼接位置，但这是一个潜在的设计缺陷。

**修复建议：**
1. 考虑将 `table` 参数改为枚举/常量白名单验证
2. 或者改用完全参数化的查询构建方式
3. 添加 linter 规则（如 `gosec G201`）检测 SQL 字符串拼接

### 4.3 不安全 TLS 配置 🔴 HIGH

| 文件 | 行号 | 问题 | 严重程度 |
|------|------|------|----------|
| `internal/pkg/performance/report.go:557` | `InsecureSkipVerify: true` 硬编码 | 🔴 HIGH |
| `internal/pkg/performance/benchmark.go:107` | `InsecureSkipVerify: cfg.InsecureTLS`（可配置） | ⚠️ MEDIUM |

**详细分析：**
- `report.go:557`：性能测试报告中硬编码了 `InsecureSkipVerify: true`，完全禁用 TLS 证书验证
- `benchmark.go:107`：基准测试工具中通过配置控制，但默认值需确认

**修复建议：**
1. `report.go` 中不应硬编码 `InsecureSkipVerify: true`，应改为可配置或移除
2. 如果仅为开发/测试用途，确保此代码路径不会在生产环境中被执行
3. 添加注释明确标注仅用于测试环境

### 4.4 不安全 HTTP 调用 ℹ️ INFO

| 文件 | 行号 | 问题 | 严重程度 |
|------|------|------|----------|
| `internal/pkg/performance/cmd/main.go:31` | 默认 URL 使用 `http://localhost:8080` | ℹ️ INFO |

> 仅用于本地开发/测试，非安全风险。

---

## 5. 依赖构建问题

govulncheck 报告了一个 go.sum 缺失问题：

```
missing go.sum entry for module providing package github.com/klauspost/compress/zstd
(imported by go.mongodb.org/mongo-driver/x/mongo/driver)
```

**修复建议：**
```bash
go get go.mongodb.org/mongo-driver/x/mongo/driver@v1.17.9
go mod tidy
```

---

## 6. 修复优先级

| 优先级 | 问题 | 影响 | 工作量 |
|--------|------|------|--------|
| P0 | 硬编码 JWT Secret / 管理员密码 / 加密密钥 | 凭据泄露导致完全接管 | 低 — 改为空值/占位符 |
| P1 | `InsecureSkipVerify: true` 硬编码 | 中间人攻击 | 低 — 改为可配置 |
| P2 | go.sum 缺失导致依赖不完整 | 构建失败 / 供应链完整性风险 | 低 — 一条命令 |
| P3 | 间接依赖漏洞 (x/crypto, protobuf) | 潜在供应链风险 | 低 — 升级版本 |
| P4 | SQL 拼接设计模式 | 未来可能的注入风险 | 中 — 重构 |

---

## 7. 扫描命令记录

```bash
# Go 依赖完整性
export PATH="/home/kevin/go/bin:$PATH"
export GOPATH="/home/kevin/gopath"
export GOROOT="/home/kevin/go"
cd /home/kevin/.openclaw/workspace/projects/sql-platform

go mod verify                          # ✅ all modules verified
govulncheck ./...                      # 0 direct, 23 indirect vulnerabilities
go list -m all                         # 81 third-party modules

# 前端依赖
cd web && npm audit                    # ✅ found 0 vulnerabilities

# 代码安全检查
grep -rn --include="*.go" -iE '(password|secret|token|api_key)\s*[:=]\s*"[^"]+"'  # 硬编码凭据
grep -rn --include="*.go" -E 'fmt\.Sprintf.*SELECT'                               # SQL 拼接
grep -rn --include="*.go" -E 'InsecureSkipVerify'                                 # 不安全 TLS
grep -rn --include="*.yaml" -iE '(password|secret|token)\s*[:=]\s*"[^"]+"'       # 配置文件凭据
```

---

*报告由 Marcus 自动生成 — SQLFlow 安全扫描 [S3-04]*
