package casbin

import "testing"

func TestNewEnforcer(t *testing.T) {
	// NewEnforcer is a placeholder — should return nil error
	err := NewEnforcer("model.conf", "policy.csv")
	if err != nil {
		t.Errorf("NewEnforcer: %v", err)
	}
}
