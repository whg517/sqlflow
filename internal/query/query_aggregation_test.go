package query

import (
	"context"
	"errors"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

// Aggregation results carry a driver-native, arbitrarily nested payload that
// the row-oriented masker cannot walk. These tests pin the rule that decides
// whether such a result may be returned at all.

// TestMaskingApplies_WithMatchingRule is the precondition for refusing an
// aggregation: a developer with no unmask grant hits a rule on the target.
func TestMaskingApplies_WithMatchingRule(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx := context.Background()
	dsID := seedQueryDatasource(t, qs.dsSvc, ctx)
	seedMaskRule(t, testDB, dsID, "testdb", "orders", "email", "partial")

	if applies, err := maskingWouldApply(t, qs, ctx, 1, "developer", dsID, []string{"orders"}); err != nil {
		t.Fatalf("maskingApplies: %v", err)
	} else if !applies {
		t.Error("masking should apply: developer has no unmask grant and a rule matches")
	}
}

// TestMaskingApplies_NoRule keeps aggregations working where nothing is
// protected — the guard must not become a blanket ban.
func TestMaskingApplies_NoRule(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()
	dsID := seedQueryDatasource(t, qs.dsSvc, ctx)

	if applies, err := maskingWouldApply(t, qs, ctx, 1, "developer", dsID, []string{"orders"}); err != nil {
		t.Fatalf("maskingApplies: %v", err)
	} else if applies {
		t.Error("masking must not apply when no rule matches the target")
	}
}

// TestMaskingApplies_BypassGrant checks the unmask path: a role allowed to see
// plaintext has nothing to be protected from, so aggregations stay available.
func TestMaskingApplies_BypassGrant(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx := context.Background()
	dsID := seedQueryDatasource(t, qs.dsSvc, ctx)
	seedMaskRule(t, testDB, dsID, "testdb", "orders", "email", "partial")

	// The seeded policy grants dba desensitize:bypass on every object.
	if applies, err := maskingWouldApply(t, qs, ctx, 2, "dba", dsID, []string{"orders"}); err != nil {
		t.Fatalf("maskingApplies: %v", err)
	} else if applies {
		t.Error("masking must not apply for a role holding a bypass grant")
	}
}

// TestMaskingApplies_UnrelatedTable ensures a rule on one target does not block
// aggregations over a different one.
func TestMaskingApplies_UnrelatedTable(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx := context.Background()
	dsID := seedQueryDatasource(t, qs.dsSvc, ctx)
	seedMaskRule(t, testDB, dsID, "testdb", "customers", "email", "partial")

	if applies, err := maskingWouldApply(t, qs, ctx, 1, "developer", dsID, []string{"orders"}); err != nil {
		t.Fatalf("maskingApplies: %v", err)
	} else if applies {
		t.Error("a rule on customers must not mark orders as masked")
	}
}

// TestRefuseUnmaskableShape_CoversBothPaths is the regression test for QA-01.
//
// The query path refused an aggregation when masking rules applied; the export
// path did not, and applyDesensitizationForActor only ever walks result.Rows.
// A user blocked from a protected field at the query entrance could therefore
// read it through export by asking for an aggregation over the same index.
//
// Both entrances must consult one decision, so this asserts on the shared
// helper rather than on either caller: a future third entrance that forgets to
// call it is the failure this cannot catch, which is why the helper exists
// instead of two copies of the condition.
func TestRefuseUnmaskableShape_CoversBothPaths(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx := context.Background()
	dsID := seedQueryDatasource(t, qs.dsSvc, ctx)
	seedMaskRule(t, testDB, dsID, "testdb", "orders", "email", "partial")

	tests := []struct {
		name   string
		shape  driver.ResultShape
		reject bool
	}{
		{"aggregation over a masked target is refused", driver.ShapeAggregation, true},
		{"table results are masked, not refused", driver.ShapeTable, false},
		{"document results are masked, not refused", driver.ShapeDocuments, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := releaseVerdictFor(t, qs, ctx, 1, "developer", dsID, tt.shape, []string{"orders"})
			if tt.reject && !errors.Is(err, ErrAggregationMaskingUnsupported) {
				t.Errorf("error = %v, want ErrAggregationMaskingUnsupported", err)
			}
			if !tt.reject && err != nil {
				t.Errorf("error = %v, want nil", err)
			}
		})
	}
}

// TestRefuseUnmaskableShape_NoRuleAllowsAggregation keeps the guard from
// becoming a blanket ban on aggregations.
func TestRefuseUnmaskableShape_NoRuleAllowsAggregation(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()
	dsID := seedQueryDatasource(t, qs.dsSvc, ctx)

	if err := releaseVerdictFor(t, qs, ctx, 1, "developer", dsID,
		driver.ShapeAggregation, []string{"orders"}); err != nil {
		t.Errorf("aggregation with no matching rule was refused: %v", err)
	}
}

// maskingWouldApply reports whether masking bears on these targets for this
// actor — the question the aggregation refusal turns on.
//
// Deliberately not "were any rows rewritten": a rule can protect a column this
// particular result did not select, and the aggregation refusal still has to
// fire, because the payload it cannot walk may expose that column through a
// bucket key. releaseRows makes the same distinction internally; this composes
// the two pieces it uses so the test asserts the same thing the decision does.
func maskingWouldApply(t *testing.T, qs *Service, ctx context.Context, userID int64, role string, dsID int64, tables []string) (bool, error) {
	t.Helper()
	// If the actor can unmask every table in the query, masking never applies.
	allUnmasked := true
	for _, table := range tables {
		if !mayUnmaskTable(ctx, qs.permSvc, releaseActor{UserID: userID, Role: role}, dsID, table) {
			allUnmasked = false
			break
		}
	}
	if allUnmasked {
		return false, nil
	}
	return anyRuleProtects(ctx, qs.client, dsID, "testdb", tables)
}

// releaseVerdictFor returns the refusal a shape produces, or nil.
func releaseVerdictFor(t *testing.T, qs *Service, ctx context.Context, userID int64, role string, dsID int64, shape driver.ResultShape, tables []string) error {
	t.Helper()
	rows := []map[string]interface{}{{"email": "a@b.c"}}
	d, err := releaseRows(ctx, qs.client, qs.permSvc, releaseActor{UserID: userID, Role: role},
		dsID, "testdb", tables, shape, rows)
	if err != nil {
		return err
	}
	if d.Verdict == releaseRefuse {
		return d.Reason
	}
	return nil
}
