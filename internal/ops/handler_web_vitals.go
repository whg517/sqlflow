package ops

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/resp"
)

// maxWebVitalsBody caps the request body size. A real Web Vitals payload is a
// few hundred bytes of JSON; anything larger is either broken or hostile.
const maxWebVitalsBody = 4 << 10 // 4 KiB

// perIPLimiter provides a simple in-memory rate limiter keyed by IP.
type perIPLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	maxRate  int           // max requests
	window   time.Duration // sliding window
	lastGC   time.Time     // last garbage-collection sweep
	gcEvery  time.Duration // how often to evict stale keys
}

func newPerIPLimiter(maxRate int, window time.Duration) *perIPLimiter {
	return &perIPLimiter{
		requests: make(map[string][]time.Time),
		maxRate:  maxRate,
		window:   window,
		gcEvery:  5 * time.Minute,
		lastGC:   time.Now(),
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

	// Periodically evict keys whose entries have all expired, so the map
	// does not grow without bound when an attacker rotates through forged
	// source IPs. This is bounded by gcEvery, not by request volume.
	if now.Sub(l.lastGC) > l.gcEvery {
		l.gcLocked(now, cutoff)
		l.lastGC = now
	}

	return true
}

// gcLocked removes entries whose timestamps have all fallen outside the
// window. Caller must hold l.mu.
func (l *perIPLimiter) gcLocked(now, cutoff time.Time) {
	for ip, times := range l.requests {
		stillValid := false
		for _, t := range times {
			if t.After(cutoff) {
				stillValid = true
				break
			}
		}
		if !stillValid {
			delete(l.requests, ip)
		}
	}
}

// clientIP returns the remote address from the actual TCP connection, NOT
// from X-Forwarded-For. A caller behind no trusted proxy can forge that
// header at will, which would make per-IP rate limiting meaningless and the
// limiter's key space unbounded (every forged header is a new key).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// WebVitalsHandler handles Core Web Vitals metric ingestion.
type WebVitalsHandler struct {
	vitalsSvc *WebVitalsService
	limiter   *perIPLimiter
}

// NewWebVitalsHandler creates a new WebVitalsHandler.
func NewWebVitalsHandler(vitalsSvc *WebVitalsService) *WebVitalsHandler {
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
	// Bound the body before reading it — a hostile or buggy client can stream
	// gigabytes into c.Bind, which would consume server memory until OOM.
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxWebVitalsBody)

	// Rate limit by the actual TCP source, not a forgeable header.
	ip := clientIP(c.Request())
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
