package security

import (
	"errors"
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/resp"
)

// Handler handles permission related requests.
type Handler struct {
	permSvc *Service
}

// NewHandler creates a new Handler.
func NewHandler(permSvc *Service) *Handler {
	return &Handler{permSvc: permSvc}
}

// ListRoles handles GET /api/roles (admin).
//
// @Summary 获取角色列表
// @Description 管理员获取所有角色列表
// @Tags 权限
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /roles [get]
func (h *Handler) ListRoles(c echo.Context) error {
	roles, err := h.permSvc.ListRoles(c.Request().Context())
	if err != nil {
		return resp.InternalError(c, "获取角色列表失败")
	}
	return resp.OK(c, roles)
}

// GetRole handles GET /api/roles/:role (admin).
//
// @Summary 获取角色详情
// @Description 管理员获取指定角色的策略信息
// @Tags 权限
// @Produce json
// @Security BearerAuth
// @Param role path string true "角色名称"
// @Success 200 {object} resp.SuccessResponse{data=RoleInfo} "成功"
// @Failure 400 {object} resp.ErrorResponse "角色名称不能为空"
// @Failure 500 {object} resp.ErrorResponse "获取角色策略失败"
// @Router /roles/{role} [get]
func (h *Handler) GetRole(c echo.Context) error {
	role := c.Param("role")
	if role == "" {
		return resp.BadRequest(c, "角色名称不能为空")
	}

	roleInfo, err := h.permSvc.GetRole(c.Request().Context(), role)
	if errors.Is(err, ErrRoleNotFound) {
		return resp.NotFound(c, "角色不存在")
	}
	if err != nil {
		return resp.InternalError(c, "获取角色失败")
	}
	policies, err := h.permSvc.GetPoliciesForRole(c.Request().Context(), role)
	if err != nil {
		return resp.InternalError(c, "获取角色策略失败")
	}

	return resp.OK(c, RoleInfo{
		ID:          roleInfo.ID,
		Name:        roleInfo.Name,
		DisplayName: roleInfo.DisplayName,
		Description: roleInfo.Description,
		IsBuiltin:   roleInfo.IsBuiltin,
		Status:      roleInfo.Status,
		UserCount:   roleInfo.UserCount,
		PolicyCount: roleInfo.PolicyCount,
		Permissions: roleInfo.Permissions,
		CreatedAt:   roleInfo.CreatedAt,
		UpdatedAt:   roleInfo.UpdatedAt,
		Policies:    policies,
	})
}

type createRoleRequest struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type updateRoleRequest struct {
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Permissions []string `json:"permissions"`
}

// CreateRole handles POST /api/roles.
func (h *Handler) CreateRole(c echo.Context) error {
	var req createRoleRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}
	role, err := h.permSvc.CreateRole(c.Request().Context(), req.Name, req.DisplayName, req.Description)
	switch {
	case errors.Is(err, ErrInvalidRoleName), errors.Is(err, ErrRoleExists):
		return resp.BadRequest(c, err.Error())
	case err != nil:
		return resp.InternalError(c, "创建角色失败")
	default:
		if err := h.permSvc.SetPlatformPermissions(c.Request().Context(), role.Name, req.Permissions); err != nil {
			_ = h.permSvc.DeleteRole(c.Request().Context(), role.Name)
			return resp.BadRequest(c, err.Error())
		}
		role, _ = h.permSvc.GetRole(c.Request().Context(), role.Name)
		return resp.Created(c, role)
	}
}

// UpdateRole handles PUT /api/roles/:role.
func (h *Handler) UpdateRole(c echo.Context) error {
	name := c.Param("role")
	if name == "" {
		return resp.BadRequest(c, "角色名称不能为空")
	}
	var req updateRoleRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}
	role, err := h.permSvc.UpdateRole(c.Request().Context(), name, req.DisplayName, req.Description, req.Status)
	switch {
	case errors.Is(err, ErrRoleNotFound):
		return resp.NotFound(c, "角色不存在")
	case errors.Is(err, ErrBuiltinRole), errors.Is(err, ErrRoleInUse):
		return resp.BadRequest(c, err.Error())
	case err != nil:
		return resp.BadRequest(c, err.Error())
	default:
		if !role.IsBuiltin {
			if err := h.permSvc.SetPlatformPermissions(c.Request().Context(), role.Name, req.Permissions); err != nil {
				return resp.BadRequest(c, err.Error())
			}
			role, _ = h.permSvc.GetRole(c.Request().Context(), role.Name)
		}
		return resp.OK(c, role)
	}
}

// DeleteRole handles DELETE /api/roles/:role.
func (h *Handler) DeleteRole(c echo.Context) error {
	name := c.Param("role")
	if name == "" {
		return resp.BadRequest(c, "角色名称不能为空")
	}
	err := h.permSvc.DeleteRole(c.Request().Context(), name)
	switch {
	case errors.Is(err, ErrRoleNotFound):
		return resp.NotFound(c, "角色不存在")
	case errors.Is(err, ErrBuiltinRole), errors.Is(err, ErrRoleInUse):
		return resp.BadRequest(c, err.Error())
	case err != nil:
		return resp.InternalError(c, "删除角色失败")
	default:
		return resp.OKWithMessage(c, "角色已删除", nil)
	}
}

type addPolicyRequest struct {
	Sub string `json:"sub"`
	Dom string `json:"dom"`
	Obj string `json:"obj"`
	Act string `json:"act"`
}

// AddPolicy handles POST /api/policies (admin).
//
// @Summary 添加策略
// @Description 管理员添加权限策略
// @Tags 权限
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body addPolicyRequest true "策略请求"
// @Success 201 {object} resp.SuccessResponse "策略添加成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误或策略已存在"
// @Failure 500 {object} resp.ErrorResponse "添加策略失败"
// @Router /policies [post]
func (h *Handler) AddPolicy(c echo.Context) error {
	var req addPolicyRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.Sub == "" || req.Dom == "" || req.Obj == "" || req.Act == "" {
		return resp.BadRequest(c, "sub, dom, obj, act 均不能为空")
	}

	if err := h.permSvc.AddPolicy(req.Sub, req.Dom, req.Obj, req.Act); err != nil {
		if err.Error() == "策略已存在" || errors.Is(err, authz.ErrInvalidTuple) {
			return resp.BadRequest(c, err.Error())
		}
		log.Printf("AddPolicy failed: %v", err)
		return resp.InternalError(c, "添加策略失败")
	}

	return resp.Created(c, map[string]string{"message": "策略添加成功"})
}

// ListPolicies handles GET /api/policies (admin, with pagination and filtering).
//
// @Summary 获取策略列表
// @Description 管理员获取权限策略列表
// @Tags 权限
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(50)
// @Param ptype query string false "策略类型"
// @Param sub query string false "主体"
// @Success 200 {object} resp.PageResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "获取策略列表失败"
// @Router /policies [get]
func (h *Handler) ListPolicies(c echo.Context) error {
	page, _ := strconv.ParseInt(c.QueryParam("page"), 10, 64)
	pageSize, _ := strconv.ParseInt(c.QueryParam("page_size"), 10, 64)
	ptype := c.QueryParam("ptype")
	sub := c.QueryParam("sub")

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}

	policies, total, err := h.permSvc.GetPolicies(c.Request().Context(), page, pageSize, ptype, sub)
	if err != nil {
		return resp.InternalError(c, "获取策略列表失败")
	}

	return resp.OKPage(c, policies, page, pageSize, total)
}

// DeletePolicy handles DELETE /api/policies/:id (admin).
//
// @Summary 删除策略
// @Description 管理员删除指定权限策略
// @Tags 权限
// @Produce json
// @Security BearerAuth
// @Param id path int true "策略ID"
// @Success 200 {object} resp.SuccessResponse "策略已删除"
// @Failure 400 {object} resp.ErrorResponse "无效的策略ID"
// @Failure 404 {object} resp.ErrorResponse "策略不存在"
// @Failure 500 {object} resp.ErrorResponse "删除策略失败"
// @Router /policies/{id} [delete]
func (h *Handler) DeletePolicy(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的策略ID")
	}

	if err := h.permSvc.RemovePolicy(c.Request().Context(), id); err != nil {
		if err.Error() == "策略不存在" {
			return resp.NotFound(c, err.Error())
		}
		log.Printf("RemovePolicy failed: %v", err)
		return resp.InternalError(c, "删除策略失败")
	}

	return resp.OKWithMessage(c, "策略已删除", nil)
}

// SyncPolicies handles POST /api/policies/sync (admin).
//
// @Summary 同步策略
// @Description 管理员重新加载权限策略
// @Tags 权限
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "策略同步成功"
// @Failure 500 {object} resp.ErrorResponse "同步策略失败"
// @Router /policies/sync [post]
func (h *Handler) SyncPolicies(c echo.Context) error {
	if err := h.permSvc.LoadPolicy(); err != nil {
		return resp.InternalError(c, "同步策略失败")
	}
	return resp.OKWithMessage(c, "策略同步成功", nil)
}
