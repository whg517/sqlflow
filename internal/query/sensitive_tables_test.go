package query

import (
	"slices"
	"testing"

	"github.com/whg517/sqlflow/internal/testutil"
)

// newSensitiveTableService returns an AI review service over a fresh schema.
//
// The lookup lives on this service because it is what grades a ticket's risk;
// it used to sit in internal/platform/sqlparser, which took a *sql.DB purely to
// run it.
func newSensitiveTableService(t *testing.T) *AIReviewService {
	t.Helper()
	return NewAIReviewService(testutil.NewDB(t), "openai", "test-model", "", "", 0)
}

// seedMaskRule marks one field of one table as masked.
func seedAIMaskRule(t *testing.T, svc *AIReviewService, datasourceID int64, table, field string) {
	t.Helper()
	if err := svc.entClient.MaskRule.Create().
		SetDatasourceID(datasourceID).
		SetDatabase("").
		SetTableName(table).
		SetField(field).
		SetMaskType("phone").
		Exec(t.Context()); err != nil {
		t.Fatalf("seed mask rule %s.%s: %v", table, field, err)
	}
}

// TestSensitiveTables checks which of a statement's tables carry masking rules.
//
// The answer drives the ticket's risk grade, so a false negative routes a
// change touching masked data through the low-risk path.
func TestSensitiveTables(t *testing.T) {
	svc := newSensitiveTableService(t)
	seedAIMaskRule(t, svc, 1, "users", "phone")
	seedAIMaskRule(t, svc, 1, "orders", "credit_card")
	seedAIMaskRule(t, svc, 2, "users", "email")

	tests := []struct {
		name         string
		tables       []string
		datasourceID int64
		want         []string
	}{
		{"matches_sensitive_tables", []string{"users", "products", "orders"}, 1, []string{"orders", "users"}},
		{"no_matches", []string{"products", "categories"}, 1, nil},
		{"different_datasource", []string{"users"}, 2, []string{"users"}},
		// The rule belongs to datasource 1, so the same table name under
		// datasource 2 must not match: masking is scoped per datasource.
		{"wrong_datasource_no_match", []string{"orders"}, 2, nil},
		{"empty_tables", []string{}, 1, nil},
		{"single_table_match", []string{"users"}, 1, []string{"users"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.sensitiveTables(t.Context(), tt.tables, tt.datasourceID)
			if err != nil {
				t.Fatalf("sensitiveTables: %v", err)
			}
			if tt.want == nil {
				if len(got) > 0 {
					t.Errorf("sensitiveTables = %v, want none", got)
				}
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("sensitiveTables = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSensitiveTablesDedupes checks that a table with several masked fields is
// reported once.
//
// It reaches the user as a warning list; naming the same table three times
// because it has three masked columns reads as three findings.
func TestSensitiveTablesDedupes(t *testing.T) {
	svc := newSensitiveTableService(t)
	seedAIMaskRule(t, svc, 1, "users", "phone")
	seedAIMaskRule(t, svc, 1, "users", "email")

	got, err := svc.sensitiveTables(t.Context(), []string{"users"}, 1)
	if err != nil {
		t.Fatalf("sensitiveTables: %v", err)
	}
	if len(got) != 1 || got[0] != "users" {
		t.Errorf("sensitiveTables = %v, want exactly [users]", got)
	}
}
