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
