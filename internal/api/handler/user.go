package handler

import (
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
	Token string       `json:"token"`
	User  userResponse `json:"user"`
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

// Login handles POST /api/auth/login.
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

	token, user, err := h.authSvc.Authenticate(req.Username, req.Password)
	if err != nil {
		return resp.Unauthorized(c, "用户名或密码错误")
	}

	return resp.OK(c, loginResponse{
		Token: token,
		User: userResponse{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	})
}

// Me handles GET /api/auth/me.
func (h *UserHandler) Me(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)

	user, err := h.authSvc.GetUserByID(userID)
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
func (h *UserHandler) ChangePassword(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)

	var req changePasswordRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if err := validatePassword(req.NewPassword); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	if err := h.authSvc.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
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

	user, err := h.authSvc.CreateUser(req.Username, req.Password, req.Role)
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
func (h *UserHandler) ListUsers(c echo.Context) error {
	page, pageSize := parsePagination(c)

	users, total, err := h.authSvc.ListUsers(page, pageSize)
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
func (h *UserHandler) GetUser(c echo.Context) error {
	id, err := parseUserID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的用户ID")
	}

	user, err := h.authSvc.GetUserByID(id)
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
	targetUser, err := h.authSvc.GetUserByID(id)
	if err != nil {
		return resp.NotFound(c, "用户不存在")
	}
	if targetUser.Role == "admin" {
		return resp.Forbidden(c, "不能编辑管理员角色的信息")
	}

	user, err := h.authSvc.UpdateUserRole(id, req.Role)
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
	targetUser, err := h.authSvc.GetUserByID(id)
	if err != nil {
		return resp.NotFound(c, "用户不存在")
	}

	// Cannot delete last admin
	if targetUser.Role == "admin" {
		adminCount, err := h.authSvc.AdminCount()
		if err != nil {
			return resp.InternalError(c, "删除用户失败")
		}
		if adminCount <= 1 {
			return resp.Forbidden(c, "不能删除最后一个管理员")
		}
	}

	if err := h.authSvc.DeleteUser(id); err != nil {
		return resp.InternalError(c, "删除用户失败")
	}

	return resp.OKWithMessage(c, "删除成功", nil)
}

// ResetPassword handles PUT /api/users/:id/reset-password (admin only).
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

	if err := h.authSvc.ResetPassword(id, req.Password); err != nil {
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
