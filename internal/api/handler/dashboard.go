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
// @Summary 获取仪表盘统计（简版）
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

// GetFullStats handles GET /api/dashboard/full-stats.
//
// @Summary 获取仪表盘完整数据
// @Description 一次性返回看板所需的所有统计数据，包括趋势、工单分布、最近活动等
// @Tags 仪表盘
// @Produce json
// @Param start_date query string false "查询趋势开始日期 (YYYY-MM-DD, 默认7天前)"
// @Param end_date query string false "查询趋势结束日期 (YYYY-MM-DD, 默认今天)"
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse{data=service.DashboardFullStats} "成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 500 {object} resp.ErrorResponse "获取统计数据失败"
// @Router /dashboard/full-stats [get]
func (h *DashboardHandler) GetFullStats(c echo.Context) error {
	startDate := c.QueryParam("start_date")
	endDate := c.QueryParam("end_date")

	stats, err := h.dashboardSvc.GetFullStats(c.Request().Context(), startDate, endDate)
	if err != nil {
		log.Printf("GetFullStats failed: %v", err)
		return resp.BadRequest(c, err.Error())
	}
	return resp.OK(c, stats)
}
