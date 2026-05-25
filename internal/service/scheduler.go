package service

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// Scheduler periodically checks for scheduled tickets whose scheduled_at
// has passed and automatically executes them.
type Scheduler struct {
	ticketSvc *TicketService
	interval  time.Duration
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewScheduler creates a new Scheduler that checks every interval.
func NewScheduler(ticketSvc *TicketService, interval time.Duration) *Scheduler {
	return &Scheduler{
		ticketSvc: ticketSvc,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

// Start launches the scheduler goroutine. Call Stop to terminate it.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.loop()
	log.Printf("ticket scheduler started (interval=%s)", s.interval)
}

// Stop gracefully stops the scheduler, waiting for the loop to exit.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	log.Println("ticket scheduler stopped")
}

// RunOnce performs a single check for due scheduled tickets.
// This is useful for testing and for the scheduler loop.
func (s *Scheduler) RunOnce(ctx context.Context) error {
	return s.executeDueTickets(ctx)
}

func (s *Scheduler) loop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := s.executeDueTickets(ctx); err != nil {
				log.Printf("scheduler: execute due tickets failed: %v", err)
			}
			cancel()
		}
	}
}

func (s *Scheduler) executeDueTickets(ctx context.Context) error {
	now := time.Now()

	// Find all SCHEDULED tickets whose scheduled_at <= now
	rows, err := s.ticketSvc.db.QueryContext(ctx,
		`SELECT id, submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, ai_review_result, reviewer_id, review_comment, scheduled_at, executed_at, created_at, updated_at
		 FROM tickets
		 WHERE status = ? AND scheduled_at IS NOT NULL AND scheduled_at <= ?
		 ORDER BY scheduled_at ASC`,
		model.TicketStatusScheduled, now,
	)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			log.Printf("scheduler: scan ticket failed: %v", err)
			continue
		}

		log.Printf("scheduler: executing scheduled ticket #%d (scheduled_at=%s)",
			t.ID, t.ScheduledAt.Format("2006-01-02 15:04:05"))

		// Execute as system (operatorID=0, role="system")
		// Skip permission check since this is an automated execution
		if err := s.executeScheduledTicket(ctx, t); err != nil {
			log.Printf("scheduler: failed to execute ticket #%d: %v", t.ID, err)
			continue
		}

		log.Printf("scheduler: ticket #%d executed successfully", t.ID)
	}

	return rows.Err()
}

// executeScheduledTicket performs the actual execution of a scheduled ticket.
// It sets status to EXECUTING first, then DONE.
func (s *Scheduler) executeScheduledTicket(ctx context.Context, t *model.Ticket) error {
	now := time.Now()

	// Transition to EXECUTING
	_, err := s.ticketSvc.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		model.TicketStatusExecuting, now, t.ID, model.TicketStatusScheduled,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			// Ticket was already cancelled or executed by someone else
			return nil
		}
		return err
	}

	// Write audit log
	s.ticketSvc.auditSvc.Write(ctx, AuditRecord{
		UserID:       0,
		Action:       "ticket_schedule_execute",
		DatasourceID: t.DatasourceID,
		Database:     t.Database,
		SQLContent:   t.SQLContent,
		SQLSummary:   t.SQLSummary,
	})

	// Transition to DONE (clear scheduled_at)
	_, err = s.ticketSvc.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, executed_at = ?, scheduled_at = NULL, updated_at = ? WHERE id = ?`,
		model.TicketStatusDone, now, now, t.ID,
	)
	if err != nil {
		return err
	}

	// Send notification
	if s.ticketSvc.notifySvc != nil {
		t.Status = model.TicketStatusDone
		t.ExecutedAt = &now
		t.ScheduledAt = nil
		t.UpdatedAt = now
		s.ticketSvc.populateTicketNames(ctx, t)
		s.ticketSvc.notifySvc.NotifyTicketExecuted(t)
	}

	return nil
}
