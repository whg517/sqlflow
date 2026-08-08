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

	if applies, err := qs.maskingApplies(ctx, 1, "developer", dsID, "testdb", []string{"orders"}); err != nil {
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

	if applies, err := qs.maskingApplies(ctx, 1, "developer", dsID, "testdb", []string{"orders"}); err != nil {
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
	if applies, err := qs.maskingApplies(ctx, 2, "dba", dsID, "testdb", []string{"orders"}); err != nil {
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

	if applies, err := qs.maskingApplies(ctx, 1, "developer", dsID, "testdb", []string{"orders"}); err != nil {
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
			err := qs.refuseUnmaskableShape(ctx, tt.shape, 1, "developer", dsID, "testdb", []string{"orders"})
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

	if err := qs.refuseUnmaskableShape(ctx, driver.ShapeAggregation,
		1, "developer", dsID, "testdb", []string{"orders"}); err != nil {
		t.Errorf("aggregation with no matching rule was refused: %v", err)
	}
}
