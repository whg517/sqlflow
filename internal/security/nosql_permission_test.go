package security

import (
	"io/fs"
	"testing"

	"github.com/whg517/sqlflow/internal/platform/sqlparser"
)

// TestMongoOpToCasbinAct verifies the MongoDB operation to Casbin action mapping.
func TestMongoOpToCasbinAct(t *testing.T) {
	tests := []struct {
		op   sqlparser.MongoOperation
		want string
	}{
		{sqlparser.MongoOpFind, "select"},
		{sqlparser.MongoOpAggregate, "select"},
		{sqlparser.MongoOpInsert, "insert"},
		{sqlparser.MongoOpUpdate, "update"},
		{sqlparser.MongoOpDelete, "delete"},
		{sqlparser.MongoOpUnknown, "select"}, // default fallback
	}

	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			got := MongoOpToCasbinAct(tt.op)
			if got != tt.want {
				t.Errorf("MongoOpToCasbinAct(%q) = %q, want %q", tt.op, got, tt.want)
			}
		})
	}
}

// TestPolicySeedContainsInsert verifies the policy seed includes insert action.
func TestPolicySeedContainsInsert(t *testing.T) {
	data, err := fs.ReadFile(casbinModelFS, "policy_seed.csv")
	if err != nil {
		t.Fatalf("read policy_seed.csv: %v", err)
	}
	if !containsSubstring(string(data), "insert") {
		t.Error("policy_seed.csv should contain 'insert' action for MongoDB support")
	}
}

// TestMongoPermissionMapping_Integration verifies the full mapping from MongoDB JSON to Casbin act.
func TestMongoPermissionMapping_Integration(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantAct  string
		wantColl string
	}{
		{
			name:     "find_maps_to_select",
			body:     `{"operation": "find", "collection": "users", "filter": {"active": true}}`,
			wantAct:  "select",
			wantColl: "users",
		},
		{
			name:     "aggregate_maps_to_select",
			body:     `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$match": {}}]}`,
			wantAct:  "select",
			wantColl: "orders",
		},
		{
			name:     "insert_maps_to_insert",
			body:     `{"operation": "insert", "collection": "logs", "document": {"level": "info"}}`,
			wantAct:  "insert",
			wantColl: "logs",
		},
		{
			name:     "update_maps_to_update",
			body:     `{"operation": "update", "collection": "users", "filter": {"_id": 1}, "update": {"$set": {"name": "test"}}}`,
			wantAct:  "update",
			wantColl: "users",
		},
		{
			name:     "delete_maps_to_delete",
			body:     `{"operation": "delete", "collection": "sessions", "filter": {"expired": true}}`,
			wantAct:  "delete",
			wantColl: "sessions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sqlparser.ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Collection != tt.wantColl {
				t.Errorf("Collection = %q, want %q", result.Collection, tt.wantColl)
			}
			act := MongoOpToCasbinAct(result.Operation)
			if act != tt.wantAct {
				t.Errorf("act = %q, want %q", act, tt.wantAct)
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
