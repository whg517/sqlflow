package service

import (
	"encoding/json"
	"testing"
)

func TestValidateConditions_Empty(t *testing.T) {
	// Empty or blank conditions should be valid (match-all)
	cases := []string{"", "  ", "{}", "[]"}
	for _, c := range cases {
		if err := ValidateConditions(c); err != nil {
			t.Errorf("ValidateConditions(%q) = %v, want nil", c, err)
		}
	}
}

func TestValidateConditions_LegacyFormat(t *testing.T) {
	// Legacy format: {"risk_levels":["high","critical"]}
	cond := `{"risk_levels":["high","critical"],"sql_types":["DROP","ALTER"],"databases":["production"]}`
	if err := ValidateConditions(cond); err != nil {
		t.Errorf("legacy format failed: %v", err)
	}
}

func TestValidateConditions_LegacyUnknownKey(t *testing.T) {
	cond := `{"unknown_field":["value"]}`
	if err := ValidateConditions(cond); err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

func TestValidateConditions_WhitelistFields(t *testing.T) {
	// Whitelisted fields
	cases := []string{
		`{"risk_level":"high"}`,
		`{"sql_type":"DROP"}`,
		`{"environment":"production"}`,
		`{"database":"mydb"}`,
		`{"affected_tables":["users","orders"]}`,
		`{"affected_rows":">100"}`,
		`{"submitter":"john"}`,
	}
	for _, c := range cases {
		if err := ValidateConditions(c); err != nil {
			t.Errorf("ValidateConditions(%q) = %v, want nil", c, err)
		}
	}
}

func TestValidateConditions_DisallowedField(t *testing.T) {
	cond := `{"exec_command":"rm -rf /"}`
	if err := ValidateConditions(cond); err == nil {
		t.Error("expected error for disallowed field, got nil")
	}
}

func TestValidateConditions_LeafNode(t *testing.T) {
	// Structured leaf node
	cond := `{"field":"risk_level","op":"=","value":"high"}`
	if err := ValidateConditions(cond); err != nil {
		t.Errorf("leaf node failed: %v", err)
	}

	// Invalid operator
	cond = `{"field":"risk_level","op":"EXEC","value":"high"}`
	if err := ValidateConditions(cond); err == nil {
		t.Error("expected error for invalid operator")
	}

	// Invalid field
	cond = `{"field":"password","op":"=","value":"x"}`
	if err := ValidateConditions(cond); err == nil {
		t.Error("expected error for invalid field")
	}
}

func TestValidateConditions_GroupNode(t *testing.T) {
	// AND group with 2 leaf children
	cond := `{
		"logic": "AND",
		"children": [
			{"field":"risk_level","op":"=","value":"high"},
			{"field":"database","op":"IN","value":["prod","staging"]}
		]
	}`
	if err := ValidateConditions(cond); err != nil {
		t.Errorf("group node failed: %v", err)
	}

	// OR group
	cond = `{
		"logic": "OR",
		"children": [
			{"field":"risk_level","op":"=","value":"high"},
			{"field":"risk_level","op":"=","value":"critical"}
		]
	}`
	if err := ValidateConditions(cond); err != nil {
		t.Errorf("OR group failed: %v", err)
	}
}

func TestValidateConditions_NestedGroupDepth(t *testing.T) {
	// 3 levels deep — should exceed max depth of 2
	cond := `{
		"logic": "AND",
		"children": [
			{
				"logic": "AND",
				"children": [
					{
						"logic": "AND",
						"children": [
							{"field":"risk_level","op":"=","value":"high"}
						]
					}
				]
			}
		]
	}`
	if err := ValidateConditions(cond); err == nil {
		t.Error("expected depth limit error for 3-level nesting")
	}
}

func TestValidateConditions_InvalidLogic(t *testing.T) {
	cond := `{"logic":"XOR","children":[{"field":"risk_level","op":"=","value":"high"}]}`
	if err := ValidateConditions(cond); err == nil {
		t.Error("expected error for invalid logic operator")
	}
}

func TestValidateConditions_EmptyChildren(t *testing.T) {
	cond := `{"logic":"AND","children":[]}`
	if err := ValidateConditions(cond); err == nil {
		t.Error("expected error for empty children in group")
	}
}

func TestValidateConditions_ValueTooLong(t *testing.T) {
	longVal := make([]byte, 501)
	for i := range longVal {
		longVal[i] = 'a'
	}
	cond := `{"field":"database","op":"=","value":"` + string(longVal) + `"}`
	if err := ValidateConditions(cond); err == nil {
		t.Error("expected error for value exceeding length limit")
	}
}

func TestValidateConditions_ArrayValue(t *testing.T) {
	// Array values should be valid
	cond := `{"field":"database","op":"IN","value":["prod","staging","test"]}`
	if err := ValidateConditions(cond); err != nil {
		t.Errorf("array value failed: %v", err)
	}

	// Empty array
	cond = `{"field":"database","op":"IN","value":[]}`
	if err := ValidateConditions(cond); err == nil {
		t.Error("expected error for empty array value")
	}
}

func TestValidateConditions_NumberValue(t *testing.T) {
	cond := `{"field":"affected_rows","op":"=","value":100}`
	if err := ValidateConditions(cond); err != nil {
		t.Errorf("number value failed: %v", err)
	}
}

func TestValidateConditions_InvalidJSON(t *testing.T) {
	cases := []string{
		"not json at all",
		`{invalid`,
		`[}`,
	}
	for _, c := range cases {
		if err := ValidateConditions(c); err == nil {
			t.Errorf("expected error for invalid JSON: %q", c)
		}
	}
}

func TestValidateConditions_OperatorWhitelist(t *testing.T) {
	validOps := []string{"=", "!=", "IN", "NOT IN", "LIKE", "CONTAINS"}
	for _, op := range validOps {
		cond := `{"field":"database","op":"` + op + `","value":"test"}`
		if err := ValidateConditions(cond); err != nil {
			t.Errorf("operator %s should be valid: %v", op, err)
		}
	}

	invalidOps := []string{"EXEC", "DROP", ">", "<", ">=", "<="}
	for _, op := range invalidOps {
		cond := `{"field":"database","op":"` + op + `","value":"test"}`
		if err := ValidateConditions(cond); err == nil {
			t.Errorf("operator %s should be invalid", op)
		}
	}
}

// Ensure conditionNode parses correctly
func TestConditionNodeParsing(t *testing.T) {
	raw := `{"logic":"AND","children":[{"field":"risk_level","op":"=","value":"high"}]}`
	var node conditionNode
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if node.Logic != "AND" {
		t.Errorf("logic = %q, want AND", node.Logic)
	}
	if len(node.Children) != 1 {
		t.Fatalf("children = %d, want 1", len(node.Children))
	}
	if node.Children[0].Field != "risk_level" {
		t.Errorf("child field = %q, want risk_level", node.Children[0].Field)
	}
}
