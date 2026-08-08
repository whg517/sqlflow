package ticket

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

	// Empty conditions = match all. This is what EnsureDefaultPolicy stores, and
	// it is the one form that means "everything" — a half-built condition set no
	// longer collapses to the same answer.
	policy := &model.ApprovalPolicy{Conditions: `{}`}
	ticket := &model.Ticket{RiskLevel: "low", Database: "testdb"}
	if !ae.policyMatches(policy, ticket) {
		t.Error("empty conditions should match all")
	}

	policy.Conditions = `{"conditions":[{"field":"risk_level","operator":"in","values":["high","critical"]}]}`
	ticket.RiskLevel = "high"
	if !ae.policyMatches(policy, ticket) {
		t.Error("risk level high should match")
	}
	ticket.RiskLevel = "low"
	if ae.policyMatches(policy, ticket) {
		t.Error("risk level low should not match")
	}

	// SQL type comes from ticket.SQLType, which sql_analyzer populates.
	policy.Conditions = `{"conditions":[{"field":"sql_type","operator":"in","values":["DROP","ALTER"]}]}`
	ticket.SQLType = "DROP"
	if !ae.policyMatches(policy, ticket) {
		t.Error("DROP should match sql_type in [DROP,ALTER]")
	}
	ticket.SQLType = "SELECT"
	if ae.policyMatches(policy, ticket) {
		t.Error("SELECT should not match sql_type in [DROP,ALTER]")
	}

	// Database scoping.
	policy.Conditions = `{"conditions":[{"field":"database","operator":"in","values":["prod"]}]}`
	ticket.Database = "prod"
	if !ae.policyMatches(policy, ticket) {
		t.Error("database prod should match")
	}
	ticket.Database = "testdb"
	if ae.policyMatches(policy, ticket) {
		t.Error("database testdb should not match a policy scoped to prod")
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
