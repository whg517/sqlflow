package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entVital "github.com/whg517/sqlflow/internal/db/ent/webvital"
	"github.com/whg517/sqlflow/internal/model"
)

const webVitalsRetentionDays = 30

// WebVitalsService handles Core Web Vitals metric ingestion.
type WebVitalsService struct {
	database *db.DB
	client   *ent.Client
}

// NewWebVitalsService creates a new WebVitalsService.
func NewWebVitalsService(database *db.DB) *WebVitalsService {
	return &WebVitalsService{database: database, client: database.Client()}
}

// RecordMetric stores a single web vital metric.
func (s *WebVitalsService) RecordMetric(ctx context.Context, m *model.WebVital) error {
	_, err := s.client.WebVital.Create().
		SetMetricName(m.MetricName).
		SetValue(m.Value).
		SetRating(m.Rating).
		SetPath(m.Path).
		SetNavigationType(m.NavigationType).
		SetUserAgent(m.UserAgent).
		Save(ctx)
	if err != nil {
		log.Printf("RecordMetric failed: %v", err)
		return fmt.Errorf("记录指标失败")
	}
	return nil
}

// RecordBatch stores multiple web vital metrics in a single transaction.
func (s *WebVitalsService) RecordBatch(ctx context.Context, metrics []model.WebVital) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i := range metrics {
		_, err := tx.WebVital.Create().
			SetMetricName(metrics[i].MetricName).
			SetValue(metrics[i].Value).
			SetRating(metrics[i].Rating).
			SetPath(metrics[i].Path).
			SetNavigationType(metrics[i].NavigationType).
			SetUserAgent(metrics[i].UserAgent).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("insert metric %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// CleanupOld removes metrics older than 30 days.
func (s *WebVitalsService) CleanupOld(ctx context.Context) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -webVitalsRetentionDays)
	n, err := s.client.WebVital.Delete().
		Where(entVital.CreatedAtLT(cutoff)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("cleanup old metrics: %w", err)
	}
	return int64(n), nil
}
