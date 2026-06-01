package handler

import (
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/pkg/metrics"
)

// HealthHandler handles health check and metrics endpoints.
type HealthHandler struct {
	db      *sql.DB
	started time.Time
	version string

	// Optional dependencies for readiness checks
	connMgr *connpool.Manager
	mu      sync.RWMutex
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{
		db:      db,
		started: time.Now(),
		version: "1.0.0",
	}
}

// SetConnPoolManager sets the connection pool manager for readiness checks.
func (h *HealthHandler) SetConnPoolManager(mgr *connpool.Manager) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connMgr = mgr
}

// HealthResponse is the JSON response for /health and /healthz.
type HealthResponse struct {
	Status    string            `json:"status"`
	Version   string            `json:"version"`
	Uptime    int64             `json:"uptime"`
	DB        string            `json:"db,omitempty"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// Health godoc
// @Summary 健康检查
// @Description 返回服务健康状态和数据库连通性
// @Tags 健康
// @Produce json
// @Success 200 {object} HealthResponse "健康"
// @Failure 503 {object} HealthResponse "不健康"
// @Router /health [get]

// Health returns the health status of the service (legacy, backward compatible).
func (h *HealthHandler) Health(c echo.Context) error {
	dbStatus := "ok"
	if err := h.db.Ping(); err != nil {
		dbStatus = "error"
	}

	resp := HealthResponse{
		Status:  "ok",
		Version: h.version,
		Uptime:  int64(time.Since(h.started).Seconds()),
		DB:      dbStatus,
	}

	if dbStatus != "ok" {
		return c.JSON(http.StatusServiceUnavailable, resp)
	}
	return c.JSON(http.StatusOK, resp)
}

// Healthz godoc
// @Summary 存活探针
// @Description Liveness probe — returns 200 if the process is alive. No dependency checks.
// @Tags 健康
// @Produce json
// @Success 200 {object} HealthResponse "存活"
// @Router /healthz [get]

// Healthz is a lightweight liveness probe that only checks process alive status.
func (h *HealthHandler) Healthz(c echo.Context) error {
	return c.JSON(http.StatusOK, HealthResponse{
		Status:  "ok",
		Version: h.version,
		Uptime:  int64(time.Since(h.started).Seconds()),
	})
}

// Readyz godoc
// @Summary 就绪探针
// @Description Readiness probe — checks SQLite, optional Redis, and external datasource connection pools
// @Tags 健康
// @Produce json
// @Success 200 {object} HealthResponse "就绪"
// @Failure 503 {object} HealthResponse "未就绪"
// @Router /readyz [get]

// Readyz checks all dependencies and returns 503 if any are unhealthy.
func (h *HealthHandler) Readyz(c echo.Context) error {
	checks := make(map[string]string)
	allOK := true

	// 1. SQLite check
	if err := h.db.Ping(); err != nil {
		checks["sqlite"] = "error: " + err.Error()
		allOK = false
	} else {
		checks["sqlite"] = "ok"
	}

	// 2. Connection pool manager check (external datasources)
	h.mu.RLock()
	mgr := h.connMgr
	h.mu.RUnlock()

	if mgr != nil {
		if err := mgr.HealthCheck(); err != nil {
			checks["datasources"] = "error: " + err.Error()
			allOK = false
		} else {
			checks["datasources"] = "ok"
		}
	}

	resp := HealthResponse{
		Status:  "ok",
		Version: h.version,
		Uptime:  int64(time.Since(h.started).Seconds()),
		Checks:  checks,
	}

	if !allOK {
		resp.Status = "degraded"
		return c.JSON(http.StatusServiceUnavailable, resp)
	}
	return c.JSON(http.StatusOK, resp)
}

// Metrics exposes Prometheus metrics via the promhttp handler adapter.
// Metrics godoc
// @Summary Prometheus 指标
// @Description 返回 Prometheus 格式的指标数据
// @Tags 健康
// @Produce plain
// @Router /metrics [get]

func (h *HealthHandler) Metrics(c echo.Context) error {
	metricsHandler := metrics.PromhttpHandler()
	metricsHandler.ServeHTTP(c.Response(), c.Request())
	return nil
}
