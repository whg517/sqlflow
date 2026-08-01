package sqlparser

import (
	"fmt"
	"strings"
	"testing"
)

// TestParseElasticsearch_BodySchemaWhitelist verifies DSL body field whitelist.
func TestParseElasticsearch_BodySchemaWhitelist(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantBlock  bool
		blockField string
	}{
		{
			name:      "allowed query field",
			body:      `{"query":{"match_all":{}}}`,
			wantBlock: false,
		},
		{
			name:      "allowed sort field",
			body:      `{"query":{"match_all":{}},"sort":[{"_score":"desc"}]}`,
			wantBlock: false,
		},
		{
			name:      "allowed aggs field",
			body:      `{"query":{"match_all":{}},"aggs":{"group_by_status":{"terms":{"field":"status"}}}}`,
			wantBlock: false,
		},
		{
			name:      "allowed size and from",
			body:      `{"query":{"match_all":{}},"size":10,"from":0}`,
			wantBlock: false,
		},
		{
			name:      "allowed _source filter",
			body:      `{"query":{"match_all":{}},"_source":["name","email"]}`,
			wantBlock: false,
		},
		{
			name:       "blocked script field",
			body:       `{"script":{"source":"doc['price'].value * 2"}}`,
			wantBlock:  true,
			blockField: "禁止使用 script 字段",
		},
		{
			name:       "blocked script_fields",
			body:       `{"query":{"match_all":{}},"script_fields":{"computed":{"script":{"source":"1"}}}}`,
			wantBlock:  true,
			blockField: "禁止使用 script 字段",
		},
		{
			name:       "blocked arbitrary field",
			body:       `{"query":{"match_all":{}},"custom_field":123}`,
			wantBlock:  true,
			blockField: "custom_field",
		},
		{
			name:       "blocked suggest field",
			body:       `{"query":{"match_all":{}},"suggest":{"my-suggestion":{"text":"test"}}}`,
			wantBlock:  true,
			blockField: "suggest",
		},
		{
			name:       "blocked collapse field",
			body:       `{"query":{"match_all":{}},"collapse":{"field":"user_id"}}`,
			wantBlock:  true,
			blockField: "collapse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := fmt.Sprintf(`{"index":"test-*","body":%s}`, tt.body)
			result, err := ParseSQL(input, "elasticsearch")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsBlocked != tt.wantBlock {
				t.Errorf("IsBlocked = %v, want %v (reason: %s)", result.IsBlocked, tt.wantBlock, result.BlockReason)
			}
			if tt.wantBlock && tt.blockField != "" {
				if !strings.Contains(result.BlockReason, tt.blockField) {
					t.Errorf("BlockReason = %q, want to contain %q", result.BlockReason, tt.blockField)
				}
			}
		})
	}
}

// TestParseElasticsearch_OperationWhitelist verifies only search/count are allowed.
func TestParseElasticsearch_OperationWhitelist(t *testing.T) {
	tests := []struct {
		name     string
		op       string
		wantPass bool
	}{
		{name: "default search", op: "", wantPass: true},
		{name: "explicit search", op: "search", wantPass: true},
		{name: "count operation", op: "count", wantPass: true},
		{name: "blocked update", op: "update", wantPass: false},
		{name: "blocked delete", op: "delete", wantPass: false},
		{name: "blocked bulk", op: "bulk", wantPass: false},
		{name: "blocked reindex", op: "reindex", wantPass: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input string
			if tt.op != "" {
				input = fmt.Sprintf(`{"index":"test-*","operation":"%s","body":{"query":{"match_all":{}}}}`, tt.op)
			} else {
				input = `{"index":"test-*","body":{"query":{"match_all":{}}}}`
			}
			result, err := ParseSQL(input, "elasticsearch")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantPass {
				if result.IsBlocked {
					t.Errorf("expected pass but got blocked: %s", result.BlockReason)
				}
				if result.Operation != OpSelect {
					t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
				}
			} else {
				if !result.IsBlocked {
					t.Error("expected blocked but got pass")
				}
			}
		})
	}
}

// TestParseElasticsearch_DangerousEndpoints verifies dangerous endpoint blocking.
func TestParseElasticsearch_DangerousEndpoints(t *testing.T) {
	tests := []struct {
		name    string
		index   string
		blocked bool
	}{
		{name: "safe index", index: "logs-*", blocked: false},
		{name: "safe specific index", index: "app-logs-2024", blocked: false},
		{name: "blocked _bulk", index: "_bulk", blocked: true},
		{name: "blocked _delete_by_query", index: "_delete_by_query", blocked: true},
		{name: "blocked _update_by_query", index: "_update_by_query", blocked: true},
		{name: "blocked _reindex", index: "_reindex", blocked: true},
		{name: "blocked _msearch", index: "_msearch", blocked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := fmt.Sprintf(`{"index":"%s","body":{"query":{"match_all":{}}}}`, tt.index)
			result, err := ParseSQL(input, "elasticsearch")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsBlocked != tt.blocked {
				t.Errorf("IsBlocked = %v, want %v (reason: %s)", result.IsBlocked, tt.blocked, result.BlockReason)
			}
		})
	}
}
