package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// PerformanceHandler handles performance analysis requests.
type PerformanceHandler struct {
	historySvc *service.QueryHistoryService
}

// NewPerformanceHandler creates a new PerformanceHandler.
func NewPerformanceHandler(historySvc *service.QueryHistoryService) *PerformanceHandler {
	return &PerformanceHandler{historySvc: historySvc}
}

// ListSlowQueries handles GET /api/query/performance/slow.
func (h *PerformanceHandler) ListSlowQueries(c echo.Context) error {
	threshold, _ := strconv.ParseInt(c.QueryParam("threshold"), 10, 64)
	if threshold <= 0 {
		threshold = 1000
	}

	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	datasourceID, _ := strconv.ParseInt(c.QueryParam("datasource_id"), 10, 64)

	params := service.SlowQueryParams{
		Threshold:    threshold,
		Page:         page,
		PageSize:     pageSize,
		DatasourceID: datasourceID,
		StartDate:    c.QueryParam("start_date"),
		EndDate:      c.QueryParam("end_date"),
	}

	list, total, err := h.historySvc.ListSlowQueries(c.Request().Context(), params)
	if err != nil {
		log.Printf("ListSlowQueries failed: %v", err)
		return resp.InternalError(c, "获取慢查询列表失败")
	}

	if list == nil {
		list = make([]model.QueryHistory, 0)
	}

	return resp.OKPage(c, list, int64(params.Page), int64(params.PageSize), int64(total))
}

// GetPerformanceStats handles GET /api/query/performance/stats.
func (h *PerformanceHandler) GetPerformanceStats(c echo.Context) error {
	days, _ := strconv.Atoi(c.QueryParam("days"))
	if days <= 0 {
		days = 7
	}

	stats, err := h.historySvc.GetPerformanceStats(c.Request().Context(), days)
	if err != nil {
		log.Printf("GetPerformanceStats failed: %v", err)
		return resp.InternalError(c, "获取性能统计失败")
	}

	return resp.OK(c, stats)
}
