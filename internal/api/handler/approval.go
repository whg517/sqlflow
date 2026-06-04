package handler

import (
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/service"
)

// ApprovalHandler handles approval policy and action endpoints.
type ApprovalHandler struct {
	engine *service.ApprovalEngine
}

// NewApprovalHandler creates a new ApprovalHandler.
func NewApprovalHandler(engine *service.ApprovalEngine) *ApprovalHandler {
	return &ApprovalHandler{engine: engine}
}

type createPolicyRequest struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	Conditions         string `json:"conditions"`
	ApprovalChain      string `json:"approval_chain"`
	AutoApproveEnabled bool   `json:"auto_approve_enabled"`
	AutoApproveReason  string `json:"auto_approve_reason"`
	IsDefault          bool   `json:"is_default"`
	Priority           int    `json:"priority"`
}

// CreatePolicy handles POST /api/approval/policies.
func (h *ApprovalHandler) CreatePolicy(c echo.Context) error {
	var req createPolicyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "策略名称不能为空"})
	}

	policy, err := h.engine.CreatePolicy(
		c.Request().Context(), req.Name, req.Description,
		req.Conditions, req.ApprovalChain,
		req.AutoApproveEnabled, req.AutoApproveReason,
		req.IsDefault, req.Priority,
	)
	if err != nil {
		log.Printf("CreatePolicy failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, policy)
}

type updatePolicyRequest struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	Enabled            bool   `json:"enabled"`
	Priority           int    `json:"priority"`
	Conditions         string `json:"conditions"`
	ApprovalChain      string `json:"approval_chain"`
	AutoApproveEnabled bool   `json:"auto_approve_enabled"`
	AutoApproveReason  string `json:"auto_approve_reason"`
	IsDefault          bool   `json:"is_default"`
}

// UpdatePolicy handles PUT /api/approval/policies/:id.
func (h *ApprovalHandler) UpdatePolicy(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的策略ID"})
	}

	var req updatePolicyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
	}

	policy, err := h.engine.UpdatePolicy(
		c.Request().Context(), id,
		req.Name, req.Description, req.Enabled, req.Priority,
		req.Conditions, req.ApprovalChain,
		req.AutoApproveEnabled, req.AutoApproveReason, req.IsDefault,
	)
	if err != nil {
		log.Printf("UpdatePolicy failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, policy)
}

// DeletePolicy handles DELETE /api/approval/policies/:id.
func (h *ApprovalHandler) DeletePolicy(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的策略ID"})
	}

	if err := h.engine.DeletePolicy(c.Request().Context(), id); err != nil {
		if ent.IsNotFound(err) {
			return c.JSON(http.StatusOK, map[string]string{"message": "删除成功"})
		}
		log.Printf("DeletePolicy failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "删除成功"})
}

// ListPolicies handles GET /api/approval/policies.
func (h *ApprovalHandler) ListPolicies(c echo.Context) error {
	policies, err := h.engine.ListPolicies(c.Request().Context())
	if err != nil {
		log.Printf("ListPolicies failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if policies == nil {
		policies = []model.ApprovalPolicy{}
	}
	return c.JSON(http.StatusOK, policies)
}

// GetPolicy handles GET /api/approval/policies/:id.
func (h *ApprovalHandler) GetPolicy(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的策略ID"})
	}

	policy, err := h.engine.GetPolicy(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, policy)
}

type processApprovalRequest struct {
	Action  string `json:"action"`  // approved, rejected
	Comment string `json:"comment"`
}

// ProcessApproval handles POST /api/tickets/:id/approve (using engine).
func (h *ApprovalHandler) ProcessApproval(c echo.Context) error {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的工单ID"})
	}

	var req processApprovalRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
	}
	if req.Action != "approved" && req.Action != "rejected" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "action 必须是 approved 或 rejected"})
	}

	userID := getContextUserID(c)
	userRole := getContextRole(c)

	record, err := h.engine.ProcessApproval(c.Request().Context(), ticketID, userID, userRole, req.Action, req.Comment)
	if err != nil {
		log.Printf("ProcessApproval failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, record)
}

// GetApprovalHistory handles GET /api/tickets/:id/approval-history.
func (h *ApprovalHandler) GetApprovalHistory(c echo.Context) error {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的工单ID"})
	}

	records, err := h.engine.GetApprovalHistory(c.Request().Context(), ticketID)
	if err != nil {
		log.Printf("GetApprovalHistory failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if records == nil {
		records = []model.ApprovalRecord{}
	}
	return c.JSON(http.StatusOK, records)
}

// getContextUserID and getContextUserRole are defined in context.go
