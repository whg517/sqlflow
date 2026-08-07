package notify

import (
	"testing"

	"github.com/whg517/sqlflow/internal/testutil"
)

// newSubscriptionService returns a service over a schema of its own.
func newSubscriptionService(t *testing.T) *WebhookSubscriptionService {
	t.Helper()
	return NewWebhookSubscriptionService(testutil.NewDB(t), testutil.EncryptionKey)
}

// seedSubscription creates an enabled subscription and returns its id.
func seedSubscription(t *testing.T, svc *WebhookSubscriptionService, name string) int64 {
	t.Helper()
	sub, _, err := svc.Create(t.Context(), CreateSubscriptionRequest{
		Name:   name,
		URL:    "https://example.com/hook",
		Events: []string{"ticket.created"},
	}, 1, "tester")
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	return sub.ID
}

// TestSubscriptionToggle covers enabling and disabling a subscription.
//
// The update wrote the enabled flag through boolToInt, which PostgreSQL rejects
// for a boolean column — so toggling failed every time, and nothing exercised
// it. Disabling a misbehaving webhook is the operator's only lever short of
// deleting it.
func TestSubscriptionToggle(t *testing.T) {
	svc := newSubscriptionService(t)
	id := seedSubscription(t, svc, "toggle-me")

	disabled, err := svc.Toggle(t.Context(), id, 1, "tester")
	if err != nil {
		t.Fatalf("Toggle to disabled: %v", err)
	}
	if disabled.Enabled {
		t.Error("subscription still reports enabled after the first toggle")
	}

	enabled, err := svc.Toggle(t.Context(), id, 1, "tester")
	if err != nil {
		t.Fatalf("Toggle to enabled: %v", err)
	}
	if !enabled.Enabled {
		t.Error("subscription still reports disabled after the second toggle")
	}
}

// TestSubscriptionToggleResetsFailureCountOnEnable checks the reset that makes
// re-enabling meaningful.
//
// Without it a subscription that was auto-disabled would come back with its
// failure count at the threshold and disable itself again on the next failure.
func TestSubscriptionToggleResetsFailureCountOnEnable(t *testing.T) {
	svc := newSubscriptionService(t)
	id := seedSubscription(t, svc, "reset-me")

	for range MaxConsecutiveFailures {
		svc.handleFailure(id)
	}

	afterFailures, err := svc.GetByID(t.Context(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if afterFailures.Enabled {
		t.Fatalf("subscription is still enabled after %d consecutive failures", MaxConsecutiveFailures)
	}

	reEnabled, err := svc.Toggle(t.Context(), id, 1, "tester")
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if !reEnabled.Enabled {
		t.Fatal("subscription did not come back enabled")
	}
	if reEnabled.FailureCount != 0 {
		t.Errorf("failure count = %d after re-enabling, want 0", reEnabled.FailureCount)
	}
}

// TestSubscriptionAutoDisablesAfterRepeatedFailures pins the circuit breaker.
//
// It compared the enabled column by scanning it into an int, which PostgreSQL
// will not do for a boolean — so the comparison always failed and a dead
// endpoint was retried forever.
func TestSubscriptionAutoDisablesAfterRepeatedFailures(t *testing.T) {
	svc := newSubscriptionService(t)
	id := seedSubscription(t, svc, "breaker")

	for i := range MaxConsecutiveFailures - 1 {
		svc.handleFailure(id)
		sub, err := svc.GetByID(t.Context(), id)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !sub.Enabled {
			t.Fatalf("disabled after %d failures, want it to survive until %d", i+1, MaxConsecutiveFailures)
		}
	}

	svc.handleFailure(id)
	sub, err := svc.GetByID(t.Context(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if sub.Enabled {
		t.Errorf("still enabled after %d consecutive failures", MaxConsecutiveFailures)
	}
	if sub.FailureCount != MaxConsecutiveFailures {
		t.Errorf("failure count = %d, want %d", sub.FailureCount, MaxConsecutiveFailures)
	}
}

// TestSubscriptionSuccessClearsFailures checks that a delivery that lands
// resets the breaker.
//
// A subscription that fails intermittently must not accumulate its way to
// disabled over days.
func TestSubscriptionSuccessClearsFailures(t *testing.T) {
	svc := newSubscriptionService(t)
	id := seedSubscription(t, svc, "recovers")

	svc.handleFailure(id)
	svc.handleFailure(id)
	svc.handleSuccess(id)

	sub, err := svc.GetByID(t.Context(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if sub.FailureCount != 0 {
		t.Errorf("failure count = %d after a success, want 0", sub.FailureCount)
	}
	if sub.LastTriggeredAt == nil {
		t.Error("last_triggered_at was not recorded")
	}
}
