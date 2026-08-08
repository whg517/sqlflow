package ticket

import (
	"os"
	"regexp"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

// TestParseCondition_AcceptsWhatTheFormBuilds is the round trip that was broken
// in both directions.
//
// The form emitted {"logic","conditions":[{"field","operator","value"}]}; the
// validator expected {"logic","children":[{"field","op","value"}]} and rejected
// every non-empty set the form could produce, so the only saveable condition was
// "{}". I proved that before changing anything by feeding the validator exactly
// what the form emits.
func TestParseCondition_AcceptsWhatTheFormBuilds(t *testing.T) {
	raw := `{"logic":"AND","conditions":[
		{"field":"risk_level","operator":"in","values":["high","critical"]},
		{"field":"database","operator":"not_in","values":["scratch"]}
	]}`

	cond, err := ParseCondition(raw)
	if err != nil {
		t.Fatalf("ParseCondition: %v", err)
	}
	if len(cond.Rules) != 2 {
		t.Fatalf("parsed %d rules, want 2", len(cond.Rules))
	}
	if cond.Logic != LogicAnd {
		t.Errorf("logic = %q, want AND", cond.Logic)
	}
}

// TestParseCondition_RefusesWhatTheMatcherCannotHonor is the rule that inverts
// the old failure direction.
//
// An unrecognized condition used to widen the match: it unmarshaled into an
// all-empty value, and all-empty meant match-all. A policy carrying one and
// auto_approve_enabled therefore auto-approved every ticket, which is a client
// choosing its own approval path.
func TestParseCondition_RefusesWhatTheMatcherCannotHonor(t *testing.T) {
	refused := []struct{ name, raw string }{
		{"unknown field", `{"conditions":[{"field":"affected_rows","operator":"in","values":["10"]}]}`},
		{"field nothing can observe", `{"conditions":[{"field":"environment","operator":"in","values":["prod"]}]}`},
		{"unknown operator", `{"conditions":[{"field":"risk_level","operator":"regex","values":["hi.*"]}]}`},
		{"unknown logic", `{"logic":"XOR","conditions":[{"field":"risk_level","values":["high"]}]}`},
		{"value outside the closed set", `{"conditions":[{"field":"risk_level","operator":"in","values":["hgih"]}]}`},
		{"no values", `{"conditions":[{"field":"risk_level","operator":"in","values":[]}]}`},
		{"logic with no rules", `{"logic":"AND","conditions":[]}`},
		{"not json", `{oops`},
	}

	for _, tt := range refused {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseCondition(tt.raw); err == nil {
				t.Error("accepted a condition the matcher cannot honor")
			}
		})
	}
}

// TestParseCondition_EmptyMeansMatchAllOnlyWhenSaidSo separates the two things
// that used to be indistinguishable.
func TestParseCondition_EmptyMeansMatchAllOnlyWhenSaidSo(t *testing.T) {
	ticket := &model.Ticket{RiskLevel: "low", SQLType: "SELECT", Database: "appdb"}

	for _, raw := range []string{"", "{}", "[]", "null"} {
		cond, err := ParseCondition(raw)
		if err != nil {
			t.Fatalf("ParseCondition(%q): %v", raw, err)
		}
		if !cond.Matches(ticket) {
			t.Errorf("ParseCondition(%q) does not match everything", raw)
		}
	}

	// And a zero Condition — one nobody parsed — must not be usable as match-all
	// by accident. It is only reachable through a bug, so the safe answer is the
	// one that does not widen a policy.
	var never Condition
	if len(never.Rules) != 0 || never.matchAll {
		t.Error("the zero Condition should carry no rules and no explicit match-all")
	}
}

// TestConditionMatches covers the operators against real ticket values.
func TestConditionMatches(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		ticket *model.Ticket
		want   bool
	}{
		{
			"risk in set",
			`{"conditions":[{"field":"risk_level","operator":"in","values":["high","critical"]}]}`,
			&model.Ticket{RiskLevel: "high"}, true,
		},
		{
			"risk outside set",
			`{"conditions":[{"field":"risk_level","operator":"in","values":["high"]}]}`,
			&model.Ticket{RiskLevel: "low"}, false,
		},
		{
			"not_in excludes",
			`{"conditions":[{"field":"database","operator":"not_in","values":["scratch"]}]}`,
			&model.Ticket{Database: "scratch"}, false,
		},
		{
			"AND needs every rule",
			`{"logic":"AND","conditions":[
				{"field":"risk_level","operator":"in","values":["high"]},
				{"field":"database","operator":"in","values":["appdb"]}]}`,
			&model.Ticket{RiskLevel: "high", Database: "other"}, false,
		},
		{
			"OR needs one rule",
			`{"logic":"OR","conditions":[
				{"field":"risk_level","operator":"in","values":["high"]},
				{"field":"database","operator":"in","values":["appdb"]}]}`,
			&model.Ticket{RiskLevel: "low", Database: "appdb"}, true,
		},
		{
			// sql_analyzer leaves SQLType empty for statements it cannot
			// classify, and a policy scoped to OTHER has to catch those — an
			// unclassifiable statement is precisely the one worth routing.
			"unclassified sql counts as OTHER",
			`{"conditions":[{"field":"sql_type","operator":"in","values":["OTHER"]}]}`,
			&model.Ticket{SQLType: ""}, true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := ParseCondition(tt.raw)
			if err != nil {
				t.Fatalf("ParseCondition: %v", err)
			}
			if got := cond.Matches(tt.ticket); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConditionSchemaCoversEveryField keeps the declaration and the parser from
// drifting — the drift that started all of this.
func TestConditionSchemaCoversEveryField(t *testing.T) {
	for _, d := range ConditionSchema() {
		raw := `{"conditions":[{"field":"` + string(d.Field) + `","operator":"in","values":["x"]}]}`
		if len(d.Values) > 0 {
			raw = `{"conditions":[{"field":"` + string(d.Field) + `","operator":"in","values":["` + d.Values[0] + `"]}]}`
		}
		if _, err := ParseCondition(raw); err != nil {
			t.Errorf("schema advertises %q but the parser refuses it: %v", d.Field, err)
		}
		if len(d.Operators) == 0 {
			t.Errorf("schema advertises %q with no operators", d.Field)
		}
	}
}

// TestConditionSQLTypesCoverWhatTheAnalyzerEmits keeps the closed value set from
// drifting from the code that produces the values.
//
// The set is what a policy author may choose from. If the analyzer emits a type
// the set omits, that type is unroutable — no policy can name it — and the
// omission is invisible until someone needs it. Reading the producer is the
// only way to know; my first draft guessed "DDL" and the analyzer emits CREATE,
// ALTER, DROP and TRUNCATE separately.
func TestConditionSQLTypesCoverWhatTheAnalyzerEmits(t *testing.T) {
	source, err := os.ReadFile("sql_analyzer.go")
	if err != nil {
		t.Fatalf("read sql_analyzer.go: %v", err)
	}

	emitted := map[string]bool{}
	for _, m := range regexp.MustCompile(`SQLType\s*[:=]+\s*"([A-Z]+)"`).FindAllSubmatch(source, -1) {
		emitted[string(m[1])] = true
	}
	if len(emitted) == 0 {
		t.Fatal("found no SQLType assignments; this test can no longer see its subject")
	}

	declared := map[string]bool{}
	for _, v := range conditionFieldValues[FieldSQLType] {
		declared[v] = true
	}
	for v := range emitted {
		if !declared[v] {
			t.Errorf("sql_analyzer emits %q but no policy can name it", v)
		}
	}
}
