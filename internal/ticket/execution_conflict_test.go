package ticket

import (
	"context"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/testutil"
)

// TestExecutionOutcomeDoesNotOverwriteAnotherActorsDecision covers the
// obligation applyTransition states and two callers used to decline.
//
// applyTransition returns whether a row matched, and its contract says a false
// return means another actor decided this ticket first — a conflict, not a
// success. The two lifecycle ends of an execution discarded that with `_`.
//
// The reachable path is the reclaim sweep: it moves an EXECUTING ticket whose
// lease expired to FAILED, and it is not coordinated with the executor that is
// still running. When that executor finished, its EXECUTING→DONE swap matched
// zero rows, the return was ignored, and the caller was told DONE while the row
// said FAILED with "语句是否已在目标库生效未知，请人工确认后再重提". The API and
// the audit trail disagreed with the database about whether the change applied.
func TestExecutionOutcomeDoesNotOverwriteAnotherActorsDecision(t *testing.T) {
	svc := New(Deps{DB: testutil.NewDB(t)})
	ctx := context.Background()

	uid := testutil.SeedUser(t, svc.database.DB, "conflict_submitter", "developer")
	dsID := testutil.SeedDatasource(t, svc.database.DB, "conflict-ds")

	tk, err := svc.CreateTicket(ctx, uid, "developer", dsID,
		testutil.DatasourceDatabase, "ALTER TABLE t ADD c INT", "conflict probe")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	// Put it in EXECUTING with a lease that has already expired, which is what a
	// crashed executor leaves behind.
	expired := time.Now().Add(-time.Hour)
	if err := svc.client.Ticket.UpdateOneID(int(tk.ID)).
		SetStatus(string(model.TicketStatusExecuting)).
		SetLeaseOwner("dead-instance").
		SetLeaseExpiresAt(expired).
		Exec(ctx); err != nil {
		t.Fatalf("stage EXECUTING: %v", err)
	}

	// Another actor decides first.
	if _, err := svc.ReclaimExpiredExecutions(ctx); err != nil {
		t.Fatalf("ReclaimExpiredExecutions: %v", err)
	}

	row, err := svc.client.Ticket.Get(ctx, int(tk.ID))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if row.Status != string(model.TicketStatusFailed) {
		t.Fatalf("precondition: status = %s, want FAILED after reclaim", row.Status)
	}

	// Now the original executor reports its outcome. The swap must not match,
	// and the caller must be told so.
	live := &model.Ticket{
		ID: tk.ID, DatasourceID: dsID, Database: tk.Database,
		SQLContent: tk.SQLContent, SQLSummary: tk.SQLSummary,
		Status: model.TicketStatusExecuting,
	}

	t.Run("success_path", func(t *testing.T) {
		err := svc.concludeExecution(ctx, live.ID, time.Now(), model.TicketStatusDone, nil)
		if err == nil {
			t.Error("an execution that lost the race reported success")
		}
		assertStillFailed(t, svc, tk.ID)
	})

	t.Run("failure_path", func(t *testing.T) {
		err := svc.failTicket(ctx, live, uid, "boom")
		if err == nil {
			t.Error("a failed execution that lost the race reported that it recorded the failure")
		}
		assertStillFailed(t, svc, tk.ID)
	})
}

func assertStillFailed(t *testing.T, svc *Service, id int64) {
	t.Helper()
	row, err := svc.client.Ticket.Get(context.Background(), int(id))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if row.Status != string(model.TicketStatusFailed) {
		t.Errorf("status = %s: the losing writer overwrote the winner's decision", row.Status)
	}
	if row.ReviewComment == "" {
		t.Error("the reclaim reason was overwritten")
	}
}
