package ticket

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
)

// Regressions for REV-P1-006 (risk level must be server-derived) and
// REV-P1-007 (approval transitions must be atomic).

func ticketRiskLevel(t *testing.T, platform *db.DB, id int64) string {
	t.Helper()
	var risk string
	if err := platform.QueryRow(`SELECT risk_level FROM tickets WHERE id = $1`, id).Scan(&risk); err != nil {
		t.Fatalf("read ticket #%d risk_level: %v", id, err)
	}
	return risk
}

// TestCreateTicket_RiskIsServerDerived pins the REV-P1-006 invariant: a
// destructive statement is scored by the server regardless of what the caller
// would like it to be. Risk selects the approval policy, so a submitter who
// could set it could also pick their own approval path.
func TestCreateTicket_RiskIsServerDerived(t *testing.T) {
	platform, ticketSvc, _ := setupTicketExecTest(t)

	const destructive = "DELETE FROM users"
	wantRisk := NewRiskEvaluator().Evaluate(NewSQLAnalyzer().Analyze(destructive)).Level
	if wantRisk == RiskLevelLow {
		t.Fatalf("fixture is not destructive enough: evaluator scored %q", wantRisk)
	}

	ticket, err := ticketSvc.CreateTicket(context.Background(), 1, "developer", 1,
		ticketExecDatabase, destructive, "mysql", "cleanup")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	if ticket.RiskLevel != wantRisk {
		t.Errorf("returned risk = %q, want %q", ticket.RiskLevel, wantRisk)
	}
	if got := ticketRiskLevel(t, platform, ticket.ID); got != wantRisk {
		t.Errorf("stored risk = %q, want %q", got, wantRisk)
	}
}

// TestCreateTicket_DerivesSQLTypeAndTables checks the other server-derived
// facts approvers rely on to judge impact.
func TestCreateTicket_DerivesSQLTypeAndTables(t *testing.T) {
	_, ticketSvc, _ := setupTicketExecTest(t)

	ticket, err := ticketSvc.CreateTicket(context.Background(), 1, "developer", 1,
		ticketExecDatabase, "ALTER TABLE users ADD COLUMN phone VARCHAR(20)", "mysql", "add phone")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	if ticket.SQLType == "" {
		t.Error("SQLType is empty, want it derived from the SQL")
	}
	if ticket.AffectedTables == "" || ticket.AffectedTables == "[]" {
		t.Errorf("AffectedTables = %q, want the parsed table list", ticket.AffectedTables)
	}
}

// TestApproveTicket_ConcurrentApprovalsProduceOneDecision is the REV-P1-007
// regression. Read-then-write let two approvers both observe PENDING_APPROVAL
// and both write; exactly one must now win.
func TestApproveTicket_ConcurrentApprovalsProduceOneDecision(t *testing.T) {
	platform, ticketSvc, _ := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusPendingApproval,
		"UPDATE demo SET n = 7", time.Time{})

	const approvers = 4
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		succeed int
		errs    []error
	)
	start := make(chan struct{})

	for i := range approvers {
		wg.Add(1)
		go func(reviewerID int64) {
			defer wg.Done()
			<-start
			_, err := ticketSvc.ApproveTicket(context.Background(), id, reviewerID, "dba", "ok")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				succeed++
			} else {
				errs = append(errs, err)
			}
		}(int64(10 + i))
	}
	close(start)
	wg.Wait()

	if succeed != 1 {
		t.Errorf("%d approvals succeeded, want exactly 1 (errors: %v)", succeed, errs)
	}
	for _, err := range errs {
		if err != ErrInvalidStatusTransition {
			t.Errorf("loser error = %v, want ErrInvalidStatusTransition", err)
		}
	}
	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusApproved) {
		t.Errorf("ticket status = %q, want %q", got, model.TicketStatusApproved)
	}
}

// TestApproveThenReject_SecondDecisionIsRefused covers the mixed race: once a
// ticket leaves PENDING_APPROVAL, no further decision may land on it.
func TestApproveThenReject_SecondDecisionIsRefused(t *testing.T) {
	platform, ticketSvc, _ := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusPendingApproval,
		"UPDATE demo SET n = 7", time.Time{})

	ctx := context.Background()
	if _, err := ticketSvc.ApproveTicket(ctx, id, 2, "dba", "ok"); err != nil {
		t.Fatalf("ApproveTicket: %v", err)
	}

	_, err := ticketSvc.RejectTicket(ctx, id, 3, "dba", "changed my mind")
	if err != ErrInvalidStatusTransition {
		t.Errorf("RejectTicket after approval: error = %v, want ErrInvalidStatusTransition", err)
	}
	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusApproved) {
		t.Errorf("ticket status = %q, want it to stay %q", got, model.TicketStatusApproved)
	}
}

// TestResubmitTicket_ConcurrentResubmitsProduceOneRevision guards the
// snapshot-plus-update pair: without a transaction and a CAS, two resubmits can
// both snapshot revision N and both claim revision N+1.
func TestResubmitTicket_ConcurrentResubmitsProduceOneRevision(t *testing.T) {
	platform, ticketSvc, _ := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusRejected,
		"DELETE FROM users", time.Time{})

	const attempts = 4
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		succeed int
	)
	start := make(chan struct{})

	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := ticketSvc.ResubmitTicket(context.Background(), id, 1,
				"DELETE FROM users WHERE status = 0", "added WHERE"); err == nil {
				mu.Lock()
				succeed++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if succeed != 1 {
		t.Errorf("%d resubmits succeeded, want exactly 1", succeed)
	}

	var revisions int
	if err := platform.QueryRow(`SELECT COUNT(*) FROM ticket_revisions WHERE ticket_id = $1`, id).
		Scan(&revisions); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if revisions != 1 {
		t.Errorf("%d revision snapshots, want exactly 1", revisions)
	}
}

// TestResubmitTicket_RefreshesAnalysis pins the second half of REV-P1-007: a
// resubmission is a new proposal, so every server-derived fact must describe
// the new SQL rather than the rejected one.
func TestResubmitTicket_RefreshesAnalysis(t *testing.T) {
	platform, ticketSvc, _ := setupTicketExecTest(t)
	id := insertTicket(t, platform, model.TicketStatusRejected,
		"DROP TABLE users", time.Time{})

	// Seed stale analysis from the rejected revision.
	if _, err := platform.Exec(
		`UPDATE tickets SET risk_level = $1, sql_type = $2, affected_tables = $3 WHERE id = $4`,
		RiskLevelCritical, "DROP", `["users"]`, id,
	); err != nil {
		t.Fatalf("seed stale analysis: %v", err)
	}

	const fixed = "ALTER TABLE orders ADD COLUMN note VARCHAR(20)"
	result, err := ticketSvc.ResubmitTicket(context.Background(), id, 1, fixed, "replaced with an additive change")
	if err != nil {
		t.Fatalf("ResubmitTicket: %v", err)
	}

	wantRisk := NewRiskEvaluator().Evaluate(NewSQLAnalyzer().Analyze(fixed)).Level
	if result.RiskLevel != wantRisk {
		t.Errorf("RiskLevel = %q, want re-derived %q", result.RiskLevel, wantRisk)
	}
	if got := ticketRiskLevel(t, platform, id); got != wantRisk {
		t.Errorf("stored risk = %q, want re-derived %q", got, wantRisk)
	}
	if result.SQLType == "DROP" {
		t.Error("SQLType still describes the rejected SQL")
	}
	if result.AffectedTables == `["users"]` {
		t.Error("AffectedTables still describes the rejected SQL")
	}
}
