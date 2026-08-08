package ticket

import (
	"context"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/testutil"
)

// executingTicket puts a ticket into EXECUTING with the given lease.
//
// It writes the row directly because the point is to reproduce what a crashed
// process leaves behind, which no live code path produces on purpose.
func executingTicket(t *testing.T, svc *Service, owner string, expires *time.Time) int64 {
	t.Helper()

	uid := testutil.SeedUser(t, svc.database.DB, "lease_submitter", "developer")
	dsID := testutil.SeedDatasource(t, svc.database.DB, "lease-ds")

	tk, err := svc.CreateTicket(context.Background(), uid, "developer", dsID,
		testutil.DatasourceDatabase, "ALTER TABLE t ADD c INT", "lease probe")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	upd := svc.client.Ticket.UpdateOneID(int(tk.ID)).
		SetStatus(string(model.TicketStatusExecuting)).
		SetLeaseOwner(owner)
	if expires != nil {
		upd = upd.SetLeaseExpiresAt(*expires)
	}
	if err := upd.Exec(context.Background()); err != nil {
		t.Fatalf("stage EXECUTING: %v", err)
	}
	return tk.ID
}

// TestReclaimExpiredExecutions_FreesACrashedRun is the crash the roadmap
// describes.
//
// EXECUTING is the one state a dead process can strand a ticket in: the process
// running it is the only thing that would move it out, so if it dies the ticket
// sits there permanently — CancelTicket refuses it, ExecuteTicket refuses it,
// and nothing else looks at it. The lease is what lets a survivor tell an
// abandoned run from a live one.
func TestReclaimExpiredExecutions_FreesACrashedRun(t *testing.T) {
	svc := New(Deps{DB: testutil.NewDB(t)})

	expired := time.Now().Add(-time.Minute)
	id := executingTicket(t, svc, "dead-instance", &expired)

	reclaimed, err := svc.ReclaimExpiredExecutions(context.Background())
	if err != nil {
		t.Fatalf("ReclaimExpiredExecutions: %v", err)
	}
	if reclaimed != 1 {
		t.Errorf("reclaimed = %d, want 1", reclaimed)
	}

	row, err := svc.client.Ticket.Get(context.Background(), int(id))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	// FAILED, not back to APPROVED. Nothing here knows whether the statement
	// reached the target database, and re-running a DDL that already applied is
	// worse than making a human look.
	if row.Status != string(model.TicketStatusFailed) {
		t.Errorf("status = %s, want FAILED", row.Status)
	}
	if row.LeaseOwner != "" || row.LeaseExpiresAt != nil {
		t.Errorf("lease not cleared: owner=%q expires=%v", row.LeaseOwner, row.LeaseExpiresAt)
	}
	if row.ReviewComment == "" {
		t.Error("no reason recorded; the operator has nothing to act on")
	}
}

// TestReclaimExpiredExecutions_LeavesALiveRunAlone is the half that matters
// more: reclaiming a run that is still going would let a second executor start
// the same statement.
func TestReclaimExpiredExecutions_LeavesALiveRunAlone(t *testing.T) {
	svc := New(Deps{DB: testutil.NewDB(t)})

	alive := time.Now().Add(10 * time.Minute)
	id := executingTicket(t, svc, "live-instance", &alive)

	reclaimed, err := svc.ReclaimExpiredExecutions(context.Background())
	if err != nil {
		t.Fatalf("ReclaimExpiredExecutions: %v", err)
	}
	if reclaimed != 0 {
		t.Errorf("reclaimed = %d, want 0 — the lease has not expired", reclaimed)
	}

	row, err := svc.client.Ticket.Get(context.Background(), int(id))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if row.Status != string(model.TicketStatusExecuting) {
		t.Errorf("status = %s, want EXECUTING left alone", row.Status)
	}
}

// TestReclaimExpiredExecutions_FreesALeaselessRun covers rows written before
// the lease existed, and any path that reaches EXECUTING without taking one.
//
// A null expiry cannot be compared against now, so treating it as "not expired"
// would make exactly the permanently-stuck ticket this exists to free.
func TestReclaimExpiredExecutions_FreesALeaselessRun(t *testing.T) {
	svc := New(Deps{DB: testutil.NewDB(t)})

	id := executingTicket(t, svc, "", nil)

	reclaimed, err := svc.ReclaimExpiredExecutions(context.Background())
	if err != nil {
		t.Fatalf("ReclaimExpiredExecutions: %v", err)
	}
	if reclaimed != 1 {
		t.Errorf("reclaimed = %d, want 1", reclaimed)
	}

	row, err := svc.client.Ticket.Get(context.Background(), int(id))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if row.Status != string(model.TicketStatusFailed) {
		t.Errorf("status = %s, want FAILED", row.Status)
	}
}

// TestExecutingTakesALease pins that the claim and the lease are one statement.
//
// Taking the lease afterwards would leave a window in which a crash strands the
// ticket in EXECUTING with nothing to expire — the permanently-stuck row this
// mechanism exists to remove, reached from a different direction.
//
// It exercises the transition rather than executeTicket, because executeTicket
// returns before claiming anything when no datasource service is wired, and a
// test that skips itself proves nothing. That executeTicket reaches EXECUTING
// only through this seam is enforced by TestTicketStatusHasOneWriter in
// internal/arch.
func TestExecutingTakesALease(t *testing.T) {
	svc := New(Deps{DB: testutil.NewDB(t)})

	uid := testutil.SeedUser(t, svc.database.DB, "lease_taker", "developer")
	dsID := testutil.SeedDatasource(t, svc.database.DB, "lease-take-ds")
	tk, err := svc.CreateTicket(context.Background(), uid, "developer", dsID,
		testutil.DatasourceDatabase, "ALTER TABLE t ADD c INT", "lease probe")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if err := svc.client.Ticket.UpdateOneID(int(tk.ID)).
		SetStatus(string(model.TicketStatusApproved)).
		Exec(context.Background()); err != nil {
		t.Fatalf("stage APPROVED: %v", err)
	}

	now := time.Now()
	// No Extra: the lease is the transition's business, so a caller cannot claim
	// a ticket without one even by forgetting.
	applied, err := applyTransition(context.Background(), svc.client.Ticket, tk.ID, now, transition{
		From: executableByOperator,
		To:   model.TicketStatusExecuting,
	})
	if err != nil {
		t.Fatalf("applyTransition: %v", err)
	}
	if !applied {
		t.Fatal("the claim did not match an APPROVED ticket")
	}

	row, err := svc.client.Ticket.Get(context.Background(), int(tk.ID))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if row.LeaseOwner == "" {
		t.Error("claimed the ticket without recording who is running it")
	}
	if row.LeaseExpiresAt == nil || !row.LeaseExpiresAt.After(now) {
		t.Errorf("lease expiry = %v, want a time after the claim", row.LeaseExpiresAt)
	}

	// And a lease taken now must not be reclaimable, or the sweep would steal
	// every run it finds.
	reclaimed, err := svc.ReclaimExpiredExecutions(context.Background())
	if err != nil {
		t.Fatalf("ReclaimExpiredExecutions: %v", err)
	}
	if reclaimed != 0 {
		t.Errorf("reclaimed = %d, want 0 — the lease was just taken", reclaimed)
	}
}
