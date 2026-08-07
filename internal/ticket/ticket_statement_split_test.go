package ticket

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
)

// These pin the invariant the whole ticket workflow rests on: the statement set
// that risk was derived from is the statement set that runs.
//
// splitStatements was strings.Split(sqlContent, ";") and the risk analyzer read
// only the first keyword, so the two disagreed in three separate ways. Each is
// reproduced below with the behavior measured before the fix.

// TestRiskCoversEveryStatementThatWillRun is the one that matters most.
//
// Risk selects the approval policy, including auto-approval. Prefixing any
// statement with "SELECT 1;" moved a ticket from critical to low while the
// executor still ran the second statement — so the submitter chose their own
// approval path by editing the SQL body, which is the thing the platform
// exists to prevent.
func TestRiskCoversEveryStatementThatWillRun(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{"drop behind a select", "SELECT 1; DROP TABLE users"},
		{"truncate behind a select", "SELECT 1; TRUNCATE TABLE orders"},
		{"drop behind two selects", "SELECT 1;\nSELECT 2;\nDROP TABLE users"},
		{"delete behind a comment", "-- routine cleanup\nSELECT count(*) FROM logs; DELETE FROM logs"},
	}

	// Graded the way a ticket is graded: planTicketSQL splits first, then folds
	// every statement's analysis. Calling analyzeTicketSQL directly would grade
	// the body as one statement, which is the defect rather than the fix.
	grade := func(t *testing.T, sql string) string {
		t.Helper()
		plan, err := planTicketSQL("mysql", sql)
		if err != nil {
			t.Fatalf("planTicketSQL(%q): %v", sql, err)
		}
		return NewRiskEvaluator().Evaluate(plan.Analysis).Level
	}

	alone := grade(t, "DROP TABLE users")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := grade(t, tt.sql)
			if got == RiskLevelLow {
				t.Errorf("risk = low for %q, which runs a statement that scores %s on its own",
					tt.sql, alone)
			}
		})
	}
}

// TestALiteralIsNotAStatement is the case that tells a correct splitter from a
// naive one.
//
// The other risk cases do not: "SELECT 1; DROP TABLE users" happens to split
// correctly on ';' too, so they pass either way. Here the DROP is inside a
// string, and cutting on ';' invents a statement that does not exist —
// upgrading a plain SELECT to critical and sending an approver a change that
// was never proposed.
func TestALiteralIsNotAStatement(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{"drop inside a literal", `SELECT * FROM t WHERE note = 'x; DROP TABLE users'`},
		{"truncate inside a literal", `UPDATE t SET note = 'y; TRUNCATE TABLE orders' WHERE id = 1`},
		{"drop inside a comment", "SELECT 1 /* todo: ; DROP TABLE users */"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planTicketSQL("mysql", tt.sql)
			if err != nil {
				t.Fatalf("planTicketSQL: %v", err)
			}
			if len(plan.Statements) != 1 {
				t.Fatalf("split into %d statements %q, want 1", len(plan.Statements), plan.Statements)
			}
			for _, tbl := range plan.Analysis.AffectedTables {
				if tbl == "users" || tbl == "orders" {
					t.Errorf("affected tables %v include %q, which only appears inside a literal",
						plan.Analysis.AffectedTables, tbl)
				}
			}
			if plan.Analysis.SQLType == "DROP" || plan.Analysis.SQLType == "TRUNCATE" {
				t.Errorf("sql_type = %q for a statement that is not one", plan.Analysis.SQLType)
			}
		})
	}
}

// TestSemicolonInsideALiteralIsNotATerminator covers the input the platform
// wrongly rejected rather than wrongly allowed.
//
// "WHERE name = 'a;b'" was cut into "WHERE name = 'a" and "b'", two fragments
// that are not SQL. The user sees a syntax error for a statement they wrote
// correctly.
func TestSemicolonInsideALiteralIsNotATerminator(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{"single quotes", `SELECT * FROM t WHERE name = 'a;b'`},
		{"semicolon only", `SELECT * FROM t WHERE tag = ';'`},
		{"escaped quote around it", `SELECT * FROM t WHERE note = 'it''s; here'`},
		{"line comment", "SELECT 1 -- trailing ; comment"},
		{"block comment", "SELECT /* a ; b */ 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := driver.SplitStatementsFor("mysql", tt.sql)
			if err != nil {
				t.Fatalf("split %q: %v", tt.sql, err)
			}
			if len(got) != 1 {
				t.Errorf("split %q into %d statements %q, want 1", tt.sql, len(got), got)
			}
		})
	}
}

// TestDollarQuotedBodyStaysOneStatement is the partial-execution case.
//
// A PostgreSQL function body is full of semicolons. Splitting on them produced
// three fragments; running them in order creates a half-defined function and
// leaves the database in a state the ticket never described.
func TestDollarQuotedBodyStaysOneStatement(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "untagged",
			sql:  "CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql",
		},
		{
			name: "tagged",
			sql:  "CREATE FUNCTION g() RETURNS int AS $body$ BEGIN RETURN 2; END; $body$ LANGUAGE plpgsql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := driver.SplitStatementsFor("postgresql", tt.sql)
			if err != nil {
				t.Fatalf("split: %v", err)
			}
			if len(got) != 1 {
				t.Errorf("split into %d statements %q, want 1", len(got), got)
			}
		})
	}
}

// TestGenuineMultiStatementStillSplits guards against a fix that solves the
// problem by refusing everything. Batched DDL is a real and supported use.
func TestGenuineMultiStatementStillSplits(t *testing.T) {
	sql := "ALTER TABLE t ADD COLUMN a int; ALTER TABLE t ADD COLUMN b int;"
	got, err := driver.SplitStatementsFor("mysql", sql)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("split into %d statements %q, want 2", len(got), got)
	}
}
