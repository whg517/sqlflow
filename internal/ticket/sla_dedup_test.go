package ticket

import (
	"sync"
	"testing"

	"github.com/whg517/sqlflow/internal/testutil"
)

// TestSLATryRecordActionDedupesRepeatedRuns pins the idempotency the SLA
// scheduler depends on.
//
// tryRecordAction decides whether an SLA action has already happened, so a
// duplicate is a duplicate escalation notification — or, with auto-reject
// enabled, a second attempt to reject a ticket. The scheduler re-evaluates
// every ticket on every pass, so it reaches this function with the same key
// many times over a day; only the first may report the action as new.
func TestSLATryRecordActionDedupesRepeatedRuns(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewSLAService(database, nil)

	const dedupKey = "42:escalate:2026-08-04"

	first, err := svc.tryRecordAction(t.Context(), 42, "escalate", dedupKey, "approver", 0)
	if err != nil {
		t.Fatalf("first record: %v", err)
	}
	if !first {
		t.Fatal("the first call did not report the action as recorded")
	}

	for i := range 3 {
		again, err := svc.tryRecordAction(t.Context(), 42, "escalate", dedupKey, "approver", 0)
		if err != nil {
			t.Fatalf("repeat %d: %v", i, err)
		}
		if again {
			t.Errorf("repeat %d reported the action as newly recorded", i)
		}
	}

	rows, err := svc.client.SLAActionLog.Query().All(t.Context())
	if err != nil {
		t.Fatalf("read action log: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("action log holds %d rows, want 1", len(rows))
	}
}

// TestSLATryRecordActionUnderConcurrency asserts the end state when several
// schedulers overlap, which is the normal case during a rolling restart.
//
// It asserts what the caller can rely on — one row, one caller told it recorded
// — rather than claiming to force the interleaving. A read-then-write
// implementation was not observed to fail this on the machines it has run on:
// the goroutines queue for pooled connections and finish close to sequentially.
// The guarantee comes from the unique constraint on dedup_key, which holds
// regardless of how the calls interleave; this checks the code above it reports
// the outcome correctly rather than surfacing the conflict as an error.
func TestSLATryRecordActionUnderConcurrency(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewSLAService(database, nil)

	const attempts = 8
	const dedupKey = "43:escalate:2026-08-04"

	var start, ready, done sync.WaitGroup
	start.Add(1)
	ready.Add(attempts)
	done.Add(attempts)

	results := make([]bool, attempts)
	errs := make([]error, attempts)
	for i := range attempts {
		go func() {
			defer done.Done()
			ready.Done()
			start.Wait()
			results[i], errs[i] = svc.tryRecordAction(
				t.Context(), 43, "escalate", dedupKey, "approver", 0,
			)
		}()
	}
	ready.Wait()
	start.Done()
	done.Wait()

	recorded := 0
	for i, ok := range results {
		if errs[i] != nil {
			t.Errorf("attempt %d: %v", i, errs[i])
		}
		if ok {
			recorded++
		}
	}
	if recorded != 1 {
		t.Errorf("%d of %d attempts reported the action as newly recorded, want exactly 1",
			recorded, attempts)
	}

	rows, err := svc.client.SLAActionLog.Query().All(t.Context())
	if err != nil {
		t.Fatalf("read action log: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("action log holds %d rows, want 1 — the dedup key did not hold", len(rows))
	}
}

// TestSLATryRecordActionAllowsDistinctKeys checks that dedup is scoped to the
// key rather than swallowing every subsequent action on the ticket.
//
// Reminders are keyed per hour, so a service that deduped on ticket id alone
// would send the first reminder and then go silent.
func TestSLATryRecordActionAllowsDistinctKeys(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewSLAService(database, nil)

	for _, key := range []string{"7:reminder:2026-08-04:09", "7:reminder:2026-08-04:10"} {
		ok, err := svc.tryRecordAction(t.Context(), 7, "reminder", key, "approver", 0)
		if err != nil {
			t.Fatalf("record %s: %v", key, err)
		}
		if !ok {
			t.Errorf("key %s was treated as a duplicate", key)
		}
	}
}
