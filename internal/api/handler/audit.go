package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// AuditHandler handles audit log related requests.
type AuditHandler struct {
	auditSvc *service.AuditService
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(auditSvc *service.AuditService) *AuditHandler {
	return &AuditHandler{auditSvc: auditSvc}
}

// ListAuditLogs handles GET /api/audit-logs.
// SearchAuditLogs handles GET /api/audit-logs/search.
// Uses FTS5 for full-text search with keyword, action, time range, and user_id filters.
//
// @Summary 全文搜索审计日志
// @Description 使用FTS5全文搜索审计日志
// @Tags 审计
// @Produce json
// @Security BearerAuth
// @Param keyword query string true "搜索关键词"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(50)
// @Param user_id query string false "用户ID"
// @Param action query string false "操作类型"
// @Param start query string false "开始时间"
// @Param end query string false "结束时间"
// @Success 200 {object} resp.PageResponse "成功"
// @Failure 400 {object} resp.ErrorResponse "keyword 参数必填"
// @Failure 500 {object} resp.ErrorResponse "搜索失败"
// @Router /audit-logs/search [get]
func (h *AuditHandler) SearchAuditLogs(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}

	keyword := c.QueryParam("keyword")
	if keyword == "" {
		return resp.BadRequest(c, "keyword parameter is required")
	}

	result, err := h.auditSvc.Search(
		c.Request().Context(),
		service.SearchParams{
			Keyword:  keyword,
			Page:     page,
			PageSize: pageSize,
			UserID:   c.QueryParam("user_id"),
			Action:   c.QueryParam("action"),
			Start:    c.QueryParam("start"),
			End:      c.QueryParam("end"),
		},
	)
	if err != nil {
		log.Printf("SearchAuditLogs failed: %v", err)
		return resp.InternalError(c, "全文搜索审计日志失败")
	}

	return resp.OKPage(c, result.Logs, int64(page), int64(pageSize), result.Total)
}

// ListAuditLogs handles GET /api/audit-logs.
//
// @Summary 获取审计日志列表
// @Description 获取审计日志列表，支持分页和筛选
// @Tags 审计
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(50)
// @Param user_id query string false "用户ID"
// @Param action query string false "操作类型"
// @Param datasource_id query string false "数据源ID"
// @Param start query string false "开始时间"
// @Param end query string false "结束时间"
// @Param keyword query string false "搜索关键词"
// @Success 200 {object} resp.PageResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "获取审计日志失败"
// @Router /audit-logs [get]
func (h *AuditHandler) ListAuditLogs(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}

	logs, total, err := h.auditSvc.List(
		c.Request().Context(),
		page, pageSize,
		c.QueryParam("user_id"),
		c.QueryParam("action"),
		c.QueryParam("datasource_id"),
		c.QueryParam("start"),
		c.QueryParam("end"),
		c.QueryParam("keyword"),
	)
	if err != nil {
		log.Printf("ListAuditLogs failed: %v", err)
		return resp.InternalError(c, "获取审计日志失败")
	}

	return resp.OKPage(c, logs, int64(page), int64(pageSize), total)
}
