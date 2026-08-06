package ticket

import "testing"

// TestTicketRiskForDocumentDatasources is the case the keyword analyzer could
// never answer.
//
// SQLAnalyzer matches on a leading SQL keyword. A MongoDB command body starts
// with "{", so it matched nothing, fell to SQLType "OTHER" and scored 15 —
// low — whatever it actually did. Risk selects the approval policy, including
// auto-approval, so a ticket that emptied a collection was filed as the least
// dangerous kind there is.
func TestTicketRiskForDocumentDatasources(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "delete with an empty filter removes every document",
			body: `{"operation":"delete","collection":"users","filter":{}}`,
			want: RiskLevelHigh,
		},
		{
			name: "update touches stored data",
			body: `{"operation":"update","collection":"users","filter":{"id":1},"update":{"$set":{"vip":true}}}`,
			want: RiskLevelMedium,
		},
		{
			name: "insert adds documents",
			body: `{"operation":"insert","collection":"users","documents":[{"a":1}]}`,
			want: RiskLevelMedium,
		},
		{
			name: "find only reads",
			body: `{"operation":"find","collection":"users","filter":{}}`,
			want: RiskLevelLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewRiskEvaluator().Evaluate(analyzeTicketSQL("mongodb", tt.body)).Level
			if got != tt.want {
				t.Errorf("risk = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestTicketRiskKeepsSQLGranularity guards the reason the keyword analyzer is
// still used for SQL datasources: the driver's OperationType collapses every
// DDL into one value, while the score separates CREATE from DROP.
func TestTicketRiskKeepsSQLGranularity(t *testing.T) {
	tests := []struct {
		sql  string
		want string
	}{
		{"DROP TABLE users", RiskLevelCritical},
		{"TRUNCATE TABLE logs", RiskLevelCritical},
		{"ALTER TABLE users ADD COLUMN phone VARCHAR(20)", RiskLevelCritical},
		// CREATE is the DDL that lands in a different band, which is the
		// distinction a single "ddl" operation type would erase.
		{"CREATE TABLE t (id INT)", RiskLevelHigh},
		{"DELETE FROM logs WHERE id = 1", RiskLevelHigh},
		{"UPDATE users SET vip = true WHERE id = 1", RiskLevelMedium},
		{"SELECT * FROM users", RiskLevelLow},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			got := NewRiskEvaluator().Evaluate(analyzeTicketSQL("mysql", tt.sql)).Level
			if got != tt.want {
				t.Errorf("risk for %q = %s, want %s", tt.sql, got, tt.want)
			}
		})
	}
}

// TestTicketRiskForUnrecognizedStatement pins the fallback.
//
// "We could not work out what this does" is not the same as "this is safe", but
// the unknown branch scored 15 and landed on low — the band that a policy may
// auto-approve. Anything the server cannot classify has to require a human.
func TestTicketRiskForUnrecognizedStatement(t *testing.T) {
	for _, sql := range []string{
		"CALL sp_rebuild_everything()",
		"LOAD DATA INFILE '/tmp/x' INTO TABLE users",
		"{}",
	} {
		t.Run(sql, func(t *testing.T) {
			if got := NewRiskEvaluator().Evaluate(analyzeTicketSQL("mysql", sql)).Level; got == RiskLevelLow {
				t.Errorf("risk for the unrecognized %q = low — an unclassifiable statement may not be auto-approvable", sql)
			}
		})
	}
}

// TestTicketRiskForUnparseableDocumentBody covers the driver refusing the body.
//
// A malformed MongoDB command cannot be shown to be safe either, so it must not
// come out as low.
func TestTicketRiskForUnparseableDocumentBody(t *testing.T) {
	if got := NewRiskEvaluator().Evaluate(analyzeTicketSQL("mongodb", `{"operation":`)).Level; got == RiskLevelLow {
		t.Error("an unparseable MongoDB body scored low")
	}
}
