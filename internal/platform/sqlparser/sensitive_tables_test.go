package sqlparser_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/whg517/sqlflow/internal/platform/sqlparser"
	"github.com/whg517/sqlflow/internal/testutil"
)

// ---------------------------------------------------------------------------
// CheckSensitiveTables — integration tests against a migrated schema
// ---------------------------------------------------------------------------

// setupMaskRulesDB returns a real migrated schema.
//
// It used to CREATE TABLE mask_rules by hand in an in-memory SQLite database.
// That is the arrangement that lets a test keep passing after the production
// column it exercises has been renamed.
// setupMaskRulesDB returns a real migrated schema.
//
// It used to CREATE TABLE mask_rules by hand in an in-memory SQLite database —
// the arrangement that lets a test keep passing after the production column it
// exercises has been renamed.
//
// These cases live in the external test package because testutil registers
// every datasource driver, one of which parses queries through this package.
// Importing it from inside sqlparser would be a cycle.
func setupMaskRulesDB(t *testing.T) *sql.DB {
	t.Helper()
	return testutil.NewDB(t).DB
}

func TestCheckSensitiveTables(t *testing.T) {
	db := setupMaskRulesDB(t)
	defer db.Close()

	_, err := db.Exec(`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type) VALUES (1, '', 'users', 'phone', 'phone')`)
	if err != nil {
		t.Fatalf("insert mask_rule: %v", err)
	}
	_, err = db.Exec(`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type) VALUES (1, '', 'orders', 'credit_card', 'credit_card')`)
	if err != nil {
		t.Fatalf("insert mask_rule: %v", err)
	}
	_, err = db.Exec(`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type) VALUES (2, '', 'users', 'email', 'email')`)
	if err != nil {
		t.Fatalf("insert mask_rule: %v", err)
	}

	tests := []struct {
		name         string
		tables       []string
		datasourceID int
		want         []string
	}{
		{"matches_sensitive_tables", []string{"users", "products", "orders"}, 1, []string{"orders", "users"}},
		{"no_matches", []string{"products", "categories"}, 1, nil},
		{"different_datasource", []string{"users"}, 2, []string{"users"}},
		{"wrong_datasource_no_match", []string{"orders"}, 2, nil},
		{"empty_tables", []string{}, 1, nil},
		{"single_table_match", []string{"users"}, 1, []string{"users"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sqlparser.CheckSensitiveTables(context.Background(), db, tt.tables, tt.datasourceID)
			if err != nil {
				t.Fatalf("sqlparser.CheckSensitiveTables() error = %v", err)
			}
			if tt.want == nil && len(got) > 0 {
				t.Errorf("sqlparser.CheckSensitiveTables() = %v, want nil/empty", got)
			}
			if tt.want != nil && !slices.Equal(got, tt.want) {
				t.Errorf("sqlparser.CheckSensitiveTables() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckSensitiveTables_NilDB(t *testing.T) {
	got, err := sqlparser.CheckSensitiveTables(context.Background(), nil, []string{"users"}, 1)
	if err != nil {
		t.Fatalf("CheckSensitiveTables with nil db: %v", err)
	}
	if got != nil {
		t.Errorf("CheckSensitiveTables with nil db = %v, want nil", got)
	}
}

func TestCheckSensitiveTables_EmptyTables(t *testing.T) {
	db := setupMaskRulesDB(t)
	defer db.Close()

	got, err := sqlparser.CheckSensitiveTables(context.Background(), db, nil, 1)
	if err != nil {
		t.Fatalf("CheckSensitiveTables with nil tables: %v", err)
	}
	if got != nil {
		t.Errorf("CheckSensitiveTables with nil tables = %v, want nil", got)
	}
}

func TestCheckSensitiveTables_DistinctDedup(t *testing.T) {
	db := setupMaskRulesDB(t)
	defer db.Close()

	// Insert two rules for same table+datasource — should deduplicate
	_, _ = db.Exec(`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type) VALUES (1, '', 'users', 'phone', 'phone')`)
	_, _ = db.Exec(`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type) VALUES (1, '', 'users', 'email', 'email')`)

	got, err := sqlparser.CheckSensitiveTables(context.Background(), db, []string{"users"}, 1)
	if err != nil {
		t.Fatalf("sqlparser.CheckSensitiveTables() error = %v", err)
	}
	if len(got) != 1 || got[0] != "users" {
		t.Errorf("sqlparser.CheckSensitiveTables() = %v, want exactly [users] (deduplicated)", got)
	}
}

// ---------------------------------------------------------------------------
// extractOperationRegex — direct unit tests
// ---------------------------------------------------------------------------
