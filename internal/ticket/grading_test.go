package ticket

import (
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/notify"
)

// TestExecutableCommentIsGradedAsWhatItRuns closes the disagreement between the
// splitter and the grader.
//
// internal/platform/sqlparser/split.go deliberately scans /*!nnnnn ... */ as
// code, because the MySQL server executes what is inside it — split_test.go
// pins that, and the comment there says treating it as a comment "would hide a
// live DROP from the analyzer". But the grader it feeds stripped exactly that
// pattern along with ordinary comments, so the body normalized to the empty
// string, took the OTHER branch and scored medium, while the statement ran.
//
// Risk selects the approval policy, so this is the client influencing its own
// approval path — invariant 5 — through a construct the platform reads two
// different ways.
func TestExecutableCommentIsGradedAsWhatItRuns(t *testing.T) {
	bare, err := planTicketSQL("mysql", "DROP TABLE users")
	if err != nil {
		t.Fatalf("plan bare DROP: %v", err)
	}
	bareRisk := NewRiskEvaluator().Evaluate(bare.Analysis)

	hidden, err := planTicketSQL("mysql", "/*!50000 DROP TABLE users */")
	if err != nil {
		t.Fatalf("plan wrapped DROP: %v", err)
	}
	hiddenRisk := NewRiskEvaluator().Evaluate(hidden.Analysis)

	if hidden.Analysis.SQLType != bare.Analysis.SQLType {
		t.Errorf("SQLType = %q for the wrapped DROP, want %q as for the bare one",
			hidden.Analysis.SQLType, bare.Analysis.SQLType)
	}
	if hiddenRisk.Level != bareRisk.Level {
		t.Errorf("risk = %q for /*!50000 DROP TABLE users */, want %q as for the bare DROP",
			hiddenRisk.Level, bareRisk.Level)
	}

	// The target has to survive too, or table-level checks see nothing to check.
	if len(hidden.Analysis.AffectedTables) == 0 {
		t.Error("the wrapped DROP named no affected table")
	}
}

// TestOrdinaryCommentsAreStillRemoved guards the other direction: only the
// executable form is code.
func TestOrdinaryCommentsAreStillRemoved(t *testing.T) {
	plan, err := planTicketSQL("mysql", "SELECT 1 /* DROP TABLE users */ -- DROP TABLE users\n")
	if err != nil {
		t.Fatalf("planTicketSQL: %v", err)
	}
	if got := NewRiskEvaluator().Evaluate(plan.Analysis).Level; got == "critical" {
		t.Errorf("risk = %q: a DROP mentioned inside an ordinary comment was graded as one", got)
	}
	for _, table := range plan.Analysis.AffectedTables {
		if table == "users" {
			t.Error("a table named only inside an ordinary comment was reported as affected")
		}
	}
}

// TestCheckSLA_ScoresAgainstTheStoredDeadline pins that the clock the platform
// acts on is the one it shows.
//
// CheckSLA measured elapsed time from created_at while SetSLADeadline anchored
// the stored deadline at now+timeout — and its only caller is a ten-minute
// ticker, so the deadline was always set late and the two never agreed. The
// unbounded case is a resubmission: RejectTicket clears the deadline,
// ResubmitTicket keeps the original created_at, so the ticket returns to
// PENDING_APPROVAL already "breached" by a clock nobody could see, gets a fresh
// deadline showing nearly a full window remaining, and is auto-rejected on the
// next pass.
func TestCheckSLA_ScoresAgainstTheStoredDeadline(t *testing.T) {
	ctx := t.Context()
	d := setupSLATestDB(t)
	slaSvc := NewSLAService(d, notify.NewService(notify.Deps{DB: d}))

	submitterID := createTestUserForSLA(t, ctx, d.DB, "sla_clock_submitter")
	dsID := createTestDatasourceForSLA(t, ctx, d.DB)

	if _, err := d.ExecContext(ctx,
		`INSERT INTO sla_config (priority, timeout_minutes, reminder_percent, escalate_to_role, auto_reject_enabled, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"medium", 60, 80, "admin", true, true, time.Now(), time.Now()); err != nil {
		t.Fatalf("create sla config: %v", err)
	}

	// A resubmitted ticket: created long ago, deadline only just set.
	deadline := time.Now().Add(25 * time.Minute)
	ticketID := createTestTicketForSLA(t, ctx, d.DB, submitterID, dsID,
		string(model.TicketStatusPendingApproval),
		time.Now().Add(-72*time.Hour), &deadline)

	if err := slaSvc.CheckSLA(ctx); err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}

	row, err := d.Client().Ticket.Get(ctx, int(ticketID))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if row.SLAStatus == "breached" {
		t.Errorf("sla_status = breached with %s still on the stored deadline",
			time.Until(deadline).Round(time.Minute))
	}
	if row.Status == string(model.TicketStatusRejected) {
		t.Error("the ticket was auto-rejected while its own deadline was still in the future")
	}
}

// TestCheckSLA_StillBreachesAPastDeadline is the other half: the fix must not
// simply stop the clock.
func TestCheckSLA_StillBreachesAPastDeadline(t *testing.T) {
	ctx := t.Context()
	d := setupSLATestDB(t)
	slaSvc := NewSLAService(d, notify.NewService(notify.Deps{DB: d}))

	submitterID := createTestUserForSLA(t, ctx, d.DB, "sla_breach_submitter")
	dsID := createTestDatasourceForSLA(t, ctx, d.DB)

	if _, err := d.ExecContext(ctx,
		`INSERT INTO sla_config (priority, timeout_minutes, reminder_percent, escalate_to_role, auto_reject_enabled, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"medium", 60, 80, "admin", true, true, time.Now(), time.Now()); err != nil {
		t.Fatalf("create sla config: %v", err)
	}

	past := time.Now().Add(-10 * time.Minute)
	ticketID := createTestTicketForSLA(t, ctx, d.DB, submitterID, dsID,
		string(model.TicketStatusPendingApproval),
		time.Now().Add(-2*time.Hour), &past)

	if err := slaSvc.CheckSLA(ctx); err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}

	row, err := d.Client().Ticket.Get(ctx, int(ticketID))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if row.SLAStatus != "breached" {
		t.Errorf("sla_status = %q for a deadline ten minutes past, want breached", row.SLAStatus)
	}
}
