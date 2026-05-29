package service

import (
	"encoding/json"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

func TestContainsAny(t *testing.T) {
	if !containsAny([]string{"HIGH", "CRITICAL"}, "high") {
		t.Error("case-insensitive match failed")
	}
	if containsAny([]string{"HIGH"}, "low") {
		t.Error("should not match")
	}
	if !containsAny([]string{"drop", "alter"}, "DROP") {
		t.Error("case-insensitive match failed (reverse)")
	}
}

func TestApprovalPolicyConditions(t *testing.T) {
	ae := &ApprovalEngine{}

	// Empty conditions = match all
	policy := &model.ApprovalPolicy{
		Conditions: `{}`,
	}
	ticket := &model.Ticket{RiskLevel: "low", Database: "testdb"}
	if !ae.policyMatches(policy, ticket) {
		t.Error("empty conditions should match all")
	}

	// Risk level match
	policy.Conditions = `{"risk_levels":["high","critical"]}`
	ticket.RiskLevel = "high"
	if !ae.policyMatches(policy, ticket) {
		t.Error("risk level high should match")
	}
	ticket.RiskLevel = "low"
	if ae.policyMatches(policy, ticket) {
		t.Error("risk level low should not match")
	}

	// SQL type match — uses ticket.SQLType (populated by sql_analyzer)
	policy.Conditions = `{"sql_types":["DROP","ALTER"]}`
	ticket.SQLContent = "DROP TABLE users"
	ticket.SQLType = "DROP"
	if !ae.policyMatches(policy, ticket) {
		t.Error("DROP should match sql_types [DROP,ALTER]")
	}
	ticket.SQLContent = "SELECT * FROM users"
	ticket.SQLType = "SELECT"
	if ae.policyMatches(policy, ticket) {
		t.Error("SELECT should not match sql_types [DROP,ALTER]")
	}

	// Database match
	policy.Conditions = `{"databases":["production"]}`
	ticket.Database = "production"
	if !ae.policyMatches(policy, ticket) {
		t.Error("production database should match")
	}
	ticket.Database = "staging"
	if ae.policyMatches(policy, ticket) {
		t.Error("staging database should not match")
	}

	// Invalid JSON
	policy.Conditions = `invalid`
	if ae.policyMatches(policy, ticket) {
		t.Error("invalid JSON should not match")
	}
}

func TestApprovalChainParsing(t *testing.T) {
	chain := `[{"role":"team_lead","auto_skip_same_submitter":false},{"role":"dba","auto_skip_same_submitter":true}]`

	var stages []ApprovalChainStage
	err := json.Unmarshal([]byte(chain), &stages)
	if err != nil {
		t.Fatalf("failed to parse chain: %v", err)
	}
	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}
	if stages[0].Role != "team_lead" {
		t.Errorf("stage 0 role = %q, want team_lead", stages[0].Role)
	}
	if stages[1].Role != "dba" {
		t.Errorf("stage 1 role = %q, want dba", stages[1].Role)
	}
	if !stages[1].AutoSkipSameSubmitter {
		t.Error("stage 1 auto_skip should be true")
	}
}

func TestPolicyConditionParsing(t *testing.T) {
	cond := `{"risk_levels":["high","critical"],"sql_types":["DROP","ALTER"],"environments":["production"]}`

	var pc PolicyCondition
	err := json.Unmarshal([]byte(cond), &pc)
	if err != nil {
		t.Fatalf("failed to parse conditions: %v", err)
	}
	if len(pc.RiskLevels) != 2 {
		t.Errorf("risk_levels = %v, want 2 items", pc.RiskLevels)
	}
	if len(pc.SQLTypes) != 2 {
		t.Errorf("sql_types = %v, want 2 items", pc.SQLTypes)
	}
	if len(pc.Environments) != 1 {
		t.Errorf("environments = %v, want 1 item", pc.Environments)
	}
}
