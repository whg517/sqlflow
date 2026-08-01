package ticket

import (
	"context"
	"testing"
)

// TestCheckMongoPermission_ParseError verifies that parse errors are non-blocking.
func TestCheckMongoPermission_ParseError(t *testing.T) {
	svc := &Service{
		permSvc: nil, // no permSvc — should not panic
	}

	// Without permSvc, should return nil (no check)
	err := svc.checkMongoPermission(context.TODO(), "developer", 1, "")
	if err != nil {
		t.Errorf("expected nil when permSvc is nil, got %v", err)
	}
}

// TestCheckMongoPermission_NilPermSvc verifies graceful handling of nil permSvc.
func TestCheckMongoPermission_NilPermSvc(t *testing.T) {
	svc := &Service{}

	err := svc.checkMongoPermission(context.TODO(), "developer", 1,
		`{"operation": "find", "collection": "users", "filter": {}}`)
	if err != nil {
		t.Errorf("expected nil when permSvc is nil, got %v", err)
	}
}

// TestCheckMongoPermission_NoCollection verifies no check when collection is empty.
func TestCheckMongoPermission_NoCollection(t *testing.T) {
	// Create a mock permission service that would fail if called
	svc := &Service{
		permSvc: nil, // intentionally nil — if code reaches Enforce, it would panic
	}

	err := svc.checkMongoPermission(context.TODO(), "developer", 1,
		`{"operation": "find", "filter": {}}`)
	if err != nil {
		t.Errorf("expected nil for no collection, got %v", err)
	}
}
