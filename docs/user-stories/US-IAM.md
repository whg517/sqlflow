# User Stories：身份与访问

## US-IAM-001：使用本地账号登录

**故事**：作为平台用户，我希望使用账号密码登录并保持可续期会话，以便安全访问被授权能力。

**关联需求**：FR-IAM-001、NFR-SEC-002

**验收标准**：

- Given 用户存在且状态有效，When 提交正确用户名和密码，Then 返回 Access Token、Refresh Token 和用户角色。
- Given 凭据错误，When 尝试登录，Then 请求被拒绝且响应不说明用户名或密码哪一项错误。
- Given Access Token 过期且 Refresh Token 有效，When 刷新会话，Then 签发新令牌并执行令牌轮换。
- Given Refresh Token 已撤销或过期，When 再次刷新，Then 请求被拒绝。

**代表性验证**：`internal/service/auth_test.go`、`internal/service/refresh_token_test.go`、`e2e/tests/auth.spec.ts`。

## US-IAM-002：修改个人密码

**故事**：作为已登录用户，我希望验证旧密码后设置新密码，以便在凭据暴露或定期轮换时保护账号。

**关联需求**：FR-IAM-002

**验收标准**：

- Given 用户已认证，When 提供正确旧密码和符合规则的新密码，Then 密码更新成功。
- Given 旧密码错误或新密码不符合规则，When 提交修改，Then 密码保持不变并返回可解释错误。
- Then 任何响应、日志和审计对象都不包含明文密码或密码哈希。

## US-IAM-003：管理平台用户

**故事**：作为 Admin，我希望创建用户、调整角色、重置密码和停用不再需要的账号，以便维护最小权限的用户目录。

**关联需求**：FR-IAM-003、FR-SEC-005

**验收标准**：

- Given 当前用户为 Admin，When 创建用户名唯一且角色有效的用户，Then 用户可使用新凭据登录。
- Given 当前用户不是 Admin，When 直接调用用户管理 API，Then 返回未授权/禁止访问。
- Given 目标是系统中唯一 Admin，When 尝试删除或降级该账号，Then 操作被拒绝。
- Given Admin 重置密码，Then 只返回操作结果，不返回服务端存储的密码材料。

**代表性验证**：`internal/service/auth_test.go`、`e2e/tests/admin-user-crud.spec.ts`、`e2e/tests/user-management*.spec.ts`。

## US-IAM-004：管理个人 API Token

**故事**：作为需要自动化访问的用户，我希望创建带 Scope 和有效期的 API Token，以便在不共享登录密码的情况下调用 SQLFlow。

**关联需求**：FR-IAM-004

**验收标准**：

- Given 用户已认证，When 创建名称、Scope 和有效期合法的 Token，Then 明文 Token 只在本次响应出现一次。
- Given Token 过期、撤销或缺少所需 Scope，When 调用受保护 API，Then 请求被拒绝。
- Given 普通用户查看 Token 列表，Then 只能看到自己的 Token 元数据和前缀。
- Given Admin 撤销任意 Token，Then 该 Token 后续请求立即失效。

**代表性验证**：`internal/service/token_test.go`、`e2e/tests/admin-tokens.spec.ts`。

## US-IAM-005：通过 OIDC 登录

**故事**：作为组织用户，我希望通过已配置的身份提供商登录，以便复用组织身份和单点登录体验。

**关联需求**：FR-IAM-005

**验收标准**：

- Given Provider 已启用，When 用户发起 OIDC 登录并完成有效回调，Then 映射或创建本地用户并建立 SQLFlow 会话。
- Given Provider 未启用、state 不匹配或回调失败，When 返回系统，Then 不创建已认证会话。
- Given 首次登录用户未配置特权映射，Then 默认角色为 Developer。

**代表性验证**：`internal/service/oidc_test.go`、`internal/api/handler/oidc_test.go`。

