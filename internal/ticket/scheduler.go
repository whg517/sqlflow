package ticket

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// Scheduler periodically checks for scheduled tickets whose scheduled_at
// has passed and automatically executes them.
type Scheduler struct {
	ticketSvc *Service
	interval  time.Duration
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewScheduler creates a new Scheduler that checks every interval.
func NewScheduler(ticketSvc *Service, interval time.Duration) *Scheduler {
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
	ids, err := s.dueTicketIDs(ctx, time.Now())
	if err != nil {
		return err
	}

	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.executeScheduledTicket(ctx, id); err != nil {
			log.Printf("scheduler: failed to execute ticket #%d: %v", id, err)
			continue
		}
	}

	return nil
}

// dueTicketIDs returns the IDs of SCHEDULED tickets whose scheduled_at has passed.
//
// Only IDs are selected, and the cursor is fully drained and closed before the
// caller performs any write. db.Open caps the platform SQLite pool at a single
// connection, so a write issued while this cursor is still open would wait for
// a connection that only closing the cursor can release — the write would block
// until the context deadline instead of failing fast.
func (s *Scheduler) dueTicketIDs(ctx context.Context, now time.Time) ([]int64, error) {
	rows, err := s.ticketSvc.database.DB.QueryContext(ctx,
		`SELECT id FROM tickets
		 WHERE status = $1 AND scheduled_at IS NOT NULL AND scheduled_at <= $2
		 ORDER BY scheduled_at ASC`,
		model.TicketStatusScheduled, now,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan due ticket id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// executeScheduledTicket executes one due ticket as the system operator.
//
// The SCHEDULED → EXECUTING transition belongs to Service.executeTicket,
// which performs it as a single compare-and-swap. The scheduler must not
// pre-set EXECUTING: that CAS only matches APPROVED or SCHEDULED, so a
// pre-transitioned ticket would match zero rows and be stranded in EXECUTING
// with no way back.
func (s *Scheduler) executeScheduledTicket(ctx context.Context, ticketID int64) error {
	t, err := s.ticketSvc.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	if t.Status != model.TicketStatusScheduled {
		// Cancelled, rescheduled or picked up by someone else since the scan.
		return nil
	}

	log.Printf("scheduler: executing scheduled ticket #%d (scheduled_at=%s)",
		t.ID, t.ScheduledAt.Format("2006-01-02 15:04:05"))

	// operatorID=0 marks a system execution; permission checks are the
	// responsibility of whoever scheduled the ticket.
	if _, err := s.ticketSvc.executeTicket(ctx, t, 0); err != nil {
		if errors.Is(err, ErrTicketNotExecutable) {
			// Another actor won the CAS between GetTicket and executeTicket.
			return nil
		}
		// executeTicket already recorded FAILED status and audit for us.
		return err
	}

	log.Printf("scheduler: ticket #%d executed successfully", t.ID)
	return nil
}
