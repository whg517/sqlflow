package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// SLAHandler handles SLA configuration and status requests.
type SLAHandler struct {
	slaSvc *service.SLAService
}

// NewSLAHandler creates a new SLAHandler.
func NewSLAHandler(slaSvc *service.SLAService) *SLAHandler {
	return &SLAHandler{slaSvc: slaSvc}
}

// ListSLAConfigs handles GET /api/settings/sla.
func (h *SLAHandler) ListSLAConfigs(c echo.Context) error {
	configs, err := h.slaSvc.ListConfigs(c.Request().Context())
	if err != nil {
		return resp.InternalError(c, "获取 SLA 配置失败")
	}
	return resp.OK(c, configs)
}

type createSLAConfigRequest struct {
	Priority        string `json:"priority"`
	TimeoutMinutes  int    `json:"timeout_minutes"`
	ReminderPercent int    `json:"reminder_percent"`
	EscalateToRole  string `json:"escalate_to_role"`
	EscalateToUser  string `json:"escalate_to_user"`
	Enabled         bool   `json:"enabled"`
}

// CreateSLAConfig handles POST /api/settings/sla.
func (h *SLAHandler) CreateSLAConfig(c echo.Context) error {
	var req createSLAConfigRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}
	if req.Priority == "" {
		return resp.BadRequest(c, "优先级不能为空")
	}
	if req.TimeoutMinutes <= 0 {
		return resp.BadRequest(c, "超时时间必须大于0")
	}

	cfg := &model.SLAConfig{
		Priority:        req.Priority,
		TimeoutMinutes:  req.TimeoutMinutes,
		ReminderPercent: req.ReminderPercent,
		EscalateToRole:  req.EscalateToRole,
		EscalateToUser:  req.EscalateToUser,
		Enabled:         req.Enabled,
	}
	created, err := h.slaSvc.CreateConfig(c.Request().Context(), cfg)
	if err != nil {
		return resp.InternalError(c, "创建 SLA 配置失败")
	}
	return resp.Created(c, created)
}

// UpdateSLAConfig handles PUT /api/settings/sla/:id.
func (h *SLAHandler) UpdateSLAConfig(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	var req createSLAConfigRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	cfg := &model.SLAConfig{
		Priority:        req.Priority,
		TimeoutMinutes:  req.TimeoutMinutes,
		ReminderPercent: req.ReminderPercent,
		EscalateToRole:  req.EscalateToRole,
		EscalateToUser:  req.EscalateToUser,
		Enabled:         req.Enabled,
	}
	if err := h.slaSvc.UpdateConfig(c.Request().Context(), id, cfg); err != nil {
		return resp.InternalError(c, "更新 SLA 配置失败")
	}
	return resp.OK(c, nil)
}

// DeleteSLAConfig handles DELETE /api/settings/sla/:id.
func (h *SLAHandler) DeleteSLAConfig(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	if err := h.slaSvc.DeleteConfig(c.Request().Context(), id); err != nil {
		return resp.InternalError(c, "删除 SLA 配置失败")
	}
	return resp.OK(c, nil)
}

// GetTicketSLAStatuses handles GET /api/tickets/sla-status?ticket_ids=1,2,3.
func (h *SLAHandler) GetTicketSLAStatuses(c echo.Context) error {
	idsStr := c.QueryParam("ticket_ids")
	if idsStr == "" {
		return resp.BadRequest(c, "ticket_ids 参数不能为空")
	}

	var ticketIDs []int64
	for _, s := range splitIDs(idsStr) {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		ticketIDs = append(ticketIDs, id)
	}

	if len(ticketIDs) == 0 {
		return resp.BadRequest(c, "无有效的 ticket_ids")
	}

	statuses, err := h.slaSvc.GetTicketSLAStatuses(c.Request().Context(), ticketIDs)
	if err != nil {
		return resp.InternalError(c, "查询 SLA 状态失败")
	}
	return resp.OK(c, statuses)
}

// ListSLANotifications handles GET /api/sla-notifications.
func (h *SLAHandler) ListSLANotifications(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	notifications, total, err := h.slaSvc.ListNotifications(c.Request().Context(), page, pageSize)
	if err != nil {
		return resp.InternalError(c, "获取 SLA 通知记录失败")
	}
	return resp.OKPage(c, notifications, int64(page), int64(pageSize), total)
}

// splitIDs splits a comma-separated string of IDs.
func splitIDs(s string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	return result
}
