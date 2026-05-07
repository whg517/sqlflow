package sqlparser

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Find Operations
// ---------------------------------------------------------------------------

func TestParseMongo_Find(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantCollection  string
		wantHasFilter   bool
		wantEmptyFilter bool
	}{
		{
			name:            "find_with_filter",
			body:            `{"operation": "find", "collection": "users", "filter": {"active": true}}`,
			wantCollection:  "users",
			wantHasFilter:   true,
			wantEmptyFilter: false,
		},
		{
			name:            "find_with_empty_filter",
			body:            `{"operation": "find", "collection": "users", "filter": {}}`,
			wantCollection:  "users",
			wantHasFilter:   true,
			wantEmptyFilter: true,
		},
		{
			name:            "find_without_filter",
			body:            `{"operation": "find", "collection": "users"}`,
			wantCollection:  "users",
			wantHasFilter:   false,
			wantEmptyFilter: true,
		},
		{
			name:            "find_with_complex_filter",
			body:            `{"operation": "find", "collection": "products", "filter": {"price": {"$gte": 100}, "category": "electronics"}}`,
			wantCollection:  "products",
			wantHasFilter:   true,
			wantEmptyFilter: false,
		},
		{
			name:           "find_no_collection",
			body:           `{"operation": "find", "filter": {"x": 1}}`,
			wantCollection: "",
			wantHasFilter: true,
			wantEmptyFilter: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != MongoOpFind {
				t.Errorf("Operation = %q, want %q", result.Operation, MongoOpFind)
			}
			if result.Collection != tt.wantCollection {
				t.Errorf("Collection = %q, want %q", result.Collection, tt.wantCollection)
			}
			if result.HasFilter != tt.wantHasFilter {
				t.Errorf("HasFilter = %v, want %v", result.HasFilter, tt.wantHasFilter)
			}
			if result.HasEmptyFilter != tt.wantEmptyFilter {
				t.Errorf("HasEmptyFilter = %v, want %v", result.HasEmptyFilter, tt.wantEmptyFilter)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Update Operations
// ---------------------------------------------------------------------------

func TestParseMongo_Update(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantIsMulti    bool
		wantEmpty      bool
		wantCollection string
	}{
		{
			name:           "update_one_with_filter",
			body:           `{"operation": "update", "collection": "users", "filter": {"id": 1}, "update": {"$set": {"name": "new"}}}`,
			wantIsMulti:    false,
			wantEmpty:      false,
			wantCollection: "users",
		},
		{
			name:           "update_many_with_empty_filter",
			body:           `{"operation": "update", "collection": "users", "multi": true, "filter": {}, "update": {"$set": {"active": true}}}`,
			wantIsMulti:    true,
			wantEmpty:      true,
			wantCollection: "users",
		},
		{
			name:           "update_many_without_filter",
			body:           `{"operation": "update", "collection": "users", "multi": true, "update": {"$set": {"active": true}}}`,
			wantIsMulti:    true,
			wantEmpty:      true,
			wantCollection: "users",
		},
		{
			name:           "update_one_with_nonempty_filter",
			body:           `{"operation": "updateOne", "collection": "users", "filter": {"_id": "abc"}, "update": {"$set": {"x": 1}}}`,
			wantIsMulti:    false,
			wantEmpty:      false,
			wantCollection: "users",
		},
		{
			name:           "update_many_with_nonempty_filter",
			body:           `{"operation": "updateMany", "collection": "users", "filter": {"status": "inactive"}, "multi": true, "update": {"$set": {"status": "active"}}}`,
			wantIsMulti:    true,
			wantEmpty:      false,
			wantCollection: "users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != MongoOpUpdate {
				t.Errorf("Operation = %q, want %q", result.Operation, MongoOpUpdate)
			}
			if result.IsMulti != tt.wantIsMulti {
				t.Errorf("IsMulti = %v, want %v", result.IsMulti, tt.wantIsMulti)
			}
			if result.HasEmptyFilter != tt.wantEmpty {
				t.Errorf("HasEmptyFilter = %v, want %v", result.HasEmptyFilter, tt.wantEmpty)
			}
			if result.Collection != tt.wantCollection {
				t.Errorf("Collection = %q, want %q", result.Collection, tt.wantCollection)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Delete Operations
// ---------------------------------------------------------------------------

func TestParseMongo_Delete(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantEmpty      bool
		wantCollection string
	}{
		{
			name:           "delete_with_filter",
			body:           `{"operation": "delete", "collection": "logs", "filter": {"created_at": {"$lt": "2024-01-01"}}}`,
			wantEmpty:      false,
			wantCollection: "logs",
		},
		{
			name:           "delete_without_filter",
			body:           `{"operation": "delete", "collection": "logs"}`,
			wantEmpty:      true,
			wantCollection: "logs",
		},
		{
			name:           "delete_with_empty_filter",
			body:           `{"operation": "delete", "collection": "logs", "filter": {}}`,
			wantEmpty:      true,
			wantCollection: "logs",
		},
		{
			name:           "delete_one",
			body:           `{"operation": "deleteOne", "collection": "logs", "filter": {"_id": "abc"}}`,
			wantEmpty:      false,
			wantCollection: "logs",
		},
		{
			name:           "delete_many_with_filter",
			body:           `{"operation": "deleteMany", "collection": "logs", "filter": {"level": "debug"}}`,
			wantEmpty:      false,
			wantCollection: "logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != MongoOpDelete {
				t.Errorf("Operation = %q, want %q", result.Operation, MongoOpDelete)
			}
			if result.HasEmptyFilter != tt.wantEmpty {
				t.Errorf("HasEmptyFilter = %v, want %v", result.HasEmptyFilter, tt.wantEmpty)
			}
			if result.Collection != tt.wantCollection {
				t.Errorf("Collection = %q, want %q", result.Collection, tt.wantCollection)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Aggregate Operations
// ---------------------------------------------------------------------------

func TestParseMongo_Aggregate(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantStages    []string
		wantDangerous bool
	}{
		{
			name:          "safe_aggregation_match_group",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$match": {"status": "active"}}, {"$group": {"_id": "$user_id", "total": {"$sum": "$amount"}}}]}`,
			wantStages:    []string{"$match", "$group"},
			wantDangerous: false,
		},
		{
			name:          "safe_aggregation_project_sort",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$project": {"name": 1, "total": 1}}, {"$sort": {"total": -1}}, {"$limit": 10}]}`,
			wantStages:    []string{"$project", "$sort", "$limit"},
			wantDangerous: false,
		},
		{
			name:          "dangerous_out",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$match": {}}, {"$out": "backup"}]}`,
			wantStages:    []string{"$match", "$out"},
			wantDangerous: true,
		},
		{
			name:          "dangerous_merge",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$merge": {"into": "target"}}]}`,
			wantStages:    []string{"$merge"},
			wantDangerous: true,
		},
		{
			name:          "allowed_unwind_count",
			body:          `{"operation": "aggregate", "collection": "users", "pipeline": [{"$unwind": "$tags"}, {"$count": "total"}]}`,
			wantStages:    []string{"$unwind", "$count"},
			wantDangerous: false,
		},
		{
			name:          "allowed_addFields",
			body:          `{"operation": "aggregate", "collection": "users", "pipeline": [{"$addFields": {"fullName": {"$concat": ["$first", " ", "$last"]}}}]}`,
			wantStages:    []string{"$addFields"},
			wantDangerous: false,
		},
		{
			name:          "empty_pipeline",
			body:          `{"operation": "aggregate", "collection": "users", "pipeline": []}`,
			wantStages:    nil,
			wantDangerous: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != MongoOpAggregate {
				t.Errorf("Operation = %q, want %q", result.Operation, MongoOpAggregate)
			}
			if !equalStringSlices(result.PipelineStages, tt.wantStages) {
				t.Errorf("PipelineStages = %v, want %v", result.PipelineStages, tt.wantStages)
			}
			if result.HasDangerousStage != tt.wantDangerous {
				t.Errorf("HasDangerousStage = %v, want %v", result.HasDangerousStage, tt.wantDangerous)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Inferred Operations (no explicit "operation" field)
// ---------------------------------------------------------------------------

func TestParseMongo_InferredOperations(t *testing.T) {
	tests := []struct {
		name string
		body string
		want MongoOperation
	}{
		{
			name: "infer_from_pipeline",
			body: `{"collection": "orders", "pipeline": [{"$match": {}}]}`,
			want: MongoOpAggregate,
		},
		{
			name: "infer_from_update_key",
			body: `{"collection": "users", "update": {"$set": {"x": 1}}}`,
			want: MongoOpUpdate,
		},
		{
			name: "infer_from_filter",
			body: `{"collection": "users", "filter": {"active": true}}`,
			want: MongoOpFind,
		},
		{
			name: "infer_default_find",
			body: `{"collection": "users"}`,
			want: MongoOpFind,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != tt.want {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Case-insensitive operation names
// ---------------------------------------------------------------------------

func TestParseMongo_CaseInsensitiveOps(t *testing.T) {
	tests := []struct {
		name string
		body string
		want MongoOperation
	}{
		{"Find", `{"operation": "Find"}`, MongoOpFind},
		{"FIND", `{"operation": "FIND"}`, MongoOpFind},
		{"Aggregate", `{"operation": "Aggregate"}`, MongoOpAggregate},
		{"UpdateOne", `{"operation": "UpdateOne"}`, MongoOpUpdate},
		{"DeleteMany", `{"operation": "DeleteMany"}`, MongoOpDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != tt.want {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Invalid Input
// ---------------------------------------------------------------------------

func TestParseMongo_InvalidInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty_string", ""},
		{"whitespace", "   "},
		{"invalid_json", "{invalid}"},
		{"broken_json", `{"operation": "find"`},
		{"array_not_object", `[1, 2, 3]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMongo(tt.body)
			if err == nil {
				t.Errorf("expected error for input %q", tt.body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Multi Flag
// ---------------------------------------------------------------------------

func TestParseMongo_MultiFlag(t *testing.T) {
	t.Run("multi_true", func(t *testing.T) {
		body := `{"operation": "update", "collection": "users", "multi": true, "filter": {"x": 1}, "update": {"$set": {"y": 2}}}`
		result, err := ParseMongo(body)
		if err != nil {
			t.Fatalf("ParseMongo error: %v", err)
		}
		if !result.IsMulti {
			t.Error("IsMulti = false, want true")
		}
	})

	t.Run("multi_false", func(t *testing.T) {
		body := `{"operation": "update", "collection": "users", "multi": false, "filter": {"x": 1}, "update": {"$set": {"y": 2}}}`
		result, err := ParseMongo(body)
		if err != nil {
			t.Fatalf("ParseMongo error: %v", err)
		}
		if result.IsMulti {
			t.Error("IsMulti = true, want false")
		}
	})

	t.Run("no_multi_field", func(t *testing.T) {
		body := `{"operation": "update", "collection": "users", "filter": {"x": 1}, "update": {"$set": {"y": 2}}}`
		result, err := ParseMongo(body)
		if err != nil {
			t.Fatalf("ParseMongo error: %v", err)
		}
		if result.IsMulti {
			t.Error("IsMulti should default to false")
		}
	})
}

// ---------------------------------------------------------------------------
// Filter edge cases
// ---------------------------------------------------------------------------

func TestParseMongo_FilterEdgeCases(t *testing.T) {
	t.Run("filter_null_value", func(t *testing.T) {
		body := `{"operation": "find", "collection": "users", "filter": null}`
		result, err := ParseMongo(body)
		if err != nil {
			t.Fatalf("ParseMongo error: %v", err)
		}
		if result.HasFilter {
			t.Error("HasFilter should be false for null filter")
		}
		if !result.HasEmptyFilter {
			t.Error("HasEmptyFilter should be true for null filter")
		}
	})

	t.Run("filter_non_empty_nested", func(t *testing.T) {
		body := `{"operation": "find", "collection": "users", "filter": {"address": {"city": "Shanghai"}}}`
		result, err := ParseMongo(body)
		if err != nil {
			t.Fatalf("ParseMongo error: %v", err)
		}
		if !result.HasFilter {
			t.Error("HasFilter should be true")
		}
		if result.HasEmptyFilter {
			t.Error("HasEmptyFilter should be false for nested filter")
		}
	})
}

// ---------------------------------------------------------------------------
// Additional Aggregation Stages ($lookup, $skip)
// ---------------------------------------------------------------------------

func TestParseMongo_AdditionalStages(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantStages    []string
		wantDangerous bool
	}{
		{
			name:          "lookup_stage",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$lookup": {"from": "users", "localField": "user_id", "foreignField": "_id", "as": "user"}}]}`,
			wantStages:    []string{"$lookup"},
			wantDangerous: true,
		},
		{
			name:          "skip_stage",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$skip": 10}]}`,
			wantStages:    []string{"$skip"},
			wantDangerous: false,
		},
		{
			name:          "mixed_safe_and_dangerous",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$match": {"status": "active"}}, {"$out": "backup"}]}`,
			wantStages:    []string{"$match", "$out"},
			wantDangerous: true,
		},
		{
			name:          "all_safe_stages",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$match": {}}, {"$group": {"_id": null}}, {"$sort": {"_id": 1}}, {"$limit": 10}, {"$skip": 0}]}`,
			wantStages:    []string{"$match", "$group", "$sort", "$limit", "$skip"},
			wantDangerous: false,
		},
		{
			name:          "unknown_stage_dangerous",
			body:          `{"operation": "aggregate", "collection": "t", "pipeline": [{"$randomStage": {}}]}`,
			wantStages:    []string{"$randomStage"},
			wantDangerous: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if !equalStringSlices(result.PipelineStages, tt.wantStages) {
				t.Errorf("PipelineStages = %v, want %v", result.PipelineStages, tt.wantStages)
			}
			if result.HasDangerousStage != tt.wantDangerous {
				t.Errorf("HasDangerousStage = %v, want %v", result.HasDangerousStage, tt.wantDangerous)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Extra fields in JSON are ignored
// ---------------------------------------------------------------------------

func TestParseMongo_ExtraFieldsIgnored(t *testing.T) {
	body := `{"operation": "find", "collection": "users", "filter": {"x": 1}, "extraField": "ignored", "another": 123}`
	result, err := ParseMongo(body)
	if err != nil {
		t.Fatalf("ParseMongo error: %v", err)
	}
	if result.Operation != MongoOpFind {
		t.Errorf("Operation = %q, want %q", result.Operation, MongoOpFind)
	}
	if result.Collection != "users" {
		t.Errorf("Collection = %q, want %q", result.Collection, "users")
	}
}
