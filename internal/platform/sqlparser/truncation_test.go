package sqlparser

import (
	"strings"
	"testing"
)

// TestParseSeesTheWholeInput pins the other half of the split defect.
//
// ParseMySQL and ParsePostgreSQL each began with
//
//	if idx := strings.IndexByte(sql, ';'); idx >= 0 { sql = sql[:idx] }
//
// so a caller handing over a multi-statement body got an answer about the first
// statement and no indication that the rest existed. The ticket path then
// executed all of them. Truncation at a byte also cut string literals in half,
// which turned a valid single statement into a syntax error.
//
// A parser may reasonably refuse a multi-statement body, or describe all of it.
// What it may not do is silently describe a prefix.
func TestParseSeesTheWholeInput(t *testing.T) {
	tests := []struct {
		name  string
		parse func(string) (*SQLParseResult, error)
		sql   string
	}{
		{"mysql drop behind select", ParseMySQLDialect, "SELECT 1; DROP TABLE users"},
		{"postgres drop behind select", ParsePostgreSQLDialect, "SELECT 1; DROP TABLE users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.parse(tt.sql)
			if err != nil {
				return // refusing the body is an acceptable answer
			}
			if result.Operation == OpSelect {
				t.Errorf("operation = select for %q, which also drops a table — "+
					"the parser described only the first statement", tt.sql)
			}
		})
	}
}

// TestParseKeepsLiteralsIntact covers the valid input truncation rejected.
func TestParseKeepsLiteralsIntact(t *testing.T) {
	tests := []struct {
		name  string
		parse func(string) (*SQLParseResult, error)
		sql   string
	}{
		{"mysql", ParseMySQLDialect, `SELECT * FROM users WHERE name = 'a;b'`},
		{"postgres", ParsePostgreSQLDialect, `SELECT * FROM users WHERE name = 'a;b'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.parse(tt.sql)
			if err != nil {
				t.Fatalf("a valid single statement was refused: %v", err)
			}
			// Only a degradation warning matters here; "no LIMIT" is an
			// unrelated observation about a well-formed statement.
			for _, w := range result.Warnings {
				if strings.Contains(w, "解析失败") {
					t.Errorf("parsed by degrading: %q — the statement is well formed", w)
				}
			}
			if len(result.Tables) != 1 || result.Tables[0] != "users" {
				t.Errorf("tables = %v, want [users]", result.Tables)
			}
		})
	}
}
