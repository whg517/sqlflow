package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// PermissionHandler handles permission related requests.
type PermissionHandler struct {
	permSvc *service.PermissionService
}

// NewPermissionHandler creates a new PermissionHandler.
func NewPermissionHandler(permSvc *service.PermissionService) *PermissionHandler {
	return &PermissionHandler{permSvc: permSvc}
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
func (h *PermissionHandler) ListRoles(c echo.Context) error {
	roles := h.permSvc.GetRoles()
	items := make([]map[string]string, 0, len(roles))
	for _, r := range roles {
		items = append(items, map[string]string{"name": r})
	}
	return resp.OK(c, items)
}

// GetRole handles GET /api/roles/:role (admin).
//
// @Summary 获取角色详情
// @Description 管理员获取指定角色的策略信息
// @Tags 权限
// @Produce json
// @Security BearerAuth
// @Param role path string true "角色名称"
// @Success 200 {object} resp.SuccessResponse{data=service.RoleInfo} "成功"
// @Failure 400 {object} resp.ErrorResponse "角色名称不能为空"
// @Failure 500 {object} resp.ErrorResponse "获取角色策略失败"
// @Router /roles/{role} [get]
func (h *PermissionHandler) GetRole(c echo.Context) error {
	role := c.Param("role")
	if role == "" {
		return resp.BadRequest(c, "角色名称不能为空")
	}

	policies, err := h.permSvc.GetPoliciesForRole(c.Request().Context(), role)
	if err != nil {
		return resp.InternalError(c, "获取角色策略失败")
	}

	return resp.OK(c, service.RoleInfo{
		Name:     role,
		Policies: policies,
	})
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
func (h *PermissionHandler) AddPolicy(c echo.Context) error {
	var req addPolicyRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.Sub == "" || req.Dom == "" || req.Obj == "" || req.Act == "" {
		return resp.BadRequest(c, "sub, dom, obj, act 均不能为空")
	}

	if err := h.permSvc.AddPolicy(req.Sub, req.Dom, req.Obj, req.Act); err != nil {
		if err.Error() == "策略已存在" {
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
func (h *PermissionHandler) ListPolicies(c echo.Context) error {
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
func (h *PermissionHandler) DeletePolicy(c echo.Context) error {
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
func (h *PermissionHandler) SyncPolicies(c echo.Context) error {
	if err := h.permSvc.LoadPolicy(); err != nil {
		return resp.InternalError(c, "同步策略失败")
	}
	return resp.OKWithMessage(c, "策略同步成功", nil)
}
