package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// ExportHandler handles data export requests for audit logs and tickets.
type ExportHandler struct {
	exportSvc      *service.ExportService
	exportAsyncSvc *service.ExportAsyncService
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(exportSvc *service.ExportService, exportAsyncSvc *service.ExportAsyncService) *ExportHandler {
	return &ExportHandler{exportSvc: exportSvc, exportAsyncSvc: exportAsyncSvc}
}

type exportAuditRequest struct {
	UserID       string `query:"user_id"`
	Action       string `query:"action"`
	DatasourceID string `query:"datasource_id"`
	Start        string `query:"start"`
	End          string `query:"end"`
	Keyword      string `query:"keyword"`
	Async        string `query:"async"` // "1" to force async
}

// ExportAuditLogs handles GET /api/export/audit.
// Streams CSV for small datasets, creates async task for large datasets.
// ExportAuditLogs godoc
// @Summary 导出审计日志
// @Description 管理员/DBA导出审计日志为CSV（流式）
// @Tags 导出
// @Produce text/csv
// @Security BearerAuth
// @Param start_date query string false "开始日期"
// @Param end_date query string false "结束日期"
// @Success 200 {file} file "CSV文件"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 403 {object} resp.ErrorResponse "无权限"
// @Router /export/audit [get]
func (h *ExportHandler) ExportAuditLogs(c echo.Context) error {
	userID := getContextUserID(c)
	username := getContextUsername(c)
	role := getContextRole(c)

	var req exportAuditRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求参数格式错误")
	}

	filters := service.AuditExportFilters{
		UserID:       req.UserID,
		Action:       req.Action,
		DatasourceID: req.DatasourceID,
		Start:        req.Start,
		End:          req.End,
		Keyword:      req.Keyword,
	}

	forceAsync := req.Async == "1"

	if forceAsync {
		return h.createAsyncExport(c, userID, username, role, "audit", filters)
	}

	// Validate: check permission + row count
	total, err := h.exportSvc.ValidateExport(c.Request().Context(), role, service.ExportTypeAudit, filters)
	if err != nil {
		if err == service.ErrExportExceedsLimit {
			return h.createAsyncExport(c, userID, username, role, "audit", filters)
		}
		switch err {
		case service.ErrExportNoPermission:
			return resp.Forbidden(c, "没有导出权限，仅管理员和DBA可以导出审计日志")
		default:
			log.Printf("ValidateExport audit failed: %v", err)
			return resp.InternalError(c, "导出审计日志失败")
		}
	}

	// Stream CSV response
	return h.streamAuditCSVResponse(c, userID, username, filters, total)
}

type exportTicketRequest struct {
	Status       string `query:"status"`
	DatasourceID string `query:"datasource_id"`
	RiskLevel    string `query:"risk_level"`
	Keyword      string `query:"keyword"`
	Async        string `query:"async"`
}

// ExportTickets handles GET /api/export/tickets.
// Streams CSV for small datasets, creates async task for large datasets.
// ExportTickets godoc
// @Summary 导出工单
// @Description 认证用户导出工单为CSV（流式）
// @Tags 导出
// @Produce text/csv
// @Security BearerAuth
// @Param status query string false "工单状态"
// @Param risk_level query string false "风险等级"
// @Success 200 {file} file "CSV文件"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Router /export/tickets [get]
func (h *ExportHandler) ExportTickets(c echo.Context) error {
	userID := getContextUserID(c)
	username := getContextUsername(c)
	role := getContextRole(c)

	var req exportTicketRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求参数格式错误")
	}

	filters := service.TicketExportFilters{
		Status:       req.Status,
		DatasourceID: req.DatasourceID,
		RiskLevel:    req.RiskLevel,
		Keyword:      req.Keyword,
	}

	forceAsync := req.Async == "1"

	if forceAsync {
		return h.createAsyncExport(c, userID, username, role, "ticket", filters)
	}

	// Validate: check permission + row count
	total, err := h.exportSvc.ValidateExport(c.Request().Context(), role, service.ExportTypeTicket, filters)
	if err != nil {
		if err == service.ErrExportExceedsLimit {
			return h.createAsyncExport(c, userID, username, role, "ticket", filters)
		}
		switch err {
		case service.ErrExportNoPermission:
			return resp.Forbidden(c, "没有导出权限")
		default:
			log.Printf("ValidateExport ticket failed: %v", err)
			return resp.InternalError(c, "导出工单失败")
		}
	}

	// Stream CSV response
	return h.streamTicketCSVResponse(c, userID, username, filters, total)
}

// streamAuditCSVResponse writes streaming CSV response for audit logs.
func (h *ExportHandler) streamAuditCSVResponse(c echo.Context, userID int64, username string, filters service.AuditExportFilters, total int64) error {
	filename := fmt.Sprintf("audit_logs_%s.csv", time.Now().Format("2006-01-02"))
	setStreamingCSVHeaders(c, filename, total)

	// Write BOM
	c.Response().Write([]byte{0xEF, 0xBB, 0xBF})

	written, err := h.exportSvc.StreamExportAuditLogs(c.Request().Context(), c.Response(), username, filters)
	if err != nil {
		log.Printf("StreamExportAuditLogs error after %d rows: %v", written, err)
		return nil // headers already sent
	}

	// Write watermark
	fmt.Fprintf(c.Response(), "\n# 导出水印: 导出人=%s | 导出时间=%s | 仅限内部使用\n",
		username,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)

	// Record audit log
	h.exportSvc.WriteAuditExportLog(c.Request().Context(), userID, written)

	return nil
}

// streamTicketCSVResponse writes streaming CSV response for tickets.
func (h *ExportHandler) streamTicketCSVResponse(c echo.Context, userID int64, username string, filters service.TicketExportFilters, total int64) error {
	filename := fmt.Sprintf("tickets_%s.csv", time.Now().Format("2006-01-02"))
	setStreamingCSVHeaders(c, filename, total)

	c.Response().Write([]byte{0xEF, 0xBB, 0xBF})

	written, err := h.exportSvc.StreamExportTickets(c.Request().Context(), c.Response(), username, filters)
	if err != nil {
		log.Printf("StreamExportTickets error after %d rows: %v", written, err)
		return nil
	}

	fmt.Fprintf(c.Response(), "\n# 导出水印: 导出人=%s | 导出时间=%s | 仅限内部使用\n",
		username,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)

	h.exportSvc.WriteTicketExportLog(c.Request().Context(), userID, written)

	return nil
}

// setStreamingCSVHeaders sets the HTTP headers for a streaming CSV download.
func setStreamingCSVHeaders(c echo.Context, filename string, totalRows int64) {
	safeFilename := filepath.Base(filename)
	c.Response().Header().Set(echo.HeaderContentType, "text/csv; charset=utf-8")
	c.Response().Header().Set(echo.HeaderContentDisposition,
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safeFilename, url.PathEscape(safeFilename)))
	c.Response().Header().Set("X-Export-Rows", fmt.Sprintf("%d", totalRows))
	c.Response().Header().Set("X-Export-Timestamp", time.Now().UTC().Format(http.TimeFormat))
	c.Response().WriteHeader(http.StatusOK)
}

// createAsyncExport creates an async export task and returns 202 with task info.
func (h *ExportHandler) createAsyncExport(c echo.Context, userID int64, username, role, exportType string, filters interface{}) error {
	if h.exportAsyncSvc == nil {
		return resp.InternalError(c, "异步导出服务未启用")
	}

	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return resp.BadRequest(c, "序列化筛选参数失败")
	}

	task, err := h.exportAsyncSvc.CreateAsyncExport(c.Request().Context(), userID, username, role, exportType, string(filtersJSON))
	if err != nil {
		switch err {
		case service.ErrExportNoPermission:
			return resp.Forbidden(c, "没有导出权限")
		default:
			log.Printf("createAsyncExport failed: %v", err)
			return resp.InternalError(c, "创建异步导出任务失败")
		}
	}

	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"code":    0,
		"message": "导出任务已创建，数据量较大，正在后台生成中",
		"data": map[string]interface{}{
			"task_id":     task.ID,
			"status":      task.Status,
			"export_type": task.ExportType,
			"created_at":  task.CreatedAt,
		},
	})
}

// GetExportTask handles GET /api/export/tasks/:id.
// GetExportTask godoc
// @Summary 获取导出任务详情
// @Description 获取指定导出任务的状态和详情
// @Tags 导出
// @Produce json
// @Security BearerAuth
// @Param id path int true "任务ID"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 404 {object} resp.ErrorResponse "任务不存在"
// @Router /export/tasks/{id} [get]
func (h *ExportHandler) GetExportTask(c echo.Context) error {
	userID := getContextUserID(c)

	taskID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的任务ID")
	}

	task, err := h.exportAsyncSvc.GetTask(c.Request().Context(), taskID, userID)
	if err != nil {
		switch err {
		case service.ErrExportNotFound:
			return resp.NotFound(c, "导出任务不存在")
		default:
			log.Printf("GetExportTask failed: %v", err)
			return resp.InternalError(c, "查询导出任务失败")
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "ok",
		"data":    task,
	})
}

// ListExportTasks handles GET /api/export/tasks.
// ListExportTasks godoc
// @Summary 导出任务列表
// @Description 获取当前用户的异步导出任务列表
// @Tags 导出
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /export/tasks [get]
func (h *ExportHandler) ListExportTasks(c echo.Context) error {
	userID := getContextUserID(c)

	tasks, err := h.exportAsyncSvc.ListTasks(c.Request().Context(), userID)
	if err != nil {
		log.Printf("ListExportTasks failed: %v", err)
		return resp.InternalError(c, "查询导出任务列表失败")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "ok",
		"data":    tasks,
	})
}

// DownloadExportFile handles GET /api/export/tasks/:id/download.
// DownloadExportFile godoc
// @Summary 下载导出文件
// @Description 下载指定导出任务生成的文件
// @Tags 导出
// @Produce application/octet-stream
// @Security BearerAuth
// @Param id path int true "任务ID"
// @Success 200 {file} file "导出文件"
// @Failure 404 {object} resp.ErrorResponse "文件不存在"
// @Router /export/tasks/{id}/download [get]
func (h *ExportHandler) DownloadExportFile(c echo.Context) error {
	userID := getContextUserID(c)

	taskID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的任务ID")
	}

	reader, filename, err := h.exportAsyncSvc.DownloadFile(c.Request().Context(), taskID, userID)
	if err != nil {
		switch err {
		case service.ErrExportNotFound:
			return resp.NotFound(c, "导出任务不存在")
		case service.ErrExportNotReady:
			return resp.BadRequest(c, "导出任务尚未完成，请稍后再试")
		case service.ErrExportFileGone:
			return resp.BadRequest(c, "导出文件已过期或已清理")
		default:
			log.Printf("DownloadExportFile failed: %v", err)
			return resp.InternalError(c, "下载导出文件失败")
		}
	}
	defer reader.Close()

	safeFilename := filepath.Base(filename)
	c.Response().Header().Set(echo.HeaderContentType, "text/csv; charset=utf-8")
	c.Response().Header().Set(echo.HeaderContentDisposition,
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safeFilename, url.PathEscape(safeFilename)))
	c.Response().WriteHeader(http.StatusOK)

	_, err = io.Copy(c.Response(), reader)
	if err != nil {
		log.Printf("DownloadExportFile write error: %v", err)
	}
	return nil
}

// writeCSVResponse writes the CSV export result as a file download response.
// Deprecated: used only for backward-compatible in-memory exports.
func writeCSVResponse(c echo.Context, result *service.ExportResult) error {
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
