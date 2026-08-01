# User Stories：数据源管理

## US-DS-001：登记并测试数据源

**故事**：作为 Admin，我希望登记并测试目标数据源连接，以便团队只通过受管入口访问数据库。

**关联需求**：FR-DS-001、FR-DS-002

**验收标准**：

- Given 类型为 MySQL、PostgreSQL、MongoDB 或 Elasticsearch 且配置完整，When Admin 测试连接，Then 返回明确的成功或失败结果。
- Given 类型未注册或连接配置不合法，When 保存数据源，Then 请求被拒绝且不创建不可用记录。
- Given 数据源包含密码/API Key，When 持久化并再次读取，Then API 响应不包含明文或密文凭据。
- Given 非 Admin，When 调用数据源写入或删除 API，Then 操作被拒绝。

**代表性验证**：`internal/datasource/datasource_test.go`。

## US-DS-002：更新或禁用数据源

**故事**：作为 Admin，我希望更新连接参数或禁用数据源，以便处理凭据轮换、扩容和下线。

**关联需求**：FR-DS-001、FR-DS-004

**验收标准**：

- Given 数据源存在，When 更新非敏感配置且密码留空，Then 保留原加密凭据。
- Given 连接配置发生变化，When 更新完成，Then 旧缓存连接被释放，下一次请求使用新配置连接。
- Given 数据源被禁用，When 用户尝试查询或执行工单，Then 请求被拒绝。

## US-DS-003：浏览数据源元数据

**故事**：作为已认证用户，我希望浏览被授权数据源的表、列、索引和字段，以便正确编写查询或变更。

**关联需求**：FR-DS-003

**验收标准**：

- Given 驱动声明元数据能力，When 请求表/列或索引/字段，Then 返回对应目标数据源的当前元数据。
- Given 驱动不支持所请求能力，When 请求元数据，Then 返回明确的不支持错误，而不是空成功伪装。
- Given 用户未认证，When 直接访问元数据 API，Then 请求被拒绝。

**代表性验证**：`internal/datasource/datasource_test.go`、`internal/datasource/datasource_test.go`。

