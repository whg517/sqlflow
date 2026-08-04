package notify

import (
	"testing"

	"github.com/whg517/sqlflow/internal/testutil"
)

// TestNotificationDedupSuppressesRepeats pins the idempotency every ticket
// notification goes through.
//
// The service asks shouldNotify before sending and records afterwards, so a
// recording step that silently fails turns every re-trigger into another
// message to the submitter. The only existing coverage ran with no database
// attached, which is the branch that deliberately skips dedup — so the path
// that actually does the work was never exercised.
func TestNotificationDedupSuppressesRepeats(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewService(Deps{DB: database})

	const ticketID = int64(7)
	const eventType = "ticket_created"

	if !svc.shouldNotify(t.Context(), ticketID, eventType) {
		t.Fatal("the first notification was suppressed")
	}
	svc.recordNotification(t.Context(), ticketID, eventType)

	if svc.shouldNotify(t.Context(), ticketID, eventType) {
		t.Error("a repeat notification was allowed through — the record did not persist")
	}
}

// TestNotificationDedupIsScopedToTicketAndEvent checks the key is both columns.
//
// A dedup keyed on the ticket alone would send the first event and then go
// silent for approval, execution and failure.
func TestNotificationDedupIsScopedToTicketAndEvent(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewService(Deps{DB: database})

	svc.recordNotification(t.Context(), 7, "ticket_created")

	if !svc.shouldNotify(t.Context(), 7, "ticket_approved") {
		t.Error("a different event on the same ticket was suppressed")
	}
	if !svc.shouldNotify(t.Context(), 8, "ticket_created") {
		t.Error("the same event on a different ticket was suppressed")
	}
}

// TestNotificationRecordIsIdempotent checks that recording twice is not an
// error.
//
// Two workers can reach the same notification concurrently; the unique index is
// what makes exactly one row land, and the second caller must treat the
// conflict as success rather than logging a failure.
func TestNotificationRecordIsIdempotent(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewService(Deps{DB: database})

	for range 3 {
		svc.recordNotification(t.Context(), 9, "ticket_created")
	}

	count, err := database.Client().TicketNotificationLog.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count notification logs: %v", err)
	}
	if count != 1 {
		t.Errorf("notification log holds %d rows, want 1", count)
	}
}
