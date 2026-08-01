package ticket

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
)

// Regressions for REV-P1-017: sql_hash pins the SQL body that was approved, so
// a ticket whose sql_content changed after approval must not execute. The check
// lived in executeTicket all along but could never fire, because the ticket read
// path did not select sql_hash — model.Ticket.SQLHash was always empty.

// tamperTicketSQL rewrites a ticket's SQL directly in the platform database,
// standing in for any path that mutates an approved ticket.
func tamperTicketSQL(t *testing.T, platform *db.DB, id int64, sqlContent string) {
	t.Helper()
	if _, err := platform.Exec(
		`UPDATE tickets SET sql_content = ?, sql_summary = ? WHERE id = ?`,
		sqlContent, sqlContent, id,
	); err != nil {
		t.Fatalf("tamper ticket #%d: %v", id, err)
	}
}

func ticketSQLHash(t *testing.T, platform *db.DB, id int64) string {
	t.Helper()
	var hash string
	if err := platform.QueryRow(`SELECT sql_hash FROM tickets WHERE id = ?`, id).Scan(&hash); err != nil {
		t.Fatalf("read ticket #%d sql_hash: %v", id, err)
	}
	return hash
}

// TestGetTicket_ReturnsSQLHash is the direct regression: approval writes
// sql_hash, and the read path must return it. While it did not, every
// downstream integrity check silently compared against an empty string.
func TestGetTicket_ReturnsSQLHash(t *testing.T) {
	platform, ticketSvc, _ := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusPendingApproval,
		"UPDATE demo SET n = 7", time.Time{})

	ctx := context.Background()
	if _, err := ticketSvc.ApproveTicket(ctx, id, 2, "dba", "ok"); err != nil {
		t.Fatalf("ApproveTicket: %v", err)
	}

	stored := ticketSQLHash(t, platform, id)
	if stored == "" {
		t.Fatal("sql_hash was not persisted on approval")
	}

	got, err := ticketSvc.GetTicket(ctx, id)
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if got.SQLHash != stored {
		t.Errorf("GetTicket().SQLHash = %q, want %q", got.SQLHash, stored)
	}
}

// TestListTickets_ReturnsSQLHash guards the second read path against the same
// column-list drift.
func TestListTickets_ReturnsSQLHash(t *testing.T) {
	platform, ticketSvc, _ := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusPendingApproval,
		"UPDATE demo SET n = 7", time.Time{})

	ctx := context.Background()
	if _, err := ticketSvc.ApproveTicket(ctx, id, 2, "dba", "ok"); err != nil {
		t.Fatalf("ApproveTicket: %v", err)
	}

	tickets, _, err := ticketSvc.ListTickets(ctx, 1, 10, "", "", "", "", "", "all", 2, "dba")
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("got %d tickets, want 1", len(tickets))
	}
	if tickets[0].SQLHash != ticketSQLHash(t, platform, id) {
		t.Errorf("ListTickets()[0].SQLHash = %q, want the persisted hash", tickets[0].SQLHash)
	}
}

// TestExecuteTicket_RejectsTamperedSQL is the behavioral regression: SQL
// changed after approval must be refused, and the tampered statement must never
// reach the target database.
func TestExecuteTicket_RejectsTamperedSQL(t *testing.T) {
	platform, ticketSvc, target := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusPendingApproval,
		"UPDATE demo SET n = 7", time.Time{})

	ctx := context.Background()
	if _, err := ticketSvc.ApproveTicket(ctx, id, 2, "dba", "ok"); err != nil {
		t.Fatalf("ApproveTicket: %v", err)
	}

	tamperTicketSQL(t, platform, id, "UPDATE demo SET n = 666")

	_, err := ticketSvc.ExecuteTicket(ctx, id, 2, "dba", "dba-user")
	if err == nil {
		t.Fatal("ExecuteTicket succeeded on a ticket whose SQL changed after approval")
	}
	if !strings.Contains(err.Error(), "与审批版本不一致") {
		t.Errorf("error = %v, want the approval-mismatch rejection", err)
	}

	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusFailed) {
		t.Errorf("ticket status = %q, want %q", got, model.TicketStatusFailed)
	}
	if n := demoValue(t, target); n != 0 {
		t.Errorf("demo.n = %d, want 0 — the tampered statement reached the target", n)
	}
}

// TestExecuteTicket_AcceptsUnchangedSQL guards against the check firing on
// legitimate executions.
func TestExecuteTicket_AcceptsUnchangedSQL(t *testing.T) {
	platform, ticketSvc, target := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusPendingApproval,
		"UPDATE demo SET n = 7", time.Time{})

	ctx := context.Background()
	if _, err := ticketSvc.ApproveTicket(ctx, id, 2, "dba", "ok"); err != nil {
		t.Fatalf("ApproveTicket: %v", err)
	}
	if _, err := ticketSvc.ExecuteTicket(ctx, id, 2, "dba", "dba-user"); err != nil {
		t.Fatalf("ExecuteTicket: %v", err)
	}

	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusDone) {
		t.Errorf("ticket status = %q, want %q", got, model.TicketStatusDone)
	}
	if n := demoValue(t, target); n != 7 {
		t.Errorf("demo.n = %d, want 7", n)
	}
}

// TestScheduler_RejectsTamperedSQL covers the same guarantee on the scheduled
// path, which only became reachable once REV-P1-003 was fixed.
func TestScheduler_RejectsTamperedSQL(t *testing.T) {
	platform, ticketSvc, target := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusPendingApproval,
		"UPDATE demo SET n = 7", time.Time{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := ticketSvc.ApproveTicket(ctx, id, 2, "dba", "ok"); err != nil {
		t.Fatalf("ApproveTicket: %v", err)
	}
	// ScheduleTicket refuses a past time, so backdate the row directly to make
	// the ticket due on this pass.
	if _, err := platform.Exec(
		`UPDATE tickets SET status = ?, scheduled_at = ? WHERE id = ?`,
		model.TicketStatusScheduled, time.Now().Add(-time.Minute), id,
	); err != nil {
		t.Fatalf("backdate schedule: %v", err)
	}

	tamperTicketSQL(t, platform, id, "UPDATE demo SET n = 666")

	if err := NewScheduler(ticketSvc, time.Hour).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusFailed) {
		t.Errorf("ticket status = %q, want %q", got, model.TicketStatusFailed)
	}
	if n := demoValue(t, target); n != 0 {
		t.Errorf("demo.n = %d, want 0 — the tampered statement reached the target", n)
	}
}

// TestResubmitTicket_ClearsSQLHash ensures a resubmitted ticket does not carry
// the hash of a previously approved body.
func TestResubmitTicket_ClearsSQLHash(t *testing.T) {
	platform, ticketSvc, _ := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusRejected,
		"UPDATE demo SET n = 7", time.Time{})

	// Simulate a hash left over from an earlier approval round.
	if _, err := platform.Exec(`UPDATE tickets SET sql_hash = ? WHERE id = ?`, "stale-hash", id); err != nil {
		t.Fatalf("seed stale hash: %v", err)
	}

	ctx := context.Background()
	if _, err := ticketSvc.ResubmitTicket(ctx, id, 1, "UPDATE demo SET n = 8", "fixed"); err != nil {
		t.Fatalf("ResubmitTicket: %v", err)
	}

	if got := ticketSQLHash(t, platform, id); got != "" {
		t.Errorf("sql_hash = %q after resubmit, want empty — the new body is not approved yet", got)
	}
}
