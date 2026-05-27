package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// SLAScheduler periodically checks tickets for SLA violations and sends alerts.
type SLAScheduler struct {
	slaSvc   *SLAService
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
	lastRun  time.Time
}

// NewSLAScheduler creates a new SLA scheduler that checks every interval.
func NewSLAScheduler(slaSvc *SLAService, interval time.Duration) *SLAScheduler {
	return &SLAScheduler{
		slaSvc:   slaSvc,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the SLA scheduler goroutine.
func (s *SLAScheduler) Start() {
	s.wg.Add(1)
	go s.loop()
	log.Printf("SLA scheduler started (interval=%s)", s.interval)
}

// Stop gracefully stops the SLA scheduler.
func (s *SLAScheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	log.Println("SLA scheduler stopped")
}

// RunOnce performs a single SLA check. Useful for testing.
func (s *SLAScheduler) RunOnce(ctx context.Context) error {
	return s.slaSvc.CheckSLA(ctx)
}

func (s *SLAScheduler) loop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

			if err := s.slaSvc.CheckSLA(ctx); err != nil {
				log.Printf("sla scheduler: check failed: %v", err)
			}

			// Also set deadlines for tickets that don't have one yet
			if err := s.setMissingDeadlines(ctx); err != nil {
				log.Printf("sla scheduler: set deadlines failed: %v", err)
			}

			s.lastRun = time.Now()

			// Health check: log warning if previous gap was too large
			if !s.lastRun.IsZero() {
				log.Printf("sla scheduler: check completed at %s", s.lastRun.Format("2006-01-02 15:04:05"))
			}

			cancel()
		}
	}
}

// setMissingDeadlines sets SLA deadlines for PENDING_APPROVAL tickets that don't have one.
func (s *SLAScheduler) setMissingDeadlines(ctx context.Context) error {
	rows, err := s.slaSvc.db.QueryContext(ctx,
		`SELECT id, risk_level FROM tickets WHERE status = 'PENDING_APPROVAL' AND sla_deadline IS NULL`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int64
		var riskLevel string
		if err := rows.Scan(&id, &riskLevel); err != nil {
			log.Printf("sla: scan missing deadline ticket: %v", err)
			continue
		}
		if err := s.slaSvc.SetSLADeadline(ctx, id, riskLevel); err != nil {
			log.Printf("sla: set deadline for ticket #%d: %v", id, err)
		}
	}
	return rows.Err()
}
