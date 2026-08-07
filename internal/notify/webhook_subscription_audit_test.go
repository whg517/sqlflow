package notify

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/db/ent/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

// TestSubscriptionAuditRecordsTheActor pins who a change is attributed to.
//
// Every call site passed 0 for the user id and stuffed the username into
// error_message, a column meant for failures. So the audit rows for webhook
// subscription changes had no actor: filtering the audit log by user never
// returned them, and the per-user reports counted them against nobody.
//
// The handler made it worse by defaulting the name to "admin" when the context
// carried none — an audit trail that invents an actor is worse than one that
// admits it does not know.
func TestSubscriptionAuditRecordsTheActor(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewWebhookSubscriptionService(database, testutil.EncryptionKey)

	const actorID = int64(42)
	_, _, err := svc.Create(context.Background(), CreateSubscriptionRequest{
		Name:   "ops-alerts",
		URL:    "https://hooks.example.com/abc",
		Events: []string{"ticket.created"},
	}, actorID, "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rows, err := database.Client().AuditLog.Query().
		Where(auditlog.ActionEQ("webhook_subscription.create")).
		All(context.Background())
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(rows))
	}
	if rows[0].UserID != actorID {
		t.Errorf("user_id = %d, want %d — the change is attributed to nobody",
			rows[0].UserID, actorID)
	}
}
