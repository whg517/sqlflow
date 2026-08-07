package sqlparser

import (
	"errors"
	"strings"
	"testing"
)

// TestSplitMySQLDialect covers every construct where a ';' does not terminate a
// statement, and every construct where one does.
//
// Each case names the consequence of getting it wrong, because the failure of a
// splitter is never "the split is untidy" — it is either a live statement no
// approver read, or a valid statement the platform refuses.
func TestSplitMySQLDialect(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "plain multi-statement splits",
			body: "SELECT 1; DROP TABLE users",
			want: []string{"SELECT 1", "DROP TABLE users"},
		},
		{
			name: "trailing delimiter produces no empty statement",
			body: "ALTER TABLE t ADD COLUMN a int; ALTER TABLE t ADD COLUMN b int;",
			want: []string{"ALTER TABLE t ADD COLUMN a int", "ALTER TABLE t ADD COLUMN b int"},
		},
		{
			name: "repeated delimiters collapse",
			body: "SELECT 1;; SELECT 2;",
			want: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name: "semicolon inside a single-quoted literal",
			body: `SELECT * FROM t WHERE name = 'a;b'`,
			want: []string{`SELECT * FROM t WHERE name = 'a;b'`},
		},
		{
			name: "doubled quote inside a literal",
			body: `SELECT * FROM t WHERE note = 'it''s; here'`,
			want: []string{`SELECT * FROM t WHERE note = 'it''s; here'`},
		},
		{
			name: "backslash-escaped quote inside a literal",
			body: `SELECT * FROM t WHERE note = 'a\'; DROP TABLE users; --'`,
			want: []string{`SELECT * FROM t WHERE note = 'a\'; DROP TABLE users; --'`},
		},
		{
			name: "semicolon inside a double-quoted literal",
			body: `SELECT * FROM t WHERE name = "a;b"`,
			want: []string{`SELECT * FROM t WHERE name = "a;b"`},
		},
		{
			name: "semicolon inside a backtick identifier",
			body: "SELECT `weird;name` FROM t",
			want: []string{"SELECT `weird;name` FROM t"},
		},
		{
			name: "semicolon inside a bracket identifier (SQLite)",
			body: "SELECT [weird;name] FROM t",
			want: []string{"SELECT [weird;name] FROM t"},
		},
		{
			name: "semicolon inside a line comment",
			body: "SELECT 1 -- trailing ; comment",
			want: []string{"SELECT 1 -- trailing ; comment"},
		},
		{
			name: "semicolon inside a hash comment",
			body: "SELECT 1 # trailing ; comment",
			want: []string{"SELECT 1 # trailing ; comment"},
		},
		{
			name: "semicolon inside a block comment",
			body: "SELECT /* a ; b */ 1",
			want: []string{"SELECT /* a ; b */ 1"},
		},
		{
			name: "multi-line block comment header",
			body: "/* ticket 123\n approved by dba */\nDROP TABLE users",
			want: []string{"/* ticket 123\n approved by dba */\nDROP TABLE users"},
		},
		{
			name: "line comment does not swallow the next statement",
			body: "SELECT 1; -- note\nDROP TABLE users",
			want: []string{"SELECT 1", "-- note\nDROP TABLE users"},
		},
		{
			// a-- is a decrement in some dialects and not a comment in MySQL
			// without the space, so the ';' still terminates.
			name: "double dash without whitespace is not a comment",
			body: "SELECT 1--x; SELECT 2",
			want: []string{"SELECT 1--x", "SELECT 2"},
		},
		{
			// The server executes what is inside an executable comment: the
			// vendored parser reads this as a DropTableStmt. Treating it as a
			// comment would hide the DROP from the analyzer.
			name: "version comment is code, not a comment",
			body: "/*!50000 DROP TABLE users */",
			want: []string{"/*!50000 DROP TABLE users */"},
		},
		{
			name: "delimiter directive wraps a routine into one statement",
			body: "DELIMITER $$\nCREATE PROCEDURE p() BEGIN INSERT INTO a VALUES (1); DELETE FROM b; END$$\nDELIMITER ;",
			want: []string{"CREATE PROCEDURE p() BEGIN INSERT INTO a VALUES (1); DELETE FROM b; END"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitMySQLDialect(tt.body)
			if err != nil {
				t.Fatalf("SplitMySQLDialect: %v", err)
			}
			assertStatements(t, got, tt.want)
		})
	}
}

// TestSplitMySQLRefusesAnUndelimitedRoutine is the worst construct in the
// enumeration, and the one case where refusing is the only safe answer.
//
// A lexical scanner cuts CREATE PROCEDURE ... BEGIN a; b; END into three. The
// first fragment is not a valid statement, and the second is a live
// unconditional DELETE that reaches the database as an ordinary statement no
// approver ever saw as one.
func TestSplitMySQLRefusesAnUndelimitedRoutine(t *testing.T) {
	body := "CREATE PROCEDURE p() BEGIN INSERT INTO a VALUES (1); DELETE FROM b; END"

	_, err := SplitMySQLDialect(body)
	if !errors.Is(err, ErrUndelimitable) {
		t.Fatalf("err = %v, want ErrUndelimitable — the body would have been cut into runnable fragments", err)
	}
}

// TestSplitMySQLRefusesEmptyInput keeps "nothing executable" distinct from
// "nothing to do": a caller that reads an empty slice as success proceeds.
func TestSplitMySQLRefusesEmptyInput(t *testing.T) {
	for _, body := range []string{"", "   \n\t ", "-- just a note", "/* nothing */"} {
		if _, err := SplitMySQLDialect(body); !errors.Is(err, ErrNoStatement) {
			t.Errorf("SplitMySQLDialect(%q) err = %v, want ErrNoStatement", body, err)
		}
	}
}

// TestSplitPostgreSQLDialect covers the dollar-quoting rules that make a
// PostgreSQL routine body indivisible by any lexer that does not know them.
func TestSplitPostgreSQLDialect(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "plain multi-statement splits",
			body: "SELECT 1; DROP TABLE users",
			want: []string{"SELECT 1", "DROP TABLE users"},
		},
		{
			name: "untagged dollar-quoted body stays whole",
			body: "CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql",
			want: []string{"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql"},
		},
		{
			name: "tagged dollar-quoted body stays whole",
			body: "CREATE FUNCTION g() RETURNS int AS $body$ BEGIN RETURN 2; END; $body$ LANGUAGE plpgsql",
			want: []string{"CREATE FUNCTION g() RETURNS int AS $body$ BEGIN RETURN 2; END; $body$ LANGUAGE plpgsql"},
		},
		{
			name: "a statement after a routine still splits off",
			body: "CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql; DROP TABLE users",
			want: []string{
				"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql",
				"DROP TABLE users",
			},
		},
		{
			name: "semicolon inside a literal",
			body: `SELECT * FROM t WHERE name = 'a;b'`,
			want: []string{`SELECT * FROM t WHERE name = 'a;b'`},
		},
		{
			name: "batched DDL splits",
			body: "ALTER TABLE t ADD COLUMN a int; ALTER TABLE t ADD COLUMN b int;",
			want: []string{"ALTER TABLE t ADD COLUMN a int", "ALTER TABLE t ADD COLUMN b int"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitPostgreSQLDialect(tt.body)
			if err != nil {
				t.Fatalf("SplitPostgreSQLDialect: %v", err)
			}
			assertStatements(t, got, tt.want)
		})
	}
}

// TestSplitPostgreSQLRefusesEmptyInput mirrors the MySQL case. libpg_query
// returns zero statements for text it does not recognize as SQL at all, and
// zero must not read as "nothing to do".
func TestSplitPostgreSQLRefusesEmptyInput(t *testing.T) {
	for _, body := range []string{"", "   ", "-- just a note"} {
		if _, err := SplitPostgreSQLDialect(body); !errors.Is(err, ErrNoStatement) {
			t.Errorf("SplitPostgreSQLDialect(%q) err = %v, want ErrNoStatement", body, err)
		}
	}
}

// assertStatements compares texts, not counts. A count-only assertion passes
// for a splitter that emits fragments beginning with a stray delimiter, which
// the server then rejects.
func assertStatements(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("split into %d statements %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range got {
		if strings.TrimSpace(got[i]) != strings.TrimSpace(want[i]) {
			t.Errorf("statement %d = %q, want %q", i, got[i], want[i])
		}
		if strings.HasPrefix(strings.TrimSpace(got[i]), ";") {
			t.Errorf("statement %d begins with a delimiter: %q", i, got[i])
		}
	}
}
