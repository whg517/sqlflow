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
func (h *PermissionHandler) ListRoles(c echo.Context) error {
	roles := h.permSvc.GetRoles()
	items := make([]map[string]string, 0, len(roles))
	for _, r := range roles {
		items = append(items, map[string]string{"name": r})
	}
	return resp.OK(c, items)
}

// GetRole handles GET /api/roles/:role (admin).
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
func (h *PermissionHandler) SyncPolicies(c echo.Context) error {
	if err := h.permSvc.LoadPolicy(); err != nil {
		return resp.InternalError(c, "同步策略失败")
	}
	return resp.OKWithMessage(c, "策略同步成功", nil)
}
