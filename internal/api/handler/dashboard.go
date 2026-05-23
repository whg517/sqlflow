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
func (h *DashboardHandler) GetStats(c echo.Context) error {
	stats, err := h.dashboardSvc.GetStats(c.Request().Context())
	if err != nil {
		log.Printf("GetStats failed: %v", err)
		return resp.InternalError(c, "获取统计数据失败")
	}
	return resp.OK(c, stats)
}
