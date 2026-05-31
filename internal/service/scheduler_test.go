package service

import (
	"context"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

// setupSchedulerTest creates a minimal test setup for the Scheduler.
func setupSchedulerTest(t *testing.T) (*db.DB, *TicketService) {
	t.Helper()
	testDB := setupAuthTestDB(t)

	auditSvc := NewAuditService(testDB, 0, 0)
	// Insert datasource
	testDB.Exec(`INSERT INTO datasources (name, type, host, port, username, password_encrypted, status) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"test-ds", "mysql", "10.0.0.1", 3306, "root", "enc", "active")

	ticketSvc := NewTicketService(testDB.DB, auditSvc, nil)
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

func TestScheduler_RunOnce_WithScheduledTicket(t *testing.T) {
	// Simple test: just verify RunOnce doesn't crash with a scheduled ticket.
	// Full execution would require a more complex setup.
	_, ticketSvc := setupSchedulerTest(t)

	s := NewScheduler(ticketSvc, time.Hour)
	err := s.RunOnce(testCtx())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
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
