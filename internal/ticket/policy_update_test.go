package ticket

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/testutil"
)

// TestUpdatePolicyLeavesUnnamedFieldsAlone is what "update" has to mean when
// the caller names one field.
//
// updatePolicyRequest had non-pointer fields, so every field the client omitted
// bound to its zero value and was written. Three visible consequences, all
// reachable from the policy admin screen:
//
//   - The enable/disable toggle sends {"enabled":true} and the reorder arrows
//     send {"priority":n}. Everything else arrived as "" or false, so the
//     approval chain was blanked and validation rejected the request — both
//     controls failed every time.
//   - The edit sheet never sends is_default at all, so saving any edit silently
//     cleared the default policy. Nothing then routes a ticket that matches no
//     other policy, which is now a refused creation rather than a stranded one.
func TestUpdatePolicyLeavesUnnamedFieldsAlone(t *testing.T) {
	database := testutil.NewDB(t)
	engine := NewApprovalEngine(database, nil)
	ctx := context.Background()

	original, err := engine.CreatePolicy(ctx, "策略甲", "原始描述", "{}",
		`[{"role":"dba"}]`, true, "低风险自动通过", true, 50)
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	t.Run("toggling_enabled_keeps_everything_else", func(t *testing.T) {
		enabled := false
		got, err := engine.UpdatePolicy(ctx, original.ID, PolicyUpdate{Enabled: &enabled})
		if err != nil {
			t.Fatalf("UpdatePolicy: %v", err)
		}
		if got.Enabled {
			t.Error("enabled was not applied")
		}
		if got.Name != "策略甲" {
			t.Errorf("name = %q, want it untouched", got.Name)
		}
		if got.ApprovalChain != `[{"role":"dba"}]` {
			t.Errorf("approval_chain = %q, want it untouched", got.ApprovalChain)
		}
		if !got.IsDefault {
			t.Error("is_default was cleared by a change to enabled")
		}
		if got.Priority != 50 {
			t.Errorf("priority = %d, want 50", got.Priority)
		}
		if !got.AutoApproveEnabled {
			t.Error("auto_approve_enabled was cleared")
		}
	})

	t.Run("reordering_keeps_everything_else", func(t *testing.T) {
		priority := 90
		got, err := engine.UpdatePolicy(ctx, original.ID, PolicyUpdate{Priority: &priority})
		if err != nil {
			t.Fatalf("UpdatePolicy: %v", err)
		}
		if got.Priority != 90 {
			t.Errorf("priority = %d, want 90", got.Priority)
		}
		if got.Name != "策略甲" || got.ApprovalChain != `[{"role":"dba"}]` || !got.IsDefault {
			t.Errorf("a priority change rewrote other fields: %+v", got)
		}
	})

	t.Run("editing_without_is_default_keeps_it", func(t *testing.T) {
		name := "策略乙"
		chain := `[{"role":"security"},{"role":"dba"}]`
		got, err := engine.UpdatePolicy(ctx, original.ID, PolicyUpdate{
			Name:          &name,
			ApprovalChain: &chain,
		})
		if err != nil {
			t.Fatalf("UpdatePolicy: %v", err)
		}
		if got.Name != "策略乙" || got.ApprovalChain != chain {
			t.Errorf("the named fields were not applied: %+v", got)
		}
		if !got.IsDefault {
			t.Error("is_default was cleared by an edit that never mentioned it")
		}
	})

	t.Run("clearing_is_default_is_still_possible", func(t *testing.T) {
		// The other half: a caller that does name the field must be obeyed,
		// or "leave it alone" would become "you can never turn it off".
		isDefault := false
		got, err := engine.UpdatePolicy(ctx, original.ID, PolicyUpdate{IsDefault: &isDefault})
		if err != nil {
			t.Fatalf("UpdatePolicy: %v", err)
		}
		if got.IsDefault {
			t.Error("is_default was not cleared when the caller asked for it")
		}
	})
}
