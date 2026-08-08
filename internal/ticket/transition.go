package ticket

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"log"

	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/db/ent/predicate"
	entTicket "github.com/whg517/sqlflow/internal/db/ent/ticket"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
)

// ErrTransitionNotDeclared reports a move the state machine does not allow.
//
// It is a programming error rather than a user error: the caller asked for an
// edge that does not exist, which no request should be able to produce.
var ErrTransitionNotDeclared = errors.New("状态迁移未在状态机中声明")

// validTransitions is the ticket state machine, and it is the implementation
// rather than a description of one.
//
// It used to be neither: `CanTransition` had no production caller at all, every
// real guard was a hand-written `if` at its own call site, and the table had
// drifted away from the code it claimed to describe — it lacked
// SUBMITTED → PENDING_APPROVAL and SUBMITTED → APPROVED (both written by
// ApplyPolicy) and SCHEDULED → APPROVED (written by CancelSchedule), while
// declaring an AI_REVIEWED state nothing has ever written. Two tests in this
// package asserted opposite things about SUBMITTED → APPROVED and both passed,
// because one of them was only checking the table against itself.
//
// Every edge below is exercised by production code; applyTransition refuses
// anything absent, so the table cannot drift from behavior again without a
// test failing.
var validTransitions = map[model.TicketStatus][]model.TicketStatus{
	// ApplyPolicy sends a new ticket either into a chain or straight to
	// APPROVED when the matched policy auto-approves.
	model.TicketStatusSubmitted: {
		model.TicketStatusPendingApproval,
		model.TicketStatusApproved,
		model.TicketStatusCancelled,
	},
	model.TicketStatusPendingApproval: {
		model.TicketStatusApproved,
		model.TicketStatusRejected,
		model.TicketStatusCancelled,
	},
	model.TicketStatusApproved: {
		model.TicketStatusExecuting,
		model.TicketStatusScheduled,
		model.TicketStatusCancelled,
	},
	// SCHEDULED returns to APPROVED when the schedule is cancelled.
	model.TicketStatusScheduled: {
		model.TicketStatusExecuting,
		model.TicketStatusApproved,
		model.TicketStatusCancelled,
	},
	model.TicketStatusExecuting: {
		model.TicketStatusDone,
		model.TicketStatusFailed,
	},
	model.TicketStatusRejected: {model.TicketStatusSubmitted},
	// Terminal. FAILED is listed explicitly rather than left absent: a missing
	// key and an empty list read the same to CanTransition but not to a reader.
	model.TicketStatusDone:      {},
	model.TicketStatusFailed:    {},
	model.TicketStatusCancelled: {},
}

// CanTransition reports whether the state machine declares this edge.
func CanTransition(from, to model.TicketStatus) bool {
	return slices.Contains(validTransitions[from], to)
}

// transition describes one lifecycle move.
//
// From lists every status the move may legitimately leave, because some moves
// have more than one — execution accepts a ticket that is APPROVED as well as
// one that is SCHEDULED.
type transition struct {
	From []model.TicketStatus
	To   model.TicketStatus

	// AtStage additionally guards on the approval stage the caller observed.
	//
	// Status alone is not enough for the staged approval route: two approvers
	// looking at the same PENDING_APPROVAL ticket are both entitled to act, but
	// only one may advance the chain. It is a pointer because "no stage guard"
	// and "stage 0" are different things — auto-approved tickets really are at
	// stage 0.
	AtStage *int

	// Extra adds column assignments to the same statement, so they land only if
	// the compare-and-swap matched.
	Extra func(*ent.TicketUpdate) *ent.TicketUpdate
}

// atStage guards a transition on the approval stage the caller read.
func atStage(n int) *int { return &n }

// applyTransition is the only place tickets.status is written.
//
// It does three things a bare Update cannot. It refuses an edge the state
// machine does not declare, so the table above cannot silently disagree with
// the code. It compares and swaps in one statement, because read-then-write
// leaves a window in which two callers both observe the state they are leaving
// and both write — the shape that once let four concurrent approvals through,
// and that still let a cancel and a schedule both report success on one ticket.
// And it can guard the approval stage in the same predicate as the status,
// which the staged route needs and previously did not have: guarding stage
// alone let a CANCELLED ticket, whose current_stage nothing resets, be walked
// back to APPROVED.
//
// It returns whether a row matched. A false return is not an error — it means
// another actor decided this ticket first, which callers must report as a
// conflict rather than as success.
func applyTransition(
	ctx context.Context,
	tickets *ent.TicketClient,
	ticketID int64,
	now time.Time,
	tr transition,
) (bool, error) {
	if err := tr.declared(); err != nil {
		return false, err
	}

	statuses := make([]string, len(tr.From))
	for i, st := range tr.From {
		statuses[i] = string(st)
	}

	preds := []predicate.Ticket{
		entTicket.IDEQ(int(ticketID)),
		entTicket.StatusIn(statuses...),
	}
	if tr.AtStage != nil {
		preds = append(preds, entTicket.CurrentStageEQ(*tr.AtStage))
	}

	upd := tickets.Update().Where(preds...).
		SetStatus(string(tr.To)).
		SetUpdatedAt(now)

	// The lease belongs to the state, not to the caller.
	//
	// Taking it here rather than as an Extra each executor remembers to pass is
	// what makes it impossible to claim a ticket without one — and a claim
	// without a lease is precisely the row that gets stranded forever when the
	// process dies. Leaving EXECUTING releases it for the same reason: a
	// finished run that kept its lease would look live to the reclaim sweep
	// until it expired.
	switch {
	case tr.To == model.TicketStatusExecuting:
		upd = upd.SetLeaseOwner(leaseOwner).SetLeaseExpiresAt(now.Add(executionLeaseTTL))
	case slices.Contains(tr.From, model.TicketStatusExecuting):
		upd = upd.SetLeaseOwner("").ClearLeaseExpiresAt()
	}

	if tr.Extra != nil {
		upd = tr.Extra(upd)
	}

	affected, err := upd.Save(ctx)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// declared checks the move against the state machine.
//
// A move whose From equals its To is a stage advance rather than a status
// change — the staged approval route walks a ticket through several stages
// while it stays PENDING_APPROVAL — so it needs no edge.
func (tr transition) declared() error {
	if len(tr.From) == 0 {
		return fmt.Errorf("%w: 未指定来源状态", ErrTransitionNotDeclared)
	}
	for _, from := range tr.From {
		if from == tr.To {
			continue
		}
		if !CanTransition(from, tr.To) {
			return fmt.Errorf("%w: %s -> %s", ErrTransitionNotDeclared, from, tr.To)
		}
	}
	return nil
}

// Source-state sets for execution.
//
// They differ, and that difference is the point. An operator pressing execute
// may run a ticket that is APPROVED or one that is SCHEDULED. The scheduler may
// only run one that is still SCHEDULED: if it accepted APPROVED, then canceling
// a schedule — which returns the ticket to APPROVED — would not stop the run
// that was already under way, and the operator who cancelled would be told it
// had worked. Narrowing the scheduler's set is what actually closes that race;
// adding a compare-and-swap to CancelSchedule alone does not, because the cancel
// still lands on a status the wider set accepts.
var (
	executableByOperator  = []model.TicketStatus{model.TicketStatusApproved, model.TicketStatusScheduled}
	executableByScheduler = []model.TicketStatus{model.TicketStatusScheduled}
)

// executionLeaseTTL bounds how long a claimed execution may go unheard from.
//
// Comfortably longer than the 30-second execution timeout, because expiring a
// lease on a run that is still going would let a second executor start the same
// statement — the failure the lease exists to prevent, arrived at from the other
// side. Long enough to absorb a slow shutdown; short enough that an operator is
// not waiting an hour on a crashed instance.
const executionLeaseTTL = 5 * time.Minute

// leaseOwner identifies this process in a lease.
//
// A ticket stranded by a crash is only recognizable if the row says who was
// running it, and "some instance" is not an answer an operator can act on.
var leaseOwner = func() string {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s/%d", host, os.Getpid())
}()

// ReclaimExpiredExecutions frees tickets whose executor is gone.
//
// EXECUTING is the only state a crash can strand a ticket in: the process
// running it is the sole thing that would move it out, so if that process dies
// the ticket is left uncancellable — CancelTicket does not accept EXECUTING —
// and unexecutable, because the claim already happened. Nothing else in the
// platform looks at it again.
//
// Reclaimed tickets go to FAILED, not back to APPROVED. Nothing here knows
// whether the statement reached the target database: the process died somewhere
// between claiming the ticket and recording the outcome, and a DDL that already
// applied would be applied a second time. Failing it puts a human in the loop
// with the evidence, which is the honest answer to "we do not know".
//
// A null expiry is treated as expired. Any row in EXECUTING without a lease
// predates this mechanism or reached the state by a path that does not claim
// one, and either way it is the permanently-stuck ticket this exists to free.
func (s *Service) ReclaimExpiredExecutions(ctx context.Context) (int, error) {
	now := time.Now()

	stale, err := s.client.Ticket.Query().
		Where(
			entTicket.StatusEQ(string(model.TicketStatusExecuting)),
			entTicket.Or(
				entTicket.LeaseExpiresAtIsNil(),
				entTicket.LeaseExpiresAtLT(now),
			),
		).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("查询过期执行租约失败: %w", err)
	}

	reclaimed := 0
	for _, row := range stale {
		owner := row.LeaseOwner
		if owner == "" {
			owner = "未知实例"
		}
		reason := fmt.Sprintf("执行进程 %s 已失联，工单被回收为失败；语句是否已在目标库生效未知，请人工确认后再重提", owner)

		// Guarded like every other move, and on the lease as well as the status:
		// the owner may have come back between the query and this write, and
		// stealing a live run is the one outcome worse than a stuck ticket.
		applied, err := applyTransition(ctx, s.client.Ticket, int64(row.ID), now, transition{
			From: []model.TicketStatus{model.TicketStatusExecuting},
			To:   model.TicketStatusFailed,
			Extra: func(u *ent.TicketUpdate) *ent.TicketUpdate {
				return u.SetReviewComment(reason)
			},
		})
		if err != nil {
			log.Printf("ticket: reclaim lease for #%d: %v", row.ID, err)
			continue
		}
		if !applied {
			continue
		}

		reclaimed++
		s.auditSvc.Write(ctx, auditlog.Record{
			UserID:       0,
			Action:       "ticket_execution_reclaimed",
			DatasourceID: row.DatasourceID,
			Database:     row.Database,
			SQLContent:   row.SQLContent,
			SQLSummary:   row.SQLSummary,
			ErrorMessage: reason,
			TicketID:     int64(row.ID),
		})
		log.Printf("ticket: reclaimed #%d from %s", row.ID, owner)
	}

	return reclaimed, nil
}
