package ticket

import (
	"testing"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/testutil"
)

// TestUnreadableConditionDoesNotMatch pins the failure direction.
func TestUnreadableConditionDoesNotMatch(t *testing.T) {
	e := NewApprovalEngine(testutil.NewDB(t), nil)
	policy := &model.ApprovalPolicy{
		ID:         1,
		Conditions: `{"conditions":[{"field":"environment","operator":"in","values":["prod"]}]}`,
	}
	if e.policyMatches(policy, &model.Ticket{RiskLevel: "low"}) {
		t.Error("a condition the platform cannot read selected the ticket")
	}
}
