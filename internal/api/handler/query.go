package handler

import (
	"strconv"

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
	SQLContent   string `json:"sql_content"`
	DBType       string `json:"db_type"`
}

// ExecuteQuery handles POST /api/query/execute.
func (h *QueryHandler) ExecuteQuery(c echo.Context) error {
	var req executeQueryRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.DatasourceID == 0 {
		return resp.BadRequest(c, "数据源ID不能为空")
	}
	if req.SQLContent == "" {
		return resp.BadRequest(c, "SQL不能为空")
	}
	if req.DBType == "" {
		req.DBType = "mysql"
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)

	result, err := h.querySvc.ExecuteQuery(userID, req.DatasourceID, req.Database, req.SQLContent, req.DBType)
	if err != nil {
		switch err {
		case service.ErrSQLOperationForbidden:
			return resp.Forbidden(c, "该操作需要提交工单，仅允许 SELECT 查询")
		case service.ErrSQLHighRisk:
			return resp.Forbidden(c, "高风险操作被拦截，请提交工单")
		case service.ErrSQLTimeout:
			return resp.BadRequest(c, "查询超时（30秒），请优化查询或缩小范围")
		case service.ErrEmptySQL:
			return resp.BadRequest(c, "SQL不能为空")
		default:
			return resp.InternalError(c, err.Error())
		}
	}

	return resp.OK(c, result)
}

// ListHistory handles GET /api/query/history.
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

	list, total, err := h.historySvc.ListHistory(userID, page, pageSize)
	if err != nil {
		return resp.InternalError(c, "获取查询历史失败")
	}

	if list == nil {
		list = make([]model.QueryHistory, 0)
	}

	return resp.OKPage(c, list, int64(page), int64(pageSize), int64(total))
}

// DeleteHistory handles DELETE /api/query/history/:id.
func (h *QueryHandler) DeleteHistory(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的历史记录ID")
	}

	if err := h.historySvc.DeleteHistory(id, userID); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	return resp.OKWithMessage(c, "删除成功", nil)
}

// ClearHistory handles DELETE /api/query/history.
func (h *QueryHandler) ClearHistory(c echo.Context) error {
	userID := c.Get(middleware.ContextKeyUserID).(int64)

	if err := h.historySvc.ClearHistory(userID); err != nil {
		return resp.InternalError(c, "清空查询历史失败")
	}

	return resp.OKWithMessage(c, "已清空所有查询历史", nil)
}
