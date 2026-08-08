package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/testutil"
)

// decisionFixture builds a ticket sitting at stage 1 of a two-stage chain whose
// first stage belongs to a role the submitter does not hold.
func decisionFixture(t *testing.T, submitterRole string) (*Service, int64, int64) {
	t.Helper()

	database := testutil.NewDB(t)
	engine := NewApprovalEngine(database, nil)
	svc := New(Deps{DB: database, ApprovalEngine: engine})

	submitter := testutil.SeedUser(t, svc.database.DB, "decider_submitter", submitterRole)
	dsID := testutil.SeedDatasource(t, svc.database.DB, "decision-ds")

	// The policy first: creation is refused when nothing routes the ticket.
	// A two-stage chain whose first stage belongs to a role nobody in these
	// tests holds by accident.
	chain, err := json.Marshal([]ApprovalChainStage{{Role: "security"}, {Role: "dba"}})
	if err != nil {
		t.Fatalf("marshal chain: %v", err)
	}
	policy, err := engine.CreatePolicy(context.Background(), "decision-policy", "",
		"{}", string(chain), false, "", true, 100)
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	tk, err := svc.CreateTicket(context.Background(), submitter, submitterRole, dsID,
		testutil.DatasourceDatabase, "ALTER TABLE t ADD c INT", "decision probe")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	// Put it where a decision is possible, under that chain.
	if err := svc.client.Ticket.UpdateOneID(int(tk.ID)).
		SetStatus(string(model.TicketStatusPendingApproval)).
		SetPolicyID(policy.ID).
		SetCurrentStage(1).
		SetTotalStages(2).
		Exec(context.Background()); err != nil {
		t.Fatalf("stage PENDING_APPROVAL: %v", err)
	}
	return svc, tk.ID, submitter
}

// TestSubmitterCannotDecideTheirOwnTicket is the separation the whole workflow
// rests on.
//
// Neither decision path checked that the approver is not the submitter. A
// developer who also holds dba — or any submitter on a chain stage naming their
// own role — could approve their own change. The platform's entire reason for
// existing is that a high-risk change is seen by someone other than its author.
//
// auto_skip_same_submitter is a different feature and does not cover this: it is
// about skipping a stage the submitter would have filled, and nothing read it
// anyway.
func TestSubmitterCannotDecideTheirOwnTicket(t *testing.T) {
	t.Run("approve", func(t *testing.T) {
		svc, ticketID, submitter := decisionFixture(t, "dba")

		_, err := svc.ApproveTicket(context.Background(), ticketID, submitter, "dba", "自审批")
		if !errors.Is(err, ErrSelfApproval) {
			t.Errorf("ApproveTicket by the submitter = %v, want ErrSelfApproval", err)
		}
		assertNotDecided(t, svc, ticketID)
	})

	t.Run("reject", func(t *testing.T) {
		svc, ticketID, submitter := decisionFixture(t, "dba")

		_, err := svc.RejectTicket(context.Background(), ticketID, submitter, "dba", "自驳回")
		if !errors.Is(err, ErrSelfApproval) {
			t.Errorf("RejectTicket by the submitter = %v, want ErrSelfApproval", err)
		}
		assertNotDecided(t, svc, ticketID)
	})

	t.Run("engine", func(t *testing.T) {
		svc, ticketID, submitter := decisionFixture(t, "dba")

		_, err := svc.approvalEngine.ProcessApproval(context.Background(), ticketID, submitter, "dba", "approved", "自审批")
		if !errors.Is(err, ErrSelfApproval) {
			t.Errorf("ProcessApproval by the submitter = %v, want ErrSelfApproval", err)
		}
		assertNotDecided(t, svc, ticketID)
	})
}

// TestRejectHonorsTheApprovalChain closes the asymmetry between the two
// decisions.
//
// ApproveTicket refuses to shortcut a chain and delegates to the engine, which
// checks the stage's role. RejectTicket did not: any admin or dba could reject
// outright and clear current_stage/total_stages, so every stage role a chain
// declared was bypassable from one side. A rejection ends the ticket just as
// finally as an approval does.
func TestRejectHonorsTheApprovalChain(t *testing.T) {
	svc, ticketID, _ := decisionFixture(t, "developer")

	other := testutil.SeedUser(t, svc.database.DB, "decider_dba", "dba")

	_, err := svc.RejectTicket(context.Background(), ticketID, other, "dba", "不同意")
	if err == nil {
		t.Error("a dba rejected a ticket whose current stage belongs to security")
	}
	assertNotDecided(t, svc, ticketID)
}

func assertNotDecided(t *testing.T, svc *Service, ticketID int64) {
	t.Helper()
	row, err := svc.client.Ticket.Get(context.Background(), int(ticketID))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if row.Status != string(model.TicketStatusPendingApproval) {
		t.Errorf("status = %s, want PENDING_APPROVAL — the decision went through", row.Status)
	}
}
