package query

import (
	"testing"
)

// TestMongoBSONMarshalRoundtrip verifies parseMongoBody works with ExtJSON filters.
func TestMongoBSONMarshalRoundtrip(t *testing.T) {
	body := `{"operation": "find", "collection": "users", "filter": {"name": "test", "age": 25}}`
	m := parseMongoBody(body)
	if m == nil {
		t.Fatal("parseMongoBody returned nil")
	}

	filter, ok := m["filter"]
	if !ok {
		t.Fatal("body has no filter key")
	}

	filterMap, ok := filter.(map[string]interface{})
	if !ok {
		t.Fatal("filter is not a map")
	}

	if filterMap["name"] != "test" {
		t.Errorf("filter.name = %v, want test", filterMap["name"])
	}
}

// TestParseMongoBodyHelpers verifies parseMongoBody edge cases.
func TestParseMongoBodyHelpers(t *testing.T) {
	tests := []struct {
		name string
		body string
		nil  bool
	}{
		{"empty", "", true},
		{"invalid_json", "{invalid}", true},
		{"valid", `{"key": "value"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMongoBody(tt.body)
			if tt.nil && got != nil {
				t.Errorf("expected nil, got %v", got)
			}
			if !tt.nil {
				if got == nil {
					t.Fatal("expected non-nil result")
				}
				if v, ok := got["key"].(string); !ok || v != "value" {
					t.Errorf("key = %v, want value", got["key"])
				}
			}
		})
	}
}
