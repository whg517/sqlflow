package ticket

import (
	"context"
	"errors"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/testutil"
)

// TestTicketSideReadsRequireAnActor closes the gap between how a ticket is read
// and how everything hanging off it is read.
//
// GetTicketForActor narrows a ticket to its submitter, its reviewer, or a
// governance role. The comments and the approval chain attached to that same
// ticket had no such check — and ListComments could not have had one, because
// its interface took only a ticket id. An interface with nowhere to put the
// actor is an interface that will not check it.
func TestTicketSideReadsRequireAnActor(t *testing.T) {
	database := testutil.NewDB(t)
	engine := NewApprovalEngine(database, nil)
	svc := New(Deps{DB: database, ApprovalEngine: engine})
	commentSvc := NewCommentService(database, svc)
	ctx := context.Background()

	// A ticket needs a policy to route it, or creation is refused — see
	// TestUnmatchedPolicyRefusesTheTicket.
	if _, err := engine.CreatePolicy(ctx, "read-policy", "", "{}",
		`[{"role":"dba"}]`, false, "", true, 100); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	owner := testutil.SeedUser(t, svc.database.DB, "read_owner", "developer")
	stranger := testutil.SeedUser(t, svc.database.DB, "read_stranger", "developer")
	dsID := testutil.SeedDatasource(t, svc.database.DB, "read-ds")

	tk, err := svc.CreateTicket(ctx, owner, "developer", dsID,
		testutil.DatasourceDatabase, "ALTER TABLE t ADD c INT", "read probe")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	if _, err := commentSvc.CreateComment(ctx, tk.ID, owner, "内部讨论：这条会动线上表", 0); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	t.Run("stranger_is_refused", func(t *testing.T) {
		if _, err := commentSvc.ListComments(ctx, tk.ID, stranger, "developer"); !errors.Is(err, ErrNoPermission) {
			t.Errorf("ListComments as a stranger = %v, want ErrNoPermission", err)
		}
		if _, err := engine.GetApprovalChainDetail(ctx, tk.ID, stranger, "developer"); !errors.Is(err, ErrNoPermission) {
			t.Errorf("GetApprovalChainDetail as a stranger = %v, want ErrNoPermission", err)
		}
		if _, err := engine.GetApprovalHistory(ctx, tk.ID, stranger, "developer"); !errors.Is(err, ErrNoPermission) {
			t.Errorf("GetApprovalHistory as a stranger = %v, want ErrNoPermission", err)
		}
	})

	t.Run("owner_and_governance_still_read", func(t *testing.T) {
		comments, err := commentSvc.ListComments(ctx, tk.ID, owner, "developer")
		if err != nil {
			t.Fatalf("ListComments as the submitter: %v", err)
		}
		if len(comments) != 1 {
			t.Errorf("submitter saw %d comments, want 1", len(comments))
		}
		if _, err := commentSvc.ListComments(ctx, tk.ID, stranger, "dba"); err != nil {
			t.Errorf("ListComments as a dba: %v", err)
		}
		if _, err := engine.GetApprovalChainDetail(ctx, tk.ID, stranger, "admin"); err != nil {
			t.Errorf("GetApprovalChainDetail as an admin: %v", err)
		}
	})
}

// TestUnmatchedPolicyRefusesTheTicket keeps a ticket out of a state with no exit.
//
// applyApprovalPolicy logged and swallowed both match and apply failures, on
// the stated theory that the ticket "stays in SUBMITTED for manual review".
// There is no manual-review route: validTransitions lets SUBMITTED reach
// PENDING_APPROVAL and APPROVED, and the only writer of either edge is
// ApplyPolicy — the thing that just failed. ApproveTicket refuses anything that
// is not PENDING_APPROVAL. So the ticket was unreviewable, unexecutable and
// undeletable, and cancel was the only move left.
//
// Refusing the creation is the honest answer: nothing has been promised to
// anyone yet, and the submitter finds out immediately rather than discovering
// it later from a ticket nobody can act on.
func TestUnmatchedPolicyRefusesTheTicket(t *testing.T) {
	database := testutil.NewDB(t)
	// An engine with no policies at all: MatchPolicy has nothing to return.
	engine := NewApprovalEngine(database, nil)
	svc := New(Deps{DB: database, ApprovalEngine: engine})
	ctx := context.Background()

	uid := testutil.SeedUser(t, svc.database.DB, "unmatched_submitter", "developer")
	dsID := testutil.SeedDatasource(t, svc.database.DB, "unmatched-ds")

	_, err := svc.CreateTicket(ctx, uid, "developer", dsID,
		testutil.DatasourceDatabase, "ALTER TABLE t ADD c INT", "no policy exists")
	if !errors.Is(err, ErrNoApprovalPolicy) {
		t.Fatalf("CreateTicket with no matching policy = %v, want ErrNoApprovalPolicy", err)
	}

	// And nothing was left behind in the dead state.
	stranded, err := svc.client.Ticket.Query().
		Where().All(ctx)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	for _, row := range stranded {
		if row.Status == string(model.TicketStatusSubmitted) {
			t.Errorf("ticket #%d was left in SUBMITTED, which no route accepts", row.ID)
		}
	}
}
