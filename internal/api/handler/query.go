package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// QueryHandler handles SQL query related requests.
type QueryHandler struct {
	querySvc   *service.QueryService
	historySvc *service.QueryHistoryService
}

// NewQueryHandler creates a new QueryHandler.
func NewQueryHandler(querySvc *service.QueryService, historySvc *service.QueryHistoryService) *QueryHandler {
	return &QueryHandler{
		querySvc:   querySvc,
		historySvc: historySvc,
	}
}

type executeQueryRequest struct {
	DatasourceID int64  `json:"datasource_id"`
	Database     string `json:"database"`
	SQL          string `json:"sql"`
}

// ExecuteQuery handles POST /api/query/execute.
//
// @Summary 执行SQL查询
// @Description 在指定数据源上执行SQL查询，仅允许SELECT操作
// @Tags 查询
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body executeQueryRequest true "查询请求"
// @Success 200 {object} resp.SuccessResponse{data=service.QueryResult} "查询成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 403 {object} resp.ErrorResponse "操作被拦截"
// @Router /query/execute [post]
func (h *QueryHandler) ExecuteQuery(c echo.Context) error {
	var req executeQueryRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.DatasourceID == 0 {
		return resp.BadRequest(c, "数据源ID不能为空")
	}
	if req.SQL == "" {
		return resp.BadRequest(c, "SQL不能为空")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	username := c.Get(middleware.ContextKeyUsername).(string)
	role := c.Get(middleware.ContextKeyRole).(string)

	result, err := h.querySvc.ExecuteQuery(c.Request().Context(), userID, username, role, req.DatasourceID, req.Database, req.SQL, "")
	if err != nil {
		switch err {
		case service.ErrSQLOperationForbidden:
			return resp.Forbidden(c, "该操作需要提交工单，仅允许 SELECT 查询")
		case service.ErrSQLHighRisk:
			return resp.Forbidden(c, "高风险操作被拦截，请提交工单")
		case service.ErrSQLBlocked:
			return resp.Forbidden(c, "SQL操作被拦截")
		case service.ErrSQLTimeout:
			return resp.BadRequest(c, "查询超时（30秒），请优化查询或缩小范围")
		case service.ErrEmptySQL:
			return resp.BadRequest(c, "SQL不能为空")
		default:
			log.Printf("ExecuteQuery failed: %v", err)
			return resp.InternalError(c, "查询执行失败")
		}
	}

	return resp.OK(c, result)
}

// ListHistory handles GET /api/query/history.
//
// @Summary 获取查询历史
// @Description 获取当前用户的SQL查询历史记录
// @Tags 查询
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(50)
// @Param keyword query string false "搜索关键词"
// @Success 200 {object} resp.PageResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "获取查询历史失败"
// @Router /query/history [get]
func (h *QueryHandler) ListHistory(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}

	keyword := c.QueryParam("keyword")

	list, total, err := h.historySvc.ListHistory(c.Request().Context(), userID, page, pageSize, keyword)
	if err != nil {
		return resp.InternalError(c, "获取查询历史失败")
	}

	if list == nil {
		list = make([]model.QueryHistory, 0)
	}

	return resp.OKPage(c, list, int64(page), int64(pageSize), int64(total))
}

// DeleteHistory handles DELETE /api/query/history/:id.
//
// @Summary 删除查询历史
// @Description 删除指定的查询历史记录
// @Tags 查询
// @Produce json
// @Security BearerAuth
// @Param id path int true "历史记录ID"
// @Success 200 {object} resp.SuccessResponse "删除成功"
// @Failure 400 {object} resp.ErrorResponse "无效的ID"
// @Router /query/history/{id} [delete]
func (h *QueryHandler) DeleteHistory(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的历史记录ID")
	}

	if err := h.historySvc.DeleteHistory(c.Request().Context(), id, userID); err != nil {
		log.Printf("DeleteHistory failed: %v", err)
		return resp.BadRequest(c, "删除失败，记录不存在或无权操作")
	}

	return resp.OKWithMessage(c, "删除成功", nil)
}

// ClearHistory handles DELETE /api/query/history.
//
// @Summary 清空查询历史
// @Description 清空当前用户的所有查询历史记录
// @Tags 查询
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "已清空"
// @Failure 500 {object} resp.ErrorResponse "清空查询历史失败"
// @Router /query/history [delete]
func (h *QueryHandler) ClearHistory(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)

	if err := h.historySvc.ClearHistory(c.Request().Context(), userID); err != nil {
		return resp.InternalError(c, "清空查询历史失败")
	}

	return resp.OKWithMessage(c, "已清空所有查询历史", nil)
}

type exportQueryRequest struct {
	DatasourceID int64  `json:"datasource_id"`
	Database     string `json:"database"`
	SQL          string `json:"sql"`
	Format       string `json:"format"` // "csv" or "json"
}

// ExportQuery handles POST /api/query/export.
//
// @Summary 导出查询结果
// @Description 导出SQL查询结果为CSV或JSON文件
// @Tags 查询
// @Accept json
// @Produce octet-stream
// @Security BearerAuth
// @Param body body exportQueryRequest true "导出请求"
// @Success 200 {file} file "导出文件"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 403 {object} resp.ErrorResponse "操作被拦截"
// @Router /query/export [post]
func (h *QueryHandler) ExportQuery(c echo.Context) error {
	var req exportQueryRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.DatasourceID == 0 {
		return resp.BadRequest(c, "数据源ID不能为空")
	}
	if req.SQL == "" {
		return resp.BadRequest(c, "SQL不能为空")
	}
	if req.Format != "csv" && req.Format != "json" {
		return resp.BadRequest(c, "导出格式仅支持 csv 或 json")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	username := c.Get(middleware.ContextKeyUsername).(string)
	role := c.Get(middleware.ContextKeyRole).(string)

	result, err := h.querySvc.ExportQuery(c.Request().Context(), userID, username, role, req.DatasourceID, req.Database, req.SQL, "")
	if err != nil {
		switch err {
		case service.ErrSQLOperationForbidden:
			return resp.Forbidden(c, "该操作需要提交工单，仅允许 SELECT 查询")
		case service.ErrSQLHighRisk:
			return resp.Forbidden(c, "高风险操作被拦截，请提交工单")
		case service.ErrSQLBlocked:
			return resp.Forbidden(c, "SQL操作被拦截")
		case service.ErrSQLTimeout:
			return resp.BadRequest(c, "查询超时（30秒），请优化查询或缩小范围")
		case service.ErrEmptySQL:
			return resp.BadRequest(c, "SQL不能为空")
		case service.ErrExportRowLimit:
			return resp.BadRequest(c, "导出数据超过10000行上限，请添加 LIMIT 条件缩小范围")
		default:
			log.Printf("ExportQuery failed: %v", err)
			return resp.InternalError(c, "导出失败")
		}
	}

	switch req.Format {
	case "csv":
		return writeCSV(c, result)
	case "json":
		return writeExportJSON(c, result)
	default:
		return resp.BadRequest(c, "不支持的导出格式")
	}
}

// csvEscape escapes a value for CSV format.
func csvEscape(s string) string {
	if strings.Contains(s, ",") || strings.Contains(s, "\"") || strings.Contains(s, "\n") || strings.Contains(s, "\r") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}

// writeCSV writes the query result as a CSV file download.
func writeCSV(c echo.Context, result *service.QueryResult) error {
	c.Response().Header().Set(echo.HeaderContentType, "text/csv; charset=utf-8")
	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=export.csv")
	c.Response().WriteHeader(http.StatusOK)

	w := c.Response().Writer

	// Write header row
	for i, col := range result.Columns {
		if i > 0 {
			_, _ = w.Write([]byte{','})
		}
		_, _ = w.Write([]byte(csvEscape(col)))
	}
	_, _ = w.Write([]byte{'\n'})

	// Write data rows
	for _, row := range result.Rows {
		for i, col := range result.Columns {
			if i > 0 {
				_, _ = w.Write([]byte{','})
			}
			val := ""
			if v, ok := row[col]; ok && v != nil {
				val = fmt.Sprintf("%v", v)
			}
			_, _ = w.Write([]byte(csvEscape(val)))
		}
		_, _ = w.Write([]byte{'\n'})
	}

	return nil
}

// writeExportJSON writes the query result as a JSON file download.
func writeExportJSON(c echo.Context, result *service.QueryResult) error {
	c.Response().Header().Set(echo.HeaderContentType, "application/json")
	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=export.json")
	c.Response().WriteHeader(http.StatusOK)

	return json.NewEncoder(c.Response().Writer).Encode(result.Rows)
}
