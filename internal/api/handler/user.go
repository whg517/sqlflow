package handler

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"unicode"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// UserHandler handles user/auth related requests.
type UserHandler struct {
	authSvc *service.AuthService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(authSvc *service.AuthService) *UserHandler {
	return &UserHandler{authSvc: authSvc}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int64        `json:"expires_in"`
	User         userResponse `json:"user"`
}

type userResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type userDetailResponse struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Login handles POST /api/auth/login.
//
// @Summary 用户登录
// @Description 使用用户名和密码登录，返回 JWT token
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body loginRequest true "登录信息"
// @Success 200 {object} resp.SuccessResponse{data=loginResponse} "登录成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 401 {object} resp.ErrorResponse "用户名或密码错误"
// @Router /auth/login [post]
func (h *UserHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if err := validateUsername(req.Username); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	if err := validatePassword(req.Password); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	token, refreshToken, user, err := h.authSvc.Authenticate(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		return resp.Unauthorized(c, "用户名或密码错误")
	}

	return resp.OK(c, loginResponse{
		AccessToken:  token,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.authSvc.JWTExpiry().Seconds()),
		User: userResponse{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	})
}

// Refresh handles POST /api/auth/refresh.
// It accepts a refresh_token and returns a new access_token + rotated refresh_token.
//
// @Summary 刷新Token
// @Description 使用 refresh_token 获取新的 access_token
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body refreshRequest true "刷新Token请求"
// @Success 200 {object} resp.SuccessResponse{data=loginResponse} "刷新成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 401 {object} resp.ErrorResponse "refresh_token 无效或已过期"
// @Router /auth/refresh [post]
func (h *UserHandler) Refresh(c echo.Context) error {
	var req refreshRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}
	if req.RefreshToken == "" {
		return resp.BadRequest(c, "refresh_token 不能为空")
	}

	// Rotate the refresh token (old one is revoked, new one issued)
	newRefreshToken, userID, err := h.authSvc.RefreshTokenService().RotateToken(c.Request().Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, service.ErrRefreshTokenInvalid) || errors.Is(err, service.ErrRefreshTokenRevoked) {
			return resp.Unauthorized(c, "refresh_token 无效或已过期，请重新登录")
		}
		return resp.InternalError(c, "刷新 token 失败")
	}

	// Look up the user to generate a new access token
	user, err := h.authSvc.GetUserByID(c.Request().Context(), userID)
	if err != nil {
		return resp.Unauthorized(c, "用户不存在")
	}

	// Generate a new access token
	accessToken, err := h.authSvc.GenerateAccessToken(user)
	if err != nil {
		return resp.InternalError(c, "生成 token 失败")
	}

	return resp.OK(c, loginResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.authSvc.JWTExpiry().Seconds()),
		User: userResponse{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	})
}

// Me handles GET /api/auth/me.
//
// @Summary 获取当前用户信息
// @Description 获取当前登录用户的详细信息
// @Tags 认证
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse{data=userDetailResponse} "成功"
// @Failure 401 {object} resp.ErrorResponse "未认证"
// @Router /auth/me [get]
func (h *UserHandler) Me(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)

	user, err := h.authSvc.GetUserByID(c.Request().Context(), userID)
	if err != nil {
		return resp.Unauthorized(c, "用户不存在")
	}

	return resp.OK(c, userDetailResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// ChangePassword handles PUT /api/auth/password.
//
// @Summary 修改密码
// @Description 修改当前用户密码
// @Tags 认证
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body changePasswordRequest true "修改密码请求"
// @Success 200 {object} resp.SuccessResponse "密码修改成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 401 {object} resp.ErrorResponse "旧密码错误"
// @Router /auth/password [put]
func (h *UserHandler) ChangePassword(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)

	var req changePasswordRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if err := validatePassword(req.NewPassword); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	if err := h.authSvc.ChangePassword(c.Request().Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		if err == service.ErrInvalidCredentials {
			return resp.Unauthorized(c, "旧密码错误")
		}
		return resp.InternalError(c, "修改密码失败")
	}

	return resp.OKWithMessage(c, "密码修改成功", nil)
}

// validateUsername checks username is 3-32 characters, alphanumeric + underscore.
func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 {
		return errUsernameLength
	}
	if !usernameRegex.MatchString(username) {
		return errUsernameFormat
	}
	return nil
}

// validatePassword checks password is 8-128 chars with at least one letter and one digit.
func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 128 {
		return errPasswordLength
	}
	var hasLetter, hasDigit bool
	for _, ch := range password {
		if unicode.IsLetter(ch) {
			hasLetter = true
		}
		if unicode.IsDigit(ch) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errPasswordComplexity
	}
	return nil
}

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

var (
	errUsernameLength     = fmt.Errorf("用户名长度需要3-32个字符")
	errUsernameFormat     = fmt.Errorf("用户名只能包含字母、数字和下划线")
	errPasswordLength     = fmt.Errorf("密码长度需要8-128个字符")
	errPasswordComplexity = fmt.Errorf("密码必须包含至少一个字母和一个数字")
)

// --- Admin CRUD handlers ---

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type updateUserRequest struct {
	Role string `json:"role"`
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

type userListResponse struct {
	Users []userDetailResponse `json:"users"`
	Total int64                `json:"total"`
}

// CreateUser handles POST /api/users (admin only).
//
// @Summary 创建用户
// @Description 管理员创建新用户
// @Tags 用户管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createUserRequest true "创建用户请求"
// @Success 201 {object} resp.SuccessResponse{data=userDetailResponse} "创建成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 500 {object} resp.ErrorResponse "创建用户失败"
// @Router /users [post]
func (h *UserHandler) CreateUser(c echo.Context) error {
	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if err := validateUsername(req.Username); err != nil {
		return resp.BadRequest(c, err.Error())
	}
	if err := validatePassword(req.Password); err != nil {
		return resp.BadRequest(c, err.Error())
	}
	if !service.ValidRoles[req.Role] {
		return resp.BadRequest(c, "角色必须是 admin、dba 或 developer")
	}

	user, err := h.authSvc.CreateUser(c.Request().Context(), req.Username, req.Password, req.Role)
	if err != nil {
		return resp.InternalError(c, "创建用户失败")
	}

	return resp.Created(c, userDetailResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// ListUsers handles GET /api/users (admin only).
//
// @Summary 获取用户列表
// @Description 管理员获取所有用户列表
// @Tags 用户管理
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} resp.SuccessResponse{data=userListResponse} "成功"
// @Failure 500 {object} resp.ErrorResponse "获取用户列表失败"
// @Router /users [get]
func (h *UserHandler) ListUsers(c echo.Context) error {
	page, pageSize := parsePagination(c)

	users, total, err := h.authSvc.ListUsers(c.Request().Context(), page, pageSize)
	if err != nil {
		return resp.InternalError(c, "获取用户列表失败")
	}

	items := make([]userDetailResponse, 0, len(users))
	for _, u := range users {
		items = append(items, userDetailResponse{
			ID:        u.ID,
			Username:  u.Username,
			Role:      u.Role,
			CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return resp.OK(c, userListResponse{Users: items, Total: total})
}

// GetUser handles GET /api/users/:id (admin only).
//
// @Summary 获取用户详情
// @Description 管理员获取指定用户详细信息
// @Tags 用户管理
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} resp.SuccessResponse{data=userDetailResponse} "成功"
// @Failure 400 {object} resp.ErrorResponse "无效的用户ID"
// @Failure 404 {object} resp.ErrorResponse "用户不存在"
// @Router /users/{id} [get]
func (h *UserHandler) GetUser(c echo.Context) error {
	id, err := parseUserID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的用户ID")
	}

	user, err := h.authSvc.GetUserByID(c.Request().Context(), id)
	if err != nil {
		return resp.NotFound(c, "用户不存在")
	}

	return resp.OK(c, userDetailResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// UpdateUser handles PUT /api/users/:id (admin only).
//
// @Summary 更新用户角色
// @Description 管理员更新指定用户的角色
// @Tags 用户管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Param body body updateUserRequest true "更新用户请求"
// @Success 200 {object} resp.SuccessResponse{data=userDetailResponse} "更新成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 403 {object} resp.ErrorResponse "不能编辑自己的角色"
// @Failure 404 {object} resp.ErrorResponse "用户不存在"
// @Router /users/{id} [put]
func (h *UserHandler) UpdateUser(c echo.Context) error {
	currentUserID := c.Get(middleware.ContextKeyUserID).(int64)

	id, err := parseUserID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的用户ID")
	}

	// Cannot edit self
	if id == currentUserID {
		return resp.Forbidden(c, "不能编辑自己的角色")
	}

	var req updateUserRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}
	if !service.ValidRoles[req.Role] {
		return resp.BadRequest(c, "角色必须是 admin、dba 或 developer")
	}

	// Check target user is not admin
	targetUser, err := h.authSvc.GetUserByID(c.Request().Context(), id)
	if err != nil {
		return resp.NotFound(c, "用户不存在")
	}
	if targetUser.Role == "admin" {
		return resp.Forbidden(c, "不能编辑管理员角色的信息")
	}

	user, err := h.authSvc.UpdateUserRole(c.Request().Context(), id, req.Role)
	if err != nil {
		return resp.InternalError(c, "更新用户失败")
	}

	return resp.OK(c, userDetailResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// DeleteUser handles DELETE /api/users/:id (admin only).
//
// @Summary 删除用户
// @Description 管理员删除指定用户
// @Tags 用户管理
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} resp.SuccessResponse "删除成功"
// @Failure 400 {object} resp.ErrorResponse "无效的用户ID"
// @Failure 403 {object} resp.ErrorResponse "不能删除自己或最后一个管理员"
// @Failure 404 {object} resp.ErrorResponse "用户不存在"
// @Router /users/{id} [delete]
func (h *UserHandler) DeleteUser(c echo.Context) error {
	currentUserID := c.Get(middleware.ContextKeyUserID).(int64)

	id, err := parseUserID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的用户ID")
	}

	// Cannot delete self
	if id == currentUserID {
		return resp.Forbidden(c, "不能删除自己")
	}

	// Check target user exists
	targetUser, err := h.authSvc.GetUserByID(c.Request().Context(), id)
	if err != nil {
		return resp.NotFound(c, "用户不存在")
	}

	// Cannot delete last admin
	if targetUser.Role == "admin" {
		adminCount, err := h.authSvc.AdminCount(c.Request().Context())
		if err != nil {
			return resp.InternalError(c, "删除用户失败")
		}
		if adminCount <= 1 {
			return resp.Forbidden(c, "不能删除最后一个管理员")
		}
	}

	if err := h.authSvc.DeleteUser(c.Request().Context(), id); err != nil {
		return resp.InternalError(c, "删除用户失败")
	}

	return resp.OKWithMessage(c, "删除成功", nil)
}

// ResetPassword handles PUT /api/users/:id/reset-password (admin only).
//
// @Summary 重置用户密码
// @Description 管理员重置指定用户密码
// @Tags 用户管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Param body body resetPasswordRequest true "重置密码请求"
// @Success 200 {object} resp.SuccessResponse "密码重置成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 404 {object} resp.ErrorResponse "用户不存在"
// @Router /users/{id}/reset-password [put]
func (h *UserHandler) ResetPassword(c echo.Context) error {
	id, err := parseUserID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的用户ID")
	}

	var req resetPasswordRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if err := validatePassword(req.Password); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	if err := h.authSvc.ResetPassword(c.Request().Context(), id, req.Password); err != nil {
		if err == service.ErrUserNotFound {
			return resp.NotFound(c, "用户不存在")
		}
		return resp.InternalError(c, "重置密码失败")
	}

	return resp.OKWithMessage(c, "密码重置成功", nil)
}

// parseUserID extracts and validates the :id path parameter.
func parseUserID(c echo.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

// parsePagination extracts page and page_size query params with defaults.
func parsePagination(c echo.Context) (int64, int64) {
	page := int64(1)
	pageSize := int64(20)

	if v := c.QueryParam("page"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			page = n
		}
	}
	if v := c.QueryParam("page_size"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 && n <= 100 {
			pageSize = n
		}
	}
	return page, pageSize
}
