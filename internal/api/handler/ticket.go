package handler

import (
	"log"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// TicketHandler handles ticket related requests.
type TicketHandler struct {
	ticketSvc *service.TicketService
}

// NewTicketHandler creates a new TicketHandler.
func NewTicketHandler(ticketSvc *service.TicketService) *TicketHandler {
	return &TicketHandler{ticketSvc: ticketSvc}
}

type createTicketRequest struct {
	DatasourceID   int64  `json:"datasource_id"`
	Database       string `json:"database"`
	SQL            string `json:"sql"`
	DBType         string `json:"db_type"`
	ChangeReason   string `json:"change_reason"`
	RiskLevel      string `json:"risk_level"`
	AIReviewResult string `json:"ai_review_result"`
}

// CreateTicket handles POST /api/tickets.
func (h *TicketHandler) CreateTicket(c echo.Context) error {
	var req createTicketRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.DatasourceID == 0 {
		return resp.BadRequest(c, "数据源ID不能为空")
	}
	if req.SQL == "" {
		return resp.BadRequest(c, "SQL内容不能为空")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)

	ticket, err := h.ticketSvc.CreateTicket(
		c.Request().Context(), userID, req.DatasourceID, req.Database, req.SQL,
		req.DBType, req.ChangeReason, req.RiskLevel, req.AIReviewResult,
	)
	if err != nil {
		switch err {
		case service.ErrTicketSQLRequired:
			return resp.BadRequest(c, err.Error())
		case service.ErrTicketDatasourceRequired:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("CreateTicket failed: %v", err)
			return resp.InternalError(c, "创建工单失败")
		}
	}

	return resp.Created(c, ticket)
}

// GetTicket handles GET /api/tickets/:id.
func (h *TicketHandler) GetTicket(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	ticket, err := h.ticketSvc.GetTicket(c.Request().Context(), id)
	if err != nil {
		switch err {
		case service.ErrTicketNotFound:
			return resp.NotFound(c, err.Error())
		default:
			log.Printf("GetTicket failed: %v", err)
			return resp.InternalError(c, "获取工单失败")
		}
	}

	return resp.OK(c, ticket)
}

// ListTickets handles GET /api/tickets.
func (h *TicketHandler) ListTickets(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	role := c.Get(middleware.ContextKeyRole).(string)

	tickets, total, err := h.ticketSvc.ListTickets(
		c.Request().Context(), page, pageSize,
		c.QueryParam("status"),
		c.QueryParam("datasource_id"),
		c.QueryParam("submitter_id"),
		c.QueryParam("risk_level"),
		c.QueryParam("keyword"),
		c.QueryParam("scope"),
		userID, role,
	)
	if err != nil {
		log.Printf("ListTickets failed: %v", err)
		return resp.InternalError(c, "获取工单列表失败")
	}

	return resp.OKPage(c, tickets, int64(page), int64(pageSize), total)
}

type approveTicketRequest struct {
	Comment string `json:"comment"`
}

// ApproveTicket handles POST /api/tickets/:id/approve.
func (h *TicketHandler) ApproveTicket(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	var req approveTicketRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	role := c.Get(middleware.ContextKeyRole).(string)

	ticket, err := h.ticketSvc.ApproveTicket(c.Request().Context(), id, userID, role, req.Comment)
	if err != nil {
		switch err {
		case service.ErrNoPermission:
			return resp.Forbidden(c, err.Error())
		case service.ErrTicketNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrInvalidStatusTransition:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("ApproveTicket failed: %v", err)
			return resp.InternalError(c, "审批工单失败")
		}
	}

	return resp.OK(c, ticket)
}

type rejectTicketRequest struct {
	Reason string `json:"reason"`
}

// RejectTicket handles POST /api/tickets/:id/reject.
func (h *TicketHandler) RejectTicket(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	var req rejectTicketRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.Reason == "" {
		return resp.BadRequest(c, "驳回原因不能为空")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	role := c.Get(middleware.ContextKeyRole).(string)

	ticket, err := h.ticketSvc.RejectTicket(c.Request().Context(), id, userID, role, req.Reason)
	if err != nil {
		switch err {
		case service.ErrNoPermission:
			return resp.Forbidden(c, err.Error())
		case service.ErrTicketNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrInvalidStatusTransition:
			return resp.BadRequest(c, err.Error())
		case service.ErrRejectReasonRequired:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("RejectTicket failed: %v", err)
			return resp.InternalError(c, "驳回工单失败")
		}
	}

	return resp.OK(c, ticket)
}

type cancelTicketRequest struct {
	Reason string `json:"reason"`
}

// CancelTicket handles POST /api/tickets/:id/cancel.
func (h *TicketHandler) CancelTicket(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	var req cancelTicketRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.Reason == "" {
		return resp.BadRequest(c, "取消原因不能为空")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	role := c.Get(middleware.ContextKeyRole).(string)

	ticket, err := h.ticketSvc.CancelTicket(c.Request().Context(), id, userID, role, req.Reason)
	if err != nil {
		switch err {
		case service.ErrNoPermission:
			return resp.Forbidden(c, err.Error())
		case service.ErrTicketNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrTicketNotCancellable:
			return resp.BadRequest(c, err.Error())
		case service.ErrCancelReasonRequired:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("CancelTicket failed: %v", err)
			return resp.InternalError(c, "取消工单失败")
		}
	}

	return resp.OK(c, ticket)
}

// ExecuteTicket handles POST /api/tickets/:id/execute.
func (h *TicketHandler) ExecuteTicket(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	role := c.Get(middleware.ContextKeyRole).(string)
	username := c.Get(middleware.ContextKeyUsername).(string)

	ticket, err := h.ticketSvc.ExecuteTicket(c.Request().Context(), id, userID, role, username)
	if err != nil {
		switch err {
		case service.ErrNoPermission:
			return resp.Forbidden(c, err.Error())
		case service.ErrTicketNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrTicketNotExecutable:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("ExecuteTicket failed: %v", err)
			return resp.InternalError(c, "执行工单失败")
		}
	}

	return resp.OK(c, ticket)
}

type scheduleTicketRequest struct {
	ScheduledAt string `json:"scheduled_at"` // RFC3339 format
}

// ScheduleTicket handles POST /api/tickets/:id/schedule.
func (h *TicketHandler) ScheduleTicket(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	var req scheduleTicketRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.ScheduledAt == "" {
		return resp.BadRequest(c, "定时执行时间不能为空")
	}

	scheduledAt, err := time.Parse(time.RFC3339, req.ScheduledAt)
	if err != nil {
		return resp.BadRequest(c, "定时执行时间格式错误，请使用 RFC3339 格式 (如: 2026-05-23T10:00:00+08:00)")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	role := c.Get(middleware.ContextKeyRole).(string)

	ticket, err := h.ticketSvc.ScheduleTicket(c.Request().Context(), id, userID, role, scheduledAt)
	if err != nil {
		switch err {
		case service.ErrNoPermission:
			return resp.Forbidden(c, err.Error())
		case service.ErrTicketNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrTicketNotSchedulable:
			return resp.BadRequest(c, err.Error())
		case service.ErrScheduleTimeRequired:
			return resp.BadRequest(c, err.Error())
		case service.ErrScheduleTimeInPast:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("ScheduleTicket failed: %v", err)
			return resp.InternalError(c, "设置定时执行失败")
		}
	}

	return resp.OK(c, ticket)
}

// CancelSchedule handles POST /api/tickets/:id/cancel-schedule.
func (h *TicketHandler) CancelSchedule(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	role := c.Get(middleware.ContextKeyRole).(string)

	ticket, err := h.ticketSvc.CancelSchedule(c.Request().Context(), id, userID, role)
	if err != nil {
		switch err {
		case service.ErrNoPermission:
			return resp.Forbidden(c, err.Error())
		case service.ErrTicketNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrTicketNotScheduled:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("CancelSchedule failed: %v", err)
			return resp.InternalError(c, "取消定时执行失败")
		}
	}

	return resp.OK(c, ticket)
}
