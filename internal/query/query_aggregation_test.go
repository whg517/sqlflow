package query

import (
	"context"
	"testing"
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

	if !qs.maskingApplies(ctx, 1, "developer", dsID, "testdb", []string{"orders"}) {
		t.Error("masking should apply: developer has no unmask grant and a rule matches")
	}
}

// TestMaskingApplies_NoRule keeps aggregations working where nothing is
// protected — the guard must not become a blanket ban.
func TestMaskingApplies_NoRule(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()
	dsID := seedQueryDatasource(t, qs.dsSvc, ctx)

	if qs.maskingApplies(ctx, 1, "developer", dsID, "testdb", []string{"orders"}) {
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
	if qs.maskingApplies(ctx, 2, "dba", dsID, "testdb", []string{"orders"}) {
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

	if qs.maskingApplies(ctx, 1, "developer", dsID, "testdb", []string{"orders"}) {
		t.Error("a rule on customers must not mark orders as masked")
	}
}
