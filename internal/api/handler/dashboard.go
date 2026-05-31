package handler

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// DashboardHandler handles dashboard related requests.
type DashboardHandler struct {
	dashboardSvc *service.DashboardService
}

// NewDashboardHandler creates a new DashboardHandler.
func NewDashboardHandler(dashboardSvc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{dashboardSvc: dashboardSvc}
}

// GetStats handles GET /api/dashboard/stats.
//
// @Summary 获取仪表盘统计
// @Description 获取待处理工单数、近7天查询数、活跃数据源数、总用户数等统计信息
// @Tags 仪表盘
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse{data=service.DashboardStats} "成功"
// @Failure 500 {object} resp.ErrorResponse "获取统计数据失败"
// @Router /dashboard/stats [get]
func (h *DashboardHandler) GetStats(c echo.Context) error {
	stats, err := h.dashboardSvc.GetStats(c.Request().Context())
	if err != nil {
		log.Printf("GetStats failed: %v", err)
		return resp.InternalError(c, "获取统计数据失败")
	}
	return resp.OK(c, stats)
}

// GetOverview handles GET /api/dashboard/overview.
//
// @Summary 获取仪表盘看板数据
// @Description 获取看板完整数据：统计卡片、查询趋势、工单状态分布、最近活动流
// @Tags 仪表盘
// @Produce json
// @Security BearerAuth
// @Param range query string false "时间范围: today|this_week|this_month|last_30d" Enums(today,this_week,this_month,last_30d)
// @Success 200 {object} resp.SuccessResponse{data=service.DashboardOverview} "成功"
// @Failure 500 {object} resp.ErrorResponse "获取看板数据失败"
// @Router /dashboard/overview [get]
func (h *DashboardHandler) GetOverview(c echo.Context) error {
	tr := service.TimeRange(c.QueryParam("range"))
	if tr == "" {
		tr = service.TimeRangeLast30d
	}
	// Validate time range
	switch tr {
	case service.TimeRangeToday, service.TimeRangeThisWeek, service.TimeRangeThisMonth, service.TimeRangeLast30d:
		// valid
	default:
		tr = service.TimeRangeLast30d
	}

	overview, err := h.dashboardSvc.GetOverview(c.Request().Context(), tr)
	if err != nil {
		log.Printf("GetOverview failed: %v", err)
		return resp.InternalError(c, "获取看板数据失败")
	}
	return resp.OK(c, overview)
}
