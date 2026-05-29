package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

type resubmitTicketRequest struct {
	SQLContent   string `json:"sql_content"`
	ChangeReason string `json:"change_reason"`
}

// ResubmitTicket handles PUT /api/tickets/:id/resubmit.
//
// @Summary 驳回后重提工单
// @Description 修改SQL后重新提交被驳回的工单
// @Tags 工单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "工单ID"
// @Param body body resubmitTicketRequest true "重提请求"
// @Success 200 {object} resp.SuccessResponse "重提成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 403 {object} resp.ErrorResponse "无权限"
// @Failure 404 {object} resp.ErrorResponse "工单不存在"
// @Router /tickets/{id}/resubmit [put]
func (h *TicketHandler) ResubmitTicket(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	var req resubmitTicketRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.SQLContent == "" {
		return resp.BadRequest(c, "SQL内容不能为空")
	}

	userID := getContextUserID(c)

	ticket, err := h.ticketSvc.ResubmitTicket(c.Request().Context(), id, userID, req.SQLContent, req.ChangeReason)
	if err != nil {
		switch err {
		case service.ErrTicketNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrNoPermission:
			return resp.Forbidden(c, err.Error())
		case service.ErrTicketNotResubmittable:
			return resp.BadRequest(c, err.Error())
		case service.ErrTicketSQLRequired:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("ResubmitTicket failed: %v", err)
			return resp.InternalError(c, "重提工单失败")
		}
	}

	return resp.OK(c, ticket)
}

// ListRevisions handles GET /api/tickets/:id/revisions.
//
// @Summary 查看工单历史版本
// @Description 获取工单的历史修订版本列表
// @Tags 工单
// @Produce json
// @Security BearerAuth
// @Param id path int true "工单ID"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 400 {object} resp.ErrorResponse "无效的工单ID"
// @Failure 404 {object} resp.ErrorResponse "工单不存在"
// @Router /tickets/{id}/revisions [get]
func (h *TicketHandler) ListRevisions(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	// Verify ticket exists
	_, err = h.ticketSvc.GetTicket(c.Request().Context(), id)
	if err != nil {
		switch err {
		case service.ErrTicketNotFound:
			return resp.NotFound(c, err.Error())
		default:
			log.Printf("ListRevisions: GetTicket failed: %v", err)
			return resp.InternalError(c, "获取工单失败")
		}
	}

	revisions, err := h.ticketSvc.ListRevisions(c.Request().Context(), id)
	if err != nil {
		log.Printf("ListRevisions failed: %v", err)
		return resp.InternalError(c, "获取历史版本失败")
	}

	return resp.OK(c, revisions)
}
