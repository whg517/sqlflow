package service

import (
	"database/sql"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
)

func setupVitalsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return database.DB
}

func TestWebVitalsService_RecordMetric(t *testing.T) {
	conn := setupVitalsTestDB(t)
	svc := NewWebVitalsService(conn)

	m := &model.WebVital{
		MetricName:     "LCP",
		Value:          1234,
		Rating:         "needs-improvement",
		Path:           "/query",
		NavigationType: "reload",
		UserAgent:      "Mozilla/5.0",
	}

	err := svc.RecordMetric(t.Context(), m)
	if err != nil {
		t.Fatalf("RecordMetric: %v", err)
	}

	// Verify by querying directly
	var count int
	conn.QueryRow("SELECT COUNT(*) FROM web_vitals WHERE metric_name = 'LCP'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 LCP record, got %d", count)
	}
}

func TestWebVitalsService_RecordBatch(t *testing.T) {
	conn := setupVitalsTestDB(t)
	svc := NewWebVitalsService(conn)

	metrics := []model.WebVital{
		{MetricName: "LCP", Value: 1200, Rating: "good", Path: "/", NavigationType: "navigate"},
		{MetricName: "INP", Value: 150, Rating: "good", Path: "/", NavigationType: "navigate"},
		{MetricName: "CLS", Value: 0.05, Rating: "good", Path: "/query", NavigationType: "reload"},
	}

	err := svc.RecordBatch(t.Context(), metrics)
	if err != nil {
		t.Fatalf("RecordBatch: %v", err)
	}

	var count int
	conn.QueryRow("SELECT COUNT(*) FROM web_vitals").Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 records, got %d", count)
	}
}

func TestWebVitalsService_CleanupOld(t *testing.T) {
	conn := setupVitalsTestDB(t)
	svc := NewWebVitalsService(conn)

	// Insert a metric with old timestamp (31 days ago)
	oldTime := time.Now().AddDate(0, 0, -31).Format("2006-01-02 15:04:05")
	_, err := conn.Exec(
		`INSERT INTO web_vitals (metric_name, value, rating, created_at) VALUES ('LCP', 1000, 'good', ?)`,
		oldTime,
	)
	if err != nil {
		t.Fatalf("insert old record: %v", err)
	}

	// Insert a recent metric
	err = svc.RecordMetric(t.Context(), &model.WebVital{
		MetricName: "LCP", Value: 500, Rating: "good",
	})
	if err != nil {
		t.Fatalf("RecordMetric: %v", err)
	}

	n, err := svc.CleanupOld(t.Context())
	if err != nil {
		t.Fatalf("CleanupOld: %v", err)
	}
	if n != 1 {
		t.Errorf("expected to clean 1 record, got %d", n)
	}

	// Verify recent record still exists
	var count int
	conn.QueryRow("SELECT COUNT(*) FROM web_vitals").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 remaining record, got %d", count)
	}
}
