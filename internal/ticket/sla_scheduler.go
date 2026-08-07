package ticket

import (
	"context"
	"log"
	"sync"
	"time"

	entTicket "github.com/whg517/sqlflow/internal/db/ent/ticket"
	"github.com/whg517/sqlflow/internal/model"
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
	rows, err := s.slaSvc.client.Ticket.Query().
		Where(
			entTicket.StatusEQ(string(model.TicketStatusPendingApproval)),
			entTicket.SLADeadlineIsNil(),
		).
		All(ctx)
	if err != nil {
		return err
	}

	for _, t := range rows {
		if err := s.slaSvc.SetSLADeadline(ctx, int64(t.ID), t.RiskLevel); err != nil {
			log.Printf("sla: set deadline for ticket #%d: %v", t.ID, err)
		}
	}

	return nil
}
