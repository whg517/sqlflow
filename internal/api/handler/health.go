package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/pkg/metrics"
)

// HealthHandler handles health check and metrics endpoints.
type HealthHandler struct {
	db      *sql.DB
	started time.Time
	version string
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{
		db:      db,
		started: time.Now(),
		version: "1.0.0",
	}
}

// HealthResponse is the JSON response for /health.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  int64  `json:"uptime"`
	DB      string `json:"db"`
}

// Health godoc
// @Summary 健康检查
// @Description 返回服务健康状态和数据库连通性
// @Tags 健康
// @Produce json
// @Success 200 {object} HealthResponse "健康"
// @Failure 503 {object} HealthResponse "不健康"
// @Router /health [get]

// Health returns the health status of the service.
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

// Metrics exposes Prometheus metrics via the promhttp handler adapter.
// Metrics godoc
// @Summary Prometheus 指标
// @Description 返回 Prometheus 格式的指标数据
// @Tags 健康
// @Produce plain
// @Router /metrics [get]

func (h *HealthHandler) Metrics(c echo.Context) error {
	// Import promhttp lazily to avoid build tag issues
	// The actual handler is registered directly in the router.
	// This method serves as a thin wrapper.
	metricsHandler := metrics.PromhttpHandler()
	metricsHandler.ServeHTTP(c.Response(), c.Request())
	return nil
}
