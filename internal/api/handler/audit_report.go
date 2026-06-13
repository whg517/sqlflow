package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// AuditReportHandler handles audit report requests.
type AuditReportHandler struct {
	reportSvc *service.AuditReportService
}

// NewAuditReportHandler creates a new AuditReportHandler.
func NewAuditReportHandler(reportSvc *service.AuditReportService) *AuditReportHandler {
	return &AuditReportHandler{reportSvc: reportSvc}
}

// GetUsageStats handles GET /api/reports/usage.
//
// @Summary 使用统计报表
// @Description 获取审计日志使用统计数据（操作总量、活跃用户、Top用户等）
// @Tags 报表
// @Produce json
// @Security BearerAuth
// @Param days query int false "统计天数" default(7)
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "查询失败"
// @Router /reports/usage [get]
func (h *AuditReportHandler) GetUsageStats(c echo.Context) error {
	days, _ := strconv.Atoi(c.QueryParam("days"))

	stats, err := h.reportSvc.GetUsageStats(c.Request().Context(), service.ReportParams{Days: days})
	if err != nil {
		log.Printf("GetUsageStats failed: %v", err)
		return resp.InternalError(c, "获取使用统计失败")
	}

	return resp.OK(c, stats)
}

// GetErrorStats handles GET /api/reports/errors.
//
// @Summary 错误分析报表
// @Description 获取审计日志错误分析数据（错误率、错误类型分布等）
// @Tags 报表
// @Produce json
// @Security BearerAuth
// @Param days query int false "统计天数" default(7)
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "查询失败"
// @Router /reports/errors [get]
func (h *AuditReportHandler) GetErrorStats(c echo.Context) error {
	days, _ := strconv.Atoi(c.QueryParam("days"))

	stats, err := h.reportSvc.GetErrorStats(c.Request().Context(), service.ReportParams{Days: days})
	if err != nil {
		log.Printf("GetErrorStats failed: %v", err)
		return resp.InternalError(c, "获取错误统计失败")
	}

	return resp.OK(c, stats)
}

// GetPerformanceReport handles GET /api/reports/performance.
//
// @Summary 性能趋势报表
// @Description 获取审计日志性能指标（平均耗时、P95、每日趋势等）
// @Tags 报表
// @Produce json
// @Security BearerAuth
// @Param days query int false "统计天数" default(7)
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "查询失败"
// @Router /reports/performance [get]
func (h *AuditReportHandler) GetPerformanceReport(c echo.Context) error {
	days, _ := strconv.Atoi(c.QueryParam("days"))

	stats, err := h.reportSvc.GetPerformanceReport(c.Request().Context(), service.ReportParams{Days: days})
	if err != nil {
		log.Printf("GetPerformanceReport failed: %v", err)
		return resp.InternalError(c, "获取性能报表失败")
	}

	return resp.OK(c, stats)
}

// GetTicketReport handles GET /api/reports/tickets.
//
// @Summary 工单统计报表
// @Description 获取工单流程统计数据（各状态数量、平均审批时间、风险分布等）
// @Tags 报表
// @Produce json
// @Security BearerAuth
// @Param days query int false "统计天数" default(7)
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "查询失败"
// @Router /reports/tickets [get]
func (h *AuditReportHandler) GetTicketReport(c echo.Context) error {
	days, _ := strconv.Atoi(c.QueryParam("days"))

	stats, err := h.reportSvc.GetTicketReport(c.Request().Context(), service.ReportParams{Days: days})
	if err != nil {
		log.Printf("GetTicketReport failed: %v", err)
		return resp.InternalError(c, "获取工单统计失败")
	}

	return resp.OK(c, stats)
}

// GetUserAnalytics handles GET /api/audit/user-analytics.
//
// @Summary 用户行为分析
// @Description 获取用户行为分析聚合数据（活跃度 TOP 10、查询频率、操作类型占比、异常行为）
// @Tags 审计
// @Produce json
// @Security BearerAuth
// @Param time_range query string false "时间范围 (7d/30d/90d/custom)" default(7d)
// @Param start_date query string false "自定义开始日期 (YYYY-MM-DD，time_range=custom 时必填)"
// @Param end_date query string false "自定义结束日期 (YYYY-MM-DD，time_range=custom 时必填)"
// @Param user_id query int false "特定用户 ID（下钻，必须为正整数）"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 500 {object} resp.ErrorResponse "查询失败"
// @Router /audit/user-analytics [get]
func (h *AuditReportHandler) GetUserAnalytics(c echo.Context) error {
	timeRange := c.QueryParam("time_range")
	if timeRange == "" {
		timeRange = "7d"
	}

	userID, err := service.ParseAnalyticsUserID(c.QueryParam("user_id"))
	if err != nil {
		return resp.BadRequest(c, err.Error())
	}

	params := service.AnalyticsParams{
		TimeRange: timeRange,
		StartDate: c.QueryParam("start_date"),
		EndDate:   c.QueryParam("end_date"),
		UserID:    userID,
	}

	analytics, err := h.reportSvc.GetUserAnalytics(c.Request().Context(), params)
	if err != nil {
		log.Printf("GetUserAnalytics failed: %v", err)
		return resp.BadRequest(c, err.Error())
	}

	return resp.OK(c, analytics)
}
