package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/platform/metrics"
)

// datasourcePingTimeout bounds the whole datasource sweep. A readiness probe
// that waits on an unresponsive target is itself a availability problem.
const datasourcePingTimeout = 2 * time.Second

// HealthHandler handles health check and metrics endpoints.
type HealthHandler struct {
	db      *sql.DB
	started time.Time
	version string

	// Optional dependencies for readiness checks
	poolMgr *driver.PoolManager
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

// SetPoolManager sets the driver PoolManager for readiness checks.
func (h *HealthHandler) SetPoolManager(pm *driver.PoolManager) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.poolMgr = pm
}

// HealthResponse is the JSON response for /health and /healthz.
type HealthResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Uptime  int64             `json:"uptime"`
	DB      string            `json:"db,omitempty"`
	Checks  map[string]string `json:"checks,omitempty"`
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
// @Description Readiness probe — gates on the platform PostgreSQL; pooled datasource connections are reported but do not affect readiness
// @Tags 健康
// @Produce json
// @Success 200 {object} HealthResponse "就绪"
// @Failure 503 {object} HealthResponse "未就绪"
// @Router /readyz [get]

// Readyz checks all dependencies and returns 503 if any are unhealthy.
func (h *HealthHandler) Readyz(c echo.Context) error {
	checks := make(map[string]string)
	allOK := true

	// The platform store is the one hard dependency: without it this instance
	// can serve nothing. The label used to read "sqlite", left over from before
	// ADR-0009 moved the metadata to PostgreSQL.
	if err := h.db.Ping(); err != nil {
		checks["platform_db"] = "error: " + err.Error()
		allOK = false
	} else {
		checks["platform_db"] = "ok"
	}

	// Governed datasources are reported, not gated on.
	//
	// This used to call connpool.Manager.HealthCheck, which walks connection
	// maps that no production code fills, and then counted driver-pool entries
	// without touching any of them — so every governed database could be
	// unreachable and the probe still answered "ok (3 connections)".
	//
	// The result stays out of allOK deliberately. A target database being down
	// is not this instance being unready: it can still serve tickets, audit and
	// administration, and failing readiness would pull the whole instance out of
	// rotation over someone else's outage.
	h.mu.RLock()
	pm := h.poolMgr
	h.mu.RUnlock()

	if pm != nil {
		pingCtx, cancel := context.WithTimeout(c.Request().Context(), datasourcePingTimeout)
		defer cancel()
		for _, id := range pm.ManagedIDs() {
			d := pm.GetCached(id)
			if d == nil {
				continue
			}
			name := fmt.Sprintf("datasource_%d", id)
			if err := d.Ping(pingCtx); err != nil {
				checks[name] = "error: " + err.Error()
			} else {
				checks[name] = "ok"
			}
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
