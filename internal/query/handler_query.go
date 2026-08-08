package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/httpx"
	"github.com/whg517/sqlflow/internal/resp"
)

// Handler handles SQL query related requests.
type Handler struct {
	querySvc   *Service
	historySvc *HistoryService
}

// NewHandler creates a new Handler.
func NewHandler(querySvc *Service, historySvc *HistoryService) *Handler {
	return &Handler{
		querySvc:   querySvc,
		historySvc: historySvc,
	}
}

type executeQueryRequest struct {
	DatasourceID int64         `json:"datasource_id"`
	Database     string        `json:"database"`
	SQL          string        `json:"sql"`
	Params       []interface{} `json:"params,omitempty"`
}

type explainQueryRequest struct {
	DatasourceID int64         `json:"datasource_id"`
	Database     string        `json:"database"`
	SQL          string        `json:"sql"`
	Params       []interface{} `json:"params,omitempty"`
}

func validateQueryParams(params []interface{}) error {
	if len(params) > 100 {
		return fmt.Errorf("查询参数不能超过 100 个")
	}
	for _, value := range params {
		switch typed := value.(type) {
		case nil, bool, float64, string:
			if text, ok := typed.(string); ok && len(text) > 64*1024 {
				return fmt.Errorf("单个查询参数不能超过 64KB")
			}
		default:
			return fmt.Errorf("查询参数仅支持字符串、数字、布尔值和 null")
		}
	}
	return nil
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
// @Success 200 {object} resp.SuccessResponse{data=QueryResult} "查询成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 403 {object} resp.ErrorResponse "操作被拦截"
// @Router /query/execute [post]
func (h *Handler) ExecuteQuery(c echo.Context) error {
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
	if err := validateQueryParams(req.Params); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	userID := httpx.UserID(c)
	username := httpx.Username(c)
	role := httpx.Role(c)

	result, err := h.querySvc.ExecuteQuery(c.Request().Context(), userID, username, role, req.DatasourceID, req.Database, req.SQL, "", req.Params...)
	if err != nil {
		switch {
		case errors.Is(err, datasource.ErrDatasourceNotFound):
			return resp.BadRequest(c, "数据源不存在")
		case errors.Is(err, datasource.ErrDatasourceDisabled):
			return resp.BadRequest(c, "数据源已禁用")
		case errors.Is(err, datasource.ErrDatabaseScopeMismatch):
			// The message names both databases, so it is passed through rather
			// than replaced: the caller has to know which one is reachable.
			return resp.BadRequest(c, err.Error())
		case errors.Is(err, ErrSQLOperationForbidden):
			return resp.Forbidden(c, "该操作需要提交工单，仅允许 SELECT 查询")
		case errors.Is(err, ErrSQLHighRisk):
			return resp.Forbidden(c, "高风险操作被拦截，请提交工单")
		case errors.Is(err, ErrSQLBlocked):
			return resp.BadRequest(c, "SQL操作被拦截")
		case errors.Is(err, ErrSQLTimeout):
			return resp.BadRequest(c, "查询超时（30秒），请优化查询或缩小范围")
		case errors.Is(err, ErrEmptySQL):
			return resp.BadRequest(c, "SQL不能为空")
		case errors.Is(err, ErrQueryParamsUnsupported):
			return resp.BadRequest(c, err.Error())
		case errors.Is(err, ErrInternalDatasourceOnly):
			return resp.Forbidden(c, err.Error())
		default:
			log.Printf("ExecuteQuery failed: %v", err)
			return resp.InternalError(c, "查询执行失败")
		}
	}

	return resp.OK(c, result)
}

// ExplainQuery handles POST /api/query/explain.
func (h *Handler) ExplainQuery(c echo.Context) error {
	var req explainQueryRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.DatasourceID == 0 {
		return resp.BadRequest(c, "数据源ID不能为空")
	}
	if req.SQL == "" {
		return resp.BadRequest(c, "SQL不能为空")
	}
	if err := validateQueryParams(req.Params); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	userID := httpx.UserID(c)
	role := httpx.Role(c)

	result, err := h.querySvc.ExplainQuery(c.Request().Context(), userID, role, req.DatasourceID, req.Database, req.SQL, req.Params...)
	if err != nil {
		switch {
		case errors.Is(err, ErrExplainNonSelect):
			return resp.BadRequest(c, "EXPLAIN 仅支持 SELECT 语句")
		case errors.Is(err, ErrExplainNotSupported):
			return resp.BadRequest(c, "EXPLAIN 仅支持 MySQL 数据源")
		case errors.Is(err, datasource.ErrDatabaseScopeMismatch):
			return resp.BadRequest(c, err.Error())
		case errors.Is(err, ErrSQLTimeout):
			return resp.BadRequest(c, "查询超时（30秒）")
		case errors.Is(err, ErrEmptySQL):
			return resp.BadRequest(c, "SQL不能为空")
		default:
			log.Printf("ExplainQuery failed: %v", err)
			return resp.InternalError(c, "获取执行计划失败")
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
func (h *Handler) ListHistory(c echo.Context) error {
	userID := httpx.UserID(c)

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
func (h *Handler) DeleteHistory(c echo.Context) error {
	userID := httpx.UserID(c)

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
func (h *Handler) ClearHistory(c echo.Context) error {
	userID := httpx.UserID(c)

	if err := h.historySvc.ClearHistory(c.Request().Context(), userID); err != nil {
		log.Printf("ClearHistory failed: %v", err)
		return resp.BadRequest(c, "清空查询历史失败")
	}

	return resp.OKWithMessage(c, "已清空所有查询历史", nil)
}

type exportQueryRequest struct {
	DatasourceID int64         `json:"datasource_id"`
	Database     string        `json:"database"`
	SQL          string        `json:"sql"`
	Format       string        `json:"format"` // "csv" or "json"
	Params       []interface{} `json:"params,omitempty"`
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
func (h *Handler) ExportQuery(c echo.Context) error {
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
	if err := validateQueryParams(req.Params); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	userID := httpx.UserID(c)
	username := httpx.Username(c)
	role := httpx.Role(c)

	result, err := h.querySvc.ExportQuery(c.Request().Context(), userID, username, role, req.DatasourceID, req.Database, req.SQL, "", req.Params...)
	if err != nil {
		switch {
		case errors.Is(err, datasource.ErrDatasourceNotFound):
			return resp.BadRequest(c, "数据源不存在")
		case errors.Is(err, datasource.ErrDatasourceDisabled):
			return resp.BadRequest(c, "数据源已禁用")
		case errors.Is(err, datasource.ErrDatabaseScopeMismatch):
			// The message names both databases, so it is passed through rather
			// than replaced: the caller has to know which one is reachable.
			return resp.BadRequest(c, err.Error())
		case errors.Is(err, ErrSQLOperationForbidden):
			return resp.Forbidden(c, "该操作需要提交工单，仅允许 SELECT 查询")
		case errors.Is(err, ErrSQLHighRisk):
			return resp.Forbidden(c, "高风险操作被拦截，请提交工单")
		case errors.Is(err, ErrSQLBlocked):
			return resp.BadRequest(c, "SQL操作被拦截")
		case errors.Is(err, ErrSQLTimeout):
			return resp.BadRequest(c, "查询超时（30秒），请优化查询或缩小范围")
		case errors.Is(err, ErrEmptySQL):
			return resp.BadRequest(c, "SQL不能为空")
		case errors.Is(err, ErrExportRowLimit):
			return resp.BadRequest(c, "导出数据超过10000行上限，请添加 LIMIT 条件缩小范围")
		case errors.Is(err, ErrQueryParamsUnsupported):
			return resp.BadRequest(c, err.Error())
		case errors.Is(err, ErrInternalDatasourceOnly):
			return resp.Forbidden(c, err.Error())
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
func writeCSV(c echo.Context, result *QueryResult) error {
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
func writeExportJSON(c echo.Context, result *QueryResult) error {
	c.Response().Header().Set(echo.HeaderContentType, "application/json")
	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=export.json")
	c.Response().WriteHeader(http.StatusOK)

	return json.NewEncoder(c.Response().Writer).Encode(result.Rows)
}

// GetFrequentQueries handles GET /api/query/history/frequent.
// Returns top 20 frequent SQL templates for the current user.
func (h *Handler) GetFrequentQueries(c echo.Context) error {
	userID := httpx.UserID(c)

	results, err := h.historySvc.GetFrequentQueries(c.Request().Context(), userID)
	if err != nil {
		log.Printf("GetFrequentQueries failed: %v", err)
		return resp.BadRequest(c, err.Error())
	}
	if results == nil {
		results = []FrequentQuery{}
	}
	return resp.OK(c, results)
}
