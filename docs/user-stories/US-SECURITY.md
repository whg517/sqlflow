# User Stories：权限与数据保护

## US-SEC-001：管理细粒度数据权限

**故事**：作为 Admin，我希望按角色、数据源、对象和动作配置策略，以便遵循最小权限原则。

**关联需求**：FR-SEC-001、FR-SEC-002

**验收标准**：

- Given Admin 新增或删除有效策略，When 策略同步完成，Then 后续请求使用新授权结果。
- Given Developer 仅拥有默认 `select`，When 尝试 update/delete/ddl，Then 操作被拒绝并被引导至工单。
- Given 非 Admin，When 调用策略管理 API，Then 操作被拒绝。
- Then 通配符策略的域、对象和动作匹配必须符合 Casbin 模型，不因字符串近似而越权。

**代表性验证**：`internal/service/permission_test.go`、`e2e/tests/rbac-policies.spec.ts`、`e2e/tests/permission-isolation.spec.ts`。

## US-SEC-002：申请临时权限

**故事**：作为 Developer，我希望为特定数据源、表和期限申请临时权限，以便处理有明确原因的例外任务。

**关联需求**：FR-SEC-003

**验收标准**：

- Given 申请包含目标、动作、原因和未来到期时间，When 提交，Then 状态为 `PENDING` 且申请人可查看。
- Given Admin 批准申请，Then 创建带到期时间的临时策略并将申请标记为 `APPROVED`。
- Given 申请被拒绝、撤销或过期，Then 对应临时策略不再授权。
- Given 用户查看申请，Then 普通用户只能看到自己的申请，Admin 可查看待处理队列。

**代表性验证**：`internal/service/permission_request_test.go`、`e2e/tests/permission-requests.spec.ts`。

## US-SEC-003：配置字段脱敏与敏感表

**故事**：作为 Admin，我希望标记敏感表并配置字段脱敏规则，以便用户查询和导出时默认减少敏感数据暴露。

**关联需求**：FR-SEC-004

**验收标准**：

- Given 规则匹配数据源、库、表和字段，When 用户查询结果包含该字段，Then 返回值按规则脱敏。
- Given 用户缺少 `unmask`，When 查询、导出或分享该数据，Then 不能通过更换入口绕过脱敏。
- Given 用户具有显式豁免，When 返回未脱敏结果，Then 豁免行为可审计。
- Given 规则包含自定义正则/模板，When 配置无效，Then 保存被拒绝而不是在查询时静默失效。

**代表性验证**：`internal/service/mask_rule_test.go`、`e2e/tests/mask-rules*.spec.ts`、`e2e/tests/sensitive-tables.spec.ts`。

## US-SEC-004：阻止跨用户与跨角色访问

**故事**：作为平台负责人，我希望所有资源在服务端检查角色和所有权，以便直接调用 API 也不能访问他人的私有资源。

**关联需求**：FR-SEC-005、NFR-SEC-002

**验收标准**：

- Given 普通用户，When 请求他人的历史、Token、私有模板、导出文件或权限申请，Then 不返回资源内容。
- Given 非审批角色，When 直接调用批准、拒绝或执行端点，Then 工单状态不改变。
- Given 未认证请求，When 访问受保护路由，Then 返回认证错误而不是业务数据。

**代表性验证**：`e2e/tests/auth-boundary.spec.ts`、`e2e/tests/ticket-audit-boundary.spec.ts`、`e2e/tests/approval-permission-ui.spec.ts`。

## US-SEC-005：按权限发现数据源对象与字段

**故事**：作为数据使用者，我只希望看到自己有权使用的表、集合、索引和字段，避免元数据泄露和无效操作。

**关联需求**：FR-SEC-006、FR-SEC-007

**验收标准**：

- Given 用户只拥有 `orders` 的 `select` 或 `metadata:view`，When 获取数据源表列表，Then 列表和 SQL 自动补全只包含 `orders`。
- Given 用户无权发现 `payroll`，When 直接访问其字段接口，Then 返回 403，不返回字段名、类型或注释。
- Given Admin 配置字段脱敏规则，When 选择目标字段，Then 候选项来自服务端授权后的真实字段元数据。
- Given 用户拥有 `select` 但没有 `unmask`，When 查询命中脱敏规则的字段，Then 数据可查询但返回值保持脱敏。

**代表性验证**：`internal/api/handler/datasource_test.go`、`internal/service/query_test.go`、`e2e/tests/permission-isolation.spec.ts`。
