package handler

import (
	"encoding/json"
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
	engine   *service.ApprovalEngine
	auditSvc *service.AuditService
}

// NewApprovalHandler creates a new ApprovalHandler.
func NewApprovalHandler(engine *service.ApprovalEngine) *ApprovalHandler {
	return &ApprovalHandler{engine: engine}
}

// SetAuditService injects the audit service for audit logging.
func (h *ApprovalHandler) SetAuditService(auditSvc *service.AuditService) {
	h.auditSvc = auditSvc
}

// writeAuditLog writes an audit entry for policy management actions.
func (h *ApprovalHandler) writeAuditLog(c echo.Context, action, detail string) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(c.Request().Context(), service.AuditRecord{
		UserID:     getContextUserID(c),
		Action:     action,
		SQLSummary: detail,
	})
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

// CreatePolicy handles POST /api/admin/approval-policies.
func (h *ApprovalHandler) CreatePolicy(c echo.Context) error {
	var req createPolicyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "策略名称不能为空"})
	}

	// Validate conditions JSON
	if err := service.ValidateConditions(req.Conditions); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Validate approval chain JSON
	if err := validateApprovalChainJSON(req.ApprovalChain); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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

	h.writeAuditLog(c, "approval_policy_create", "创建审批策略: "+req.Name)
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

// UpdatePolicy handles PUT /api/admin/approval-policies/:id.
func (h *ApprovalHandler) UpdatePolicy(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的策略ID"})
	}

	var req updatePolicyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
	}

	// Validate conditions JSON
	if err := service.ValidateConditions(req.Conditions); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Validate approval chain JSON
	if err := validateApprovalChainJSON(req.ApprovalChain); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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

	h.writeAuditLog(c, "approval_policy_update", "更新审批策略: "+req.Name)
	return c.JSON(http.StatusOK, policy)
}

// DeletePolicy handles DELETE /api/admin/approval-policies/:id.
func (h *ApprovalHandler) DeletePolicy(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的策略ID"})
	}

	// Get policy name before deletion for audit
	policy, _ := h.engine.GetPolicy(c.Request().Context(), id)
	policyName := ""
	if policy != nil {
		policyName = policy.Name
	}

	if err := h.engine.DeletePolicy(c.Request().Context(), id); err != nil {
		if ent.IsNotFound(err) {
			return c.JSON(http.StatusOK, map[string]string{"message": "删除成功"})
		}
		log.Printf("DeletePolicy failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	h.writeAuditLog(c, "approval_policy_delete", "删除审批策略: "+policyName)
	return c.JSON(http.StatusOK, map[string]string{"message": "删除成功"})
}

// ListPolicies handles GET /api/admin/approval-policies with filtering and pagination.
func (h *ApprovalHandler) ListPolicies(c echo.Context) error {
	// Check if any query params are present for the enhanced endpoint
	enabled := c.QueryParam("enabled")
	sort := c.QueryParam("sort")
	pageStr := c.QueryParam("page")
	pageSizeStr := c.QueryParam("page_size")

	// If no pagination/filter params, use simple list (backward compatibility)
	if pageStr == "" && pageSizeStr == "" && enabled == "" && sort == "" {
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

	// Enhanced endpoint with pagination and filtering
	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)

	policies, total, err := h.engine.ListPoliciesFiltered(c.Request().Context(), enabled, sort, page, pageSize)
	if err != nil {
		log.Printf("ListPoliciesFiltered failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"items": policies,
		"total": total,
		"page":  page,
	})
}

// GetPolicy handles GET /api/admin/approval-policies/:id.
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

// TogglePolicy handles PUT /api/admin/approval-policies/:id/toggle.
func (h *ApprovalHandler) TogglePolicy(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的策略ID"})
	}

	policy, err := h.engine.TogglePolicy(c.Request().Context(), id)
	if err != nil {
		log.Printf("TogglePolicy failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	status := "启用"
	if !policy.Enabled {
		status = "禁用"
	}
	h.writeAuditLog(c, "approval_policy_toggle", status+"审批策略: "+policy.Name)

	return c.JSON(http.StatusOK, policy)
}

type reorderRequest struct {
	Priorities map[int64]int `json:"priorities"` // policy ID -> new priority
}

// ReorderPolicies handles PUT /api/admin/approval-policies/reorder.
func (h *ApprovalHandler) ReorderPolicies(c echo.Context) error {
	var req reorderRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
	}
	if len(req.Priorities) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "优先级列表不能为空"})
	}

	if err := h.engine.ReorderPolicies(c.Request().Context(), req.Priorities); err != nil {
		log.Printf("ReorderPolicies failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	h.writeAuditLog(c, "approval_policy_reorder", "调整审批策略优先级")

	return c.JSON(http.StatusOK, map[string]string{"message": "排序已更新"})
}

// GetApprovers handles GET /api/admin/approval-policies/approvers.
func (h *ApprovalHandler) GetApprovers(c echo.Context) error {
	approvers, err := h.engine.GetApprovers(c.Request().Context())
	if err != nil {
		log.Printf("GetApprovers failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if approvers == nil {
		approvers = []service.ApproverInfo{}
	}
	return c.JSON(http.StatusOK, approvers)
}

// GetApprovalChain handles GET /api/tickets/:id/approval-chain.
func (h *ApprovalHandler) GetApprovalChain(c echo.Context) error {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的工单ID"})
	}

	detail, err := h.engine.GetApprovalChainDetail(c.Request().Context(), ticketID)
	if err != nil {
		log.Printf("GetApprovalChain failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, detail)
}

type processApprovalRequest struct {
	Action  string `json:"action"` // approved, rejected
	Comment string `json:"comment"`
}

// ProcessApproval handles POST /api/tickets/:id/engine-approve (using engine).
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

// validateApprovalChainJSON validates the approval chain JSON string.
func validateApprovalChainJSON(chain string) error {
	chain = trimSpace(chain)
	if chain == "" || chain == "[]" {
		return nil // empty chain is valid (will be caught by engine later)
	}

	var stages []service.ApprovalChainStage
	if err := json.Unmarshal([]byte(chain), &stages); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "审批链 JSON 格式无效: "+err.Error())
	}

	validRoles := map[string]bool{
		"admin": true, "dba": true, "team_lead": true, "developer": true,
	}

	for i, s := range stages {
		if s.Role == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "审批链第 "+strconv.Itoa(i+1)+" 阶段缺少角色")
		}
		if !validRoles[s.Role] {
			return echo.NewHTTPError(http.StatusBadRequest, "审批链第 "+strconv.Itoa(i+1)+" 阶段角色无效: "+s.Role)
		}
	}

	return nil
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
