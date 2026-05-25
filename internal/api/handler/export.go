package handler

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// ExportHandler handles data export requests for audit logs and tickets.
type ExportHandler struct {
	exportSvc *service.ExportService
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(exportSvc *service.ExportService) *ExportHandler {
	return &ExportHandler{exportSvc: exportSvc}
}

type exportAuditRequest struct {
	UserID       string `query:"user_id"`
	Action       string `query:"action"`
	DatasourceID string `query:"datasource_id"`
	Start        string `query:"start"`
	End          string `query:"end"`
	Keyword      string `query:"keyword"`
}

// ExportAuditLogs handles GET /api/export/audit.
// Exports audit logs as CSV with BOM header and user watermark.
// Requires admin or dba role (export permission).
//
// @Summary 导出审计日志
// @Description 导出审计日志为CSV文件，包含用户水印，单次最多10000条
// @Tags 导出
// @Produce octet-stream
// @Security BearerAuth
// @Param user_id query string false "用户ID"
// @Param action query string false "操作类型"
// @Param datasource_id query string false "数据源ID"
// @Param start query string false "开始时间"
// @Param end query string false "结束时间"
// @Param keyword query string false "搜索关键词"
// @Success 200 {file} file "CSV文件"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 403 {object} resp.ErrorResponse "无导出权限"
// @Failure 500 {object} resp.ErrorResponse "导出失败"
// @Router /export/audit [get]
func (h *ExportHandler) ExportAuditLogs(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)
	username := c.Get(middleware.ContextKeyUsername).(string)
	role := c.Get(middleware.ContextKeyRole).(string)

	var req exportAuditRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求参数格式错误")
	}

	result, err := h.exportSvc.ExportAuditLogs(c.Request().Context(), userID, username, role, service.AuditExportFilters{
		UserID:       req.UserID,
		Action:       req.Action,
		DatasourceID: req.DatasourceID,
		Start:        req.Start,
		End:          req.End,
		Keyword:      req.Keyword,
	})
	if err != nil {
		switch err {
		case service.ErrExportNoPermission:
			return resp.Forbidden(c, "没有导出权限，仅管理员和DBA可以导出审计日志")
		case service.ErrExportExceedsLimit:
			return resp.BadRequest(c, "导出数据超过10000行上限，请添加筛选条件缩小范围")
		default:
			log.Printf("ExportAuditLogs failed: %v", err)
			return resp.InternalError(c, "导出审计日志失败")
		}
	}

	return writeCSVResponse(c, result)
}

type exportTicketRequest struct {
	Status       string `query:"status"`
	DatasourceID string `query:"datasource_id"`
	RiskLevel    string `query:"risk_level"`
	Keyword      string `query:"keyword"`
}

// ExportTickets handles GET /api/export/tickets.
// Exports tickets as CSV with BOM header and user watermark.
// All authenticated users can export tickets.
//
// @Summary 导出工单
// @Description 导出工单为CSV文件，包含用户水印，单次最多10000条
// @Tags 导出
// @Produce octet-stream
// @Security BearerAuth
// @Param status query string false "工单状态"
// @Param datasource_id query string false "数据源ID"
// @Param risk_level query string false "风险等级"
// @Param keyword query string false "搜索关键词"
// @Success 200 {file} file "CSV文件"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 403 {object} resp.ErrorResponse "无导出权限"
// @Failure 500 {object} resp.ErrorResponse "导出失败"
// @Router /export/tickets [get]
func (h *ExportHandler) ExportTickets(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)
	username := c.Get(middleware.ContextKeyUsername).(string)
	role := c.Get(middleware.ContextKeyRole).(string)

	var req exportTicketRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求参数格式错误")
	}

	result, err := h.exportSvc.ExportTickets(c.Request().Context(), userID, username, role, service.TicketExportFilters{
		Status:       req.Status,
		DatasourceID: req.DatasourceID,
		RiskLevel:    req.RiskLevel,
		Keyword:      req.Keyword,
	})
	if err != nil {
		switch err {
		case service.ErrExportNoPermission:
			return resp.Forbidden(c, "没有导出权限")
		case service.ErrExportExceedsLimit:
			return resp.BadRequest(c, "导出数据超过10000行上限，请添加筛选条件缩小范围")
		default:
			log.Printf("ExportTickets failed: %v", err)
			return resp.InternalError(c, "导出工单失败")
		}
	}

	return writeCSVResponse(c, result)
}

// writeCSVResponse writes the CSV export result as a file download response.
func writeCSVResponse(c echo.Context, result *service.ExportResult) error {
	// Sanitize filename to prevent path traversal
	safeFilename := filepath.Base(result.Filename)

	c.Response().Header().Set(echo.HeaderContentType, result.ContentType)
	c.Response().Header().Set(echo.HeaderContentDisposition,
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safeFilename, url.PathEscape(safeFilename)))
	c.Response().Header().Set("X-Export-Rows", fmt.Sprintf("%d", result.TotalRows))
	c.Response().Header().Set("X-Export-Timestamp", result.GeneratedAt.UTC().Format(http.TimeFormat))
	c.Response().WriteHeader(http.StatusOK)

	_, err := c.Response().Write(result.CSVBytes)
	if err != nil {
		log.Printf("writeCSVResponse write error: %v", err)
	}
	return nil
}
