package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

const webVitalsRetentionDays = 30

// WebVitalsService handles Core Web Vitals metric ingestion.
type WebVitalsService struct {
	db *sql.DB
}

// NewWebVitalsService creates a new WebVitalsService.
func NewWebVitalsService(db *sql.DB) *WebVitalsService {
	return &WebVitalsService{db: db}
}

// RecordMetric stores a single web vital metric.
func (s *WebVitalsService) RecordMetric(ctx context.Context, m *model.WebVital) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO web_vitals (metric_name, value, rating, path, navigation_type, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		m.MetricName, m.Value, m.Rating, m.Path, m.NavigationType, m.UserAgent,
	)
	if err != nil {
		log.Printf("RecordMetric failed: %v", err)
		return fmt.Errorf("记录指标失败")
	}
	return nil
}

// RecordBatch stores multiple web vital metrics in a single transaction.
func (s *WebVitalsService) RecordBatch(ctx context.Context, metrics []model.WebVital) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO web_vitals (metric_name, value, rating, path, navigation_type, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for i := range metrics {
		_, err := stmt.ExecContext(ctx,
			metrics[i].MetricName, metrics[i].Value, metrics[i].Rating,
			metrics[i].Path, metrics[i].NavigationType, metrics[i].UserAgent,
		)
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
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM web_vitals WHERE created_at < ?`, cutoff.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup old metrics: %w", err)
	}
	return result.RowsAffected()
}
