package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace = "sqlflow"
)

var (
	// HTTPRequestDuration tracks API response latency in seconds.
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "http_request_duration_seconds",
		Help:      "API request duration in seconds",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"method", "path", "status"})

	// HTTPRequestsTotal counts total HTTP requests.
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "http_requests_total",
		Help:      "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	// ActiveTickets tracks the current number of active (non-terminal) tickets.
	ActiveTickets = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "active_tickets",
		Help:      "Current number of active SQL review tickets",
	})

	// DBQueriesTotal counts total database query executions.
	DBQueriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "db_queries_total",
		Help:      "Total number of database queries executed",
	}, []string{"datasource"})

	// TicketsTotal tracks ticket counts by status.
	TicketsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "tickets_total",
		Help:      "Number of tickets by status",
	}, []string{"status"})

	// ActiveDatasources tracks the current number of active datasources.
	ActiveDatasources = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "active_datasources",
		Help:      "Current number of active datasources",
	})

	// DBQueryDuration tracks external datasource query latency in seconds.
	DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "db_query_duration_seconds",
		Help:      "External datasource query duration in seconds",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"datasource_type"})
)

// Middleware returns an Echo middleware that records Prometheus metrics for each request.
func Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			duration := time.Since(start).Seconds()

			status := fmt.Sprintf("%d", c.Response().Status)
			path := c.Path()
			if path == "" {
				path = c.Request().URL.Path
			}

			HTTPRequestDuration.WithLabelValues(c.Request().Method, path, status).Observe(duration)
			HTTPRequestsTotal.WithLabelValues(c.Request().Method, path, status).Inc()

			return err
		}
	}
}

// PromhttpHandler returns the standard Prometheus metrics HTTP handler.
func PromhttpHandler() http.Handler {
	return promhttp.Handler()
}
