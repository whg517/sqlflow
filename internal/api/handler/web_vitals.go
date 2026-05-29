package handler

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// perIPLimiter provides a simple in-memory rate limiter keyed by IP.
type perIPLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	maxRate  int           // max requests
	window   time.Duration // sliding window
}

func newPerIPLimiter(maxRate int, window time.Duration) *perIPLimiter {
	return &perIPLimiter{
		requests: make(map[string][]time.Time),
		maxRate:  maxRate,
		window:   window,
	}
}

func (l *perIPLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Filter out old entries
	times := l.requests[ip]
	filtered := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}

	if len(filtered) >= l.maxRate {
		l.requests[ip] = filtered
		return false
	}

	l.requests[ip] = append(filtered, now)
	return true
}

// WebVitalsHandler handles Core Web Vitals metric ingestion.
type WebVitalsHandler struct {
	vitalsSvc *service.WebVitalsService
	limiter   *perIPLimiter
}

// NewWebVitalsHandler creates a new WebVitalsHandler.
func NewWebVitalsHandler(vitalsSvc *service.WebVitalsService) *WebVitalsHandler {
	return &WebVitalsHandler{
		vitalsSvc: vitalsSvc,
		// 10 requests per IP per minute — more than enough for real user traffic
		limiter: newPerIPLimiter(10, time.Minute),
	}
}

type webVitalPayload struct {
	Name           string  `json:"name"`
	Value          float64 `json:"value"`
	Rating         string  `json:"rating"`
	Path           string  `json:"path"`
	NavigationType string  `json:"navigationType"`
}

// RecordVitals handles POST /api/metrics/web-vitals.
//
// @Summary 上报 Core Web Vitals 指标
// @Description 接收前端上报的 LCP/INP/CLS 性能指标（公开端点，rate limit 保护）
// @Tags 性能指标
// @Accept json
// @Produce json
// @Param body body webVitalPayload true "指标数据"
// @Success 200 {object} resp.SuccessResponse "上报成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 429 {object} resp.ErrorResponse "请求过于频繁"
// @Router /metrics/web-vitals [post]
func (h *WebVitalsHandler) RecordVitals(c echo.Context) error {
	// Rate limit by IP
	ip := c.RealIP()
	if !h.limiter.allow(ip) {
		return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
			"code":    429,
			"message": "请求过于频繁，请稍后重试",
		})
	}

	var req webVitalPayload
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	// Validate metric name
	validNames := map[string]bool{"LCP": true, "INP": true, "CLS": true}
	if !validNames[req.Name] {
		return resp.BadRequest(c, "不支持的指标名称")
	}

	if req.Value < 0 {
		return resp.BadRequest(c, "指标值不能为负")
	}

	validRatings := map[string]bool{"good": true, "needs-improvement": true, "poor": true}
	if req.Rating != "" && !validRatings[req.Rating] {
		return resp.BadRequest(c, "无效的 rating 值")
	}

	metric := &model.WebVital{
		MetricName:     req.Name,
		Value:          req.Value,
		Rating:         req.Rating,
		Path:           req.Path,
		NavigationType: req.NavigationType,
		UserAgent:      c.Request().UserAgent(),
	}

	if err := h.vitalsSvc.RecordMetric(c.Request().Context(), metric); err != nil {
		log.Printf("RecordVitals failed: %v", err)
		return resp.InternalError(c, "记录指标失败")
	}

	return resp.OKWithMessage(c, "上报成功", nil)
}
