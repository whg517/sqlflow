package ticket

import (
	"context"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/testutil"
)

// Execution fixtures (setupTicketExecTest, insertTicket, ticketStatus,
// demoValue) live in ticket_exec_helpers_test.go.

// setupSchedulerTest creates a minimal test setup for the Scheduler.
func setupSchedulerTest(t *testing.T) (*db.DB, *Service) {
	t.Helper()
	testDB := testutil.NewDB(t)

	auditSvc := audit.NewService(testDB, 0, 0)
	// Insert datasource
	testDB.Exec(`INSERT INTO datasources (name, type, host, port, username, password_encrypted, status) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		"test-ds", "mysql", "10.0.0.1", 3306, "root", "enc", "active")

	ticketSvc := New(Deps{DB: testDB, Audit: auditSvc})
	return testDB, ticketSvc
}

func TestScheduler_StartStop(t *testing.T) {
	_, ticketSvc := setupSchedulerTest(t)

	s := NewScheduler(ticketSvc, 100*time.Millisecond)

	// Start should not block
	s.Start()

	// Give it a moment to enter the loop
	time.Sleep(150 * time.Millisecond)

	// Stop should return gracefully
	s.Stop()
}

func TestScheduler_RunOnce_EmptyDB(t *testing.T) {
	_, ticketSvc := setupSchedulerTest(t)

	s := NewScheduler(ticketSvc, time.Hour)
	err := s.RunOnce(testCtx())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
}

// TestScheduler_RunOnce_ExecutesDueTicket is the regression for REV-P1-003.
// A due ticket must reach DONE and its SQL must actually run on the target.
func TestScheduler_RunOnce_ExecutesDueTicket(t *testing.T) {
	platform, ticketSvc, target := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusScheduled,
		"UPDATE demo SET n = 42", time.Now().Add(-time.Minute))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := NewScheduler(ticketSvc, time.Hour).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusDone) {
		t.Fatalf("ticket status = %q, want %q", got, model.TicketStatusDone)
	}
	if n := demoValue(t, target); n != 42 {
		t.Errorf("demo.n = %d, want 42 — ticket SQL did not reach the target", n)
	}
}

// TestScheduler_RunOnce_MultipleDueTickets guards the read/write interleaving.
// The due-ticket cursor must be drained and closed before the first write:
// the platform pool allows a single connection, so a write issued while the
// cursor is open blocks until the context deadline. The short context here
// fails the test rather than hanging if that regresses.
func TestScheduler_RunOnce_MultipleDueTickets(t *testing.T) {
	platform, ticketSvc, target := setupTicketExecTest(t)

	due := time.Now().Add(-time.Minute)
	ids := []int64{
		insertTicket(t, platform, model.TicketStatusScheduled, "UPDATE demo SET n = n + 1", due),
		insertTicket(t, platform, model.TicketStatusScheduled, "UPDATE demo SET n = n + 1", due),
		insertTicket(t, platform, model.TicketStatusScheduled, "UPDATE demo SET n = n + 1", due),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := NewScheduler(ticketSvc, time.Hour).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	for _, id := range ids {
		if got := ticketStatus(t, platform, id); got != string(model.TicketStatusDone) {
			t.Errorf("ticket #%d status = %q, want %q", id, got, model.TicketStatusDone)
		}
	}
	if n := demoValue(t, target); n != 3 {
		t.Errorf("demo.n = %d, want 3 — expected one increment per ticket", n)
	}
}

// TestScheduler_RunOnce_Idempotent verifies a second pass does not re-execute
// an already finished ticket.
func TestScheduler_RunOnce_Idempotent(t *testing.T) {
	platform, ticketSvc, target := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusScheduled,
		"UPDATE demo SET n = n + 1", time.Now().Add(-time.Minute))

	s := NewScheduler(ticketSvc, time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := range 3 {
		if err := s.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce #%d: %v", i+1, err)
		}
	}

	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusDone) {
		t.Fatalf("ticket status = %q, want %q", got, model.TicketStatusDone)
	}
	if n := demoValue(t, target); n != 1 {
		t.Errorf("demo.n = %d, want 1 — ticket executed more than once", n)
	}
}

// TestScheduler_RunOnce_SkipsFutureTicket ensures a ticket scheduled ahead of
// now is left untouched.
func TestScheduler_RunOnce_SkipsFutureTicket(t *testing.T) {
	platform, ticketSvc, target := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusScheduled,
		"UPDATE demo SET n = 99", time.Now().Add(time.Hour))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := NewScheduler(ticketSvc, time.Hour).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusScheduled) {
		t.Errorf("ticket status = %q, want %q", got, model.TicketStatusScheduled)
	}
	if n := demoValue(t, target); n != 0 {
		t.Errorf("demo.n = %d, want 0 — a future ticket must not execute", n)
	}
}

// TestScheduler_RunOnce_SkipsNonScheduledTicket ensures the scheduler only
// picks up SCHEDULED tickets, so a ticket already being executed elsewhere is
// not started a second time.
func TestScheduler_RunOnce_SkipsNonScheduledTicket(t *testing.T) {
	platform, ticketSvc, target := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusExecuting,
		"UPDATE demo SET n = 99", time.Now().Add(-time.Minute))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := NewScheduler(ticketSvc, time.Hour).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusExecuting) {
		t.Errorf("ticket status = %q, want %q", got, model.TicketStatusExecuting)
	}
	if n := demoValue(t, target); n != 0 {
		t.Errorf("demo.n = %d, want 0 — a non-SCHEDULED ticket must not execute", n)
	}
}

func TestScheduler_NewScheduler(t *testing.T) {
	_, ticketSvc := setupSchedulerTest(t)

	s := NewScheduler(ticketSvc, 5*time.Second)
	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}
	if s.interval != 5*time.Second {
		t.Errorf("interval = %v, want 5s", s.interval)
	}
	if s.ticketSvc != ticketSvc {
		t.Error("ticketSvc not set")
	}
}

func testCtx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_ = cancel
	return ctx
}
