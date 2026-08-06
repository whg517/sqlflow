package ticket

import (
	"context"
	"sync"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/testutil"
)

// TestApplyPolicy_ConcurrentIsSingleWinner is the regression test for QA-02.
//
// ApplyPolicy moved a ticket into APPROVED or PENDING_APPROVAL with a bare
// UpdateOneID — a read-modify-write with no precondition on the state it
// believed it was leaving. Two concurrent applications therefore both
// "succeeded", which is the same defect that once let four concurrent approvals
// through and is why invariant 2 requires compare-and-swap.
//
// Exactly one caller must observe the transition.
func TestApplyPolicy_ConcurrentIsSingleWinner(t *testing.T) {
	d := testutil.NewDB(t)
	engine := NewApprovalEngine(d, nil)

	uid := testutil.SeedUser(t, d.DB, "cas_submitter", "developer")
	dsID := testutil.SeedDatasource(t, d.DB, "cas-ds")

	policy, err := engine.CreatePolicy(context.Background(),
		"auto", "auto approve everything", `{}`, `[]`, true, "test", true, 1)
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	svc := New(Deps{DB: d})
	tk, err := svc.CreateTicket(context.Background(), uid, "developer", dsID,
		"appdb", "ALTER TABLE t ADD c INT", "cas probe")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if tk.Status != model.TicketStatusSubmitted {
		t.Fatalf("ticket status = %s, want SUBMITTED", tk.Status)
	}

	const racers = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		applied  int
		firstErr error
	)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := engine.ApplyPolicy(context.Background(), tk.ID, policy, uid)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				if firstErr == nil {
					firstErr = err
				}
			case res != nil && res.Applied:
				applied++
			}
		}()
	}
	close(start)
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("ApplyPolicy returned an error: %v", firstErr)
	}
	if applied != 1 {
		t.Errorf("%d concurrent applications succeeded, want exactly 1", applied)
	}

	// And the ticket must have moved exactly once.
	var records int
	if err := d.QueryRow(
		`SELECT count(*) FROM approval_records WHERE ticket_id = $1`, tk.ID,
	).Scan(&records); err != nil {
		t.Fatalf("count approval records: %v", err)
	}
	if records != 1 {
		t.Errorf("approval_records for the ticket = %d, want 1", records)
	}
}
