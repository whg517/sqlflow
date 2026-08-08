package ticket

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/testutil"
)

// stagedTicket creates a ticket governed by the given approval chain and
// returns the service, the engine and the ticket.
//
// The chain is what these tests are about, so it is the only parameter: every
// caller wants a ticket sitting at stage 1 of a real policy, which is the state
// the approval routes are supposed to respect.
func stagedTicket(t *testing.T, chain string, autoApprove bool) (*Service, *ApprovalEngine, *model.Ticket, int64) {
	t.Helper()

	d := testutil.NewDB(t)
	engine := NewApprovalEngine(d, nil)

	uid := testutil.SeedUser(t, d.DB, "stage_submitter", "developer")
	dsID := testutil.SeedDatasource(t, d.DB, "stage-ds")

	if _, err := engine.CreatePolicy(context.Background(),
		"staged", "multi stage", `{}`, chain, autoApprove, "", true, 1); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	svc := New(Deps{DB: d, ApprovalEngine: engine})
	tk, err := svc.CreateTicket(context.Background(), uid, "developer", dsID,
		testutil.DatasourceDatabase, "ALTER TABLE t ADD c INT", "stage probe")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	return svc, engine, tk, uid
}

// ticketStage reads the persisted status and stage counters.
func ticketStage(t *testing.T, svc *Service, id int64) (model.TicketStatus, int, int) {
	t.Helper()
	row, err := svc.client.Ticket.Get(context.Background(), int(id))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	return model.TicketStatus(row.Status), row.CurrentStage, row.TotalStages
}

// TestApproveTicket_DoesNotFinalizeAMultiStageChain pins the boundary between
// the two approval routes.
//
// ApproveTicket checked only the actor's role and the ticket's status; it holds
// no reference to CurrentStage or TotalStages at all. So a dba facing the chain
// [security, dba] finalized it in one call and stage 1 never happened, while
// GetApprovalChainDetail went on rendering a two-stage chain that was never
// walked. This is the route the production UI's approve button calls, and
// RequireScope is a no-op for JWT sessions, so the role check inside the
// service was the only gate in front of it.
func TestApproveTicket_DoesNotFinalizeAMultiStageChain(t *testing.T) {
	svc, _, tk, _ := stagedTicket(t, `[{"role":"security"},{"role":"dba"}]`, false)

	if status, stage, total := ticketStage(t, svc, tk.ID); status != model.TicketStatusPendingApproval || stage != 1 || total != 2 {
		t.Fatalf("setup: status=%s stage=%d/%d, want PENDING_APPROVAL 1/2", status, stage, total)
	}

	dba := testutil.SeedUser(t, svc.database.DB, "stage_dba", "dba")
	_, err := svc.ApproveTicket(context.Background(), tk.ID, dba, "dba", "looks fine")
	if err == nil {
		t.Error("a dba approved past stage 1, which expects the security role")
	}

	status, stage, total := ticketStage(t, svc, tk.ID)
	if status == model.TicketStatusApproved {
		t.Errorf("status = APPROVED after one call on a %d-stage chain", total)
	}
	if status != model.TicketStatusPendingApproval || stage != 1 {
		t.Errorf("status=%s stage=%d, want the ticket left at PENDING_APPROVAL stage 1", status, stage)
	}
}

// TestProcessApproval_RefusesACancelledTicket pins that a decided ticket cannot
// be revived through the staged route.
//
// The engine's compare-and-swap guarded on current_stage alone — a predicate
// that never mentions status — while the mutation it carried wrote status.
// Nothing resets current_stage when a ticket is cancelled or rejected, so a
// CANCELLED ticket kept stage 1 and matched. Worse, the approve branch also
// writes a fresh sql_hash, so the integrity check executeTicket performs before
// running the statement passed too.
func TestProcessApproval_RefusesACancelledTicket(t *testing.T) {
	svc, engine, tk, uid := stagedTicket(t, `[{"role":"dba"}]`, false)

	if _, err := svc.CancelTicket(context.Background(), tk.ID, uid, "developer", "changed my mind"); err != nil {
		t.Fatalf("CancelTicket: %v", err)
	}

	// Put the stage counter back the way the defect left it.
	//
	// Canceling now clears current_stage, which on its own would make this test
	// pass without proving anything about the predicate — the engine's bounds
	// check would reject stage 0 before the compare-and-swap ran. Restoring
	// stage 1 reproduces the row that used to exist and leaves the status guard
	// as the only thing standing between a cancelled ticket and APPROVED, which
	// is exactly what needs testing.
	if err := svc.client.Ticket.UpdateOneID(int(tk.ID)).
		SetCurrentStage(1).SetTotalStages(1).
		Exec(context.Background()); err != nil {
		t.Fatalf("restore stage: %v", err)
	}
	if status, stage, _ := ticketStage(t, svc, tk.ID); status != model.TicketStatusCancelled || stage != 1 {
		t.Fatalf("setup: status=%s stage=%d, want CANCELLED with stage 1", status, stage)
	}

	dba := testutil.SeedUser(t, svc.database.DB, "revive_dba", "dba")
	if _, err := engine.ProcessApproval(context.Background(), tk.ID, dba, "dba", "approved", "ship it"); err == nil {
		t.Error("a cancelled ticket was approved through the staged route")
	}

	if status, _, _ := ticketStage(t, svc, tk.ID); status != model.TicketStatusCancelled {
		t.Errorf("status = %s, want the ticket to stay CANCELLED", status)
	}
}

// TestCancellableTransition_DoesNotMatchAnExecutingTicket is the deterministic
// half of the cancel-race regression.
//
// CancelTicket read the ticket, checked the status it had just read, then wrote
// with a bare UpdateOneID — so a cancel that arrived while the statement was
// running returned 200 and wrote a ticket_cancel audit record, and the row
// flipped back to DONE when execution finished. The cancel was not late, it was
// lost, and the user was told it had worked.
//
// The guarantee is that the write refuses a row the read did not see, and that
// is a property of the predicate rather than of any timing. Asserting it here
// costs no race window at all; TestTicketStatusHasOneWriter in internal/arch is
// what stops CancelTicket quietly going back to a bare write.
func TestCancellableTransition_DoesNotMatchAnExecutingTicket(t *testing.T) {
	svc, _, tk, _ := stagedTicket(t, `[]`, true)

	// EXECUTING is reachable and is not cancellable: this is the row a cancel
	// with a stale read would land on.
	if err := svc.client.Ticket.UpdateOneID(int(tk.ID)).
		SetStatus(string(model.TicketStatusExecuting)).
		Exec(context.Background()); err != nil {
		t.Fatalf("stage EXECUTING: %v", err)
	}

	applied, err := applyTransition(context.Background(), svc.client.Ticket, tk.ID, time.Now(), transition{
		From: cancellableStatuses,
		To:   model.TicketStatusCancelled,
	})
	if err != nil {
		t.Fatalf("applyTransition: %v", err)
	}
	if applied {
		t.Error("a cancel matched a ticket that was already executing")
	}
	if status, _, _ := ticketStage(t, svc, tk.ID); status != model.TicketStatusExecuting {
		t.Errorf("status = %s, want EXECUTING left untouched", status)
	}
}

// TestLeavingApprovedHasExactlyOneWinner asserts that concurrent callers making
// the same move produce one success.
//
// Treat it as a backstop, not as the regression test for the compare-and-swap.
// Measured on this fixture only two of six callers read the ticket before the
// first write lands, so with a bare write the count is often 1 anyway and the
// test passes with the defect present — verified by sabotage. It still earns
// its place: it fails loudly if the window ever widens, which is exactly when a
// missing predicate does real damage. The binding guarantees are
// TestCancellableTransition_DoesNotMatchAnExecutingTicket above and
// TestTicketStatusHasOneWriter in internal/arch.
//
// The two moves are raced separately on purpose. Racing them against each other
// proves nothing, because canceling a ticket that another caller has just
// scheduled is legitimate — SCHEDULED is a cancellable state, so two successes
// there would be correct behavior rather than a defect.
func TestLeavingApprovedHasExactlyOneWinner(t *testing.T) {
	const racers = 6

	// raceOnApproved runs `attempt` from `racers` goroutines against one freshly
	// approved ticket and returns how many reported success.
	raceOnApproved := func(t *testing.T, attempt func(svc *Service, id, uid int64) error) (int, []error, model.TicketStatus) {
		t.Helper()
		svc, _, tk, uid := stagedTicket(t, `[]`, true)
		if status, _, _ := ticketStage(t, svc, tk.ID); status != model.TicketStatusApproved {
			t.Fatalf("setup: status=%s, want APPROVED via auto-approval", status)
		}

		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			won   int
			errs  []error
			start = make(chan struct{})
		)
		for range racers {
			wg.Go(func() {
				<-start
				err := attempt(svc, tk.ID, uid)
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					won++
					return
				}
				errs = append(errs, err)
			})
		}
		close(start)
		wg.Wait()

		status, _, _ := ticketStage(t, svc, tk.ID)
		return won, errs, status
	}

	t.Run("cancel", func(t *testing.T) {
		won, errs, status := raceOnApproved(t, func(svc *Service, id, uid int64) error {
			_, err := svc.CancelTicket(context.Background(), id, uid, "developer", "cancel race")
			return err
		})
		if won != 1 {
			t.Errorf("%d of %d cancels succeeded, want exactly 1 (errors: %v)", won, racers, errs)
		}
		if status != model.TicketStatusCancelled {
			t.Errorf("final status = %s, want CANCELLED", status)
		}
	})

	t.Run("schedule", func(t *testing.T) {
		won, errs, status := raceOnApproved(t, func(svc *Service, id, uid int64) error {
			_, err := svc.ScheduleTicket(context.Background(), id, uid, "developer", time.Now().Add(time.Hour))
			return err
		})
		if won != 1 {
			t.Errorf("%d of %d schedules succeeded, want exactly 1 (errors: %v)", won, racers, errs)
		}
		if status != model.TicketStatusScheduled {
			t.Errorf("final status = %s, want SCHEDULED", status)
		}
	})
}

// TestSchedulerCannotExecuteAnUnscheduledTicket pins the narrower source set the
// scheduler runs with.
//
// Execution's compare-and-swap accepted APPROVED as well as SCHEDULED, for the
// operator's benefit — pressing execute on an approved ticket is normal. The
// scheduler shared it, and that is where it bit: canceling a schedule returns
// the ticket to APPROVED, so a cancel landing after the scheduler's read still
// matched the swap. The operator was told the schedule was cancelled and the
// statement ran anyway.
//
// Asserting on the sets themselves rather than on a live execution keeps this
// test at the transition module's interface, where the distinction actually
// lives, and off the target-database fixture it would otherwise need.
func TestSchedulerCannotExecuteAnUnscheduledTicket(t *testing.T) {
	if slices.Contains(executableByScheduler, model.TicketStatusApproved) {
		t.Error("the scheduler accepts APPROVED, so canceling a schedule cannot stop a run")
	}
	if !slices.Contains(executableByScheduler, model.TicketStatusScheduled) {
		t.Error("the scheduler rejects SCHEDULED, so nothing scheduled would ever run")
	}
	if !slices.Contains(executableByOperator, model.TicketStatusApproved) {
		t.Error("an operator cannot execute an approved ticket")
	}
	if !slices.Contains(executableByOperator, model.TicketStatusScheduled) {
		t.Error("an operator cannot execute a ticket that is waiting on a schedule")
	}
}

// TestApplyTransition_RefusesAnUndeclaredEdge pins that the state machine is the
// implementation rather than a description of one.
//
// validTransitions had no production caller at all: every guard was a hand
// written `if` beside its own write, and the table had drifted into declaring
// edges nothing walks while missing three the code takes.
func TestApplyTransition_RefusesAnUndeclaredEdge(t *testing.T) {
	svc, _, tk, _ := stagedTicket(t, `[]`, true)

	applied, err := applyTransition(context.Background(), svc.client.Ticket, tk.ID, time.Now(), transition{
		From: []model.TicketStatus{model.TicketStatusApproved},
		To:   model.TicketStatusDone,
	})
	if !errors.Is(err, ErrTransitionNotDeclared) {
		t.Errorf("APPROVED -> DONE: err = %v, want ErrTransitionNotDeclared", err)
	}
	if applied {
		t.Error("an undeclared edge was applied")
	}

	if status, _, _ := ticketStage(t, svc, tk.ID); status != model.TicketStatusApproved {
		t.Errorf("status = %s, want the ticket untouched at APPROVED", status)
	}
}
