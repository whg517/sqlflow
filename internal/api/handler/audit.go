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
