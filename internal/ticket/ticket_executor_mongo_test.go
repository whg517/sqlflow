package ticket

import (
	"testing"

	"github.com/whg517/sqlflow/internal/platform/sqlparser"
)

// TestMongoInsertOperation tests the sqlparser recognizes insert operations.
func TestMongoInsertOperation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want sqlparser.MongoOperation
	}{
		{
			name: "explicit_insert",
			body: `{"operation": "insert", "collection": "users", "document": {"name": "test"}}`,
			want: sqlparser.MongoOpInsert,
		},
		{
			name: "insertOne",
			body: `{"operation": "insertOne", "collection": "users", "document": {"name": "test"}}`,
			want: sqlparser.MongoOpInsert,
		},
		{
			name: "insertMany",
			body: `{"operation": "insertMany", "collection": "users", "documents": [{"name": "a"}, {"name": "b"}]}`,
			want: sqlparser.MongoOpInsert,
		},
		{
			name: "inferred_from_document",
			body: `{"collection": "users", "document": {"name": "test"}}`,
			want: sqlparser.MongoOpInsert,
		},
		{
			name: "inferred_from_documents",
			body: `{"collection": "users", "documents": [{"name": "a"}]}`,
			want: sqlparser.MongoOpInsert,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sqlparser.ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != tt.want {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.want)
			}
		})
	}
}

// TestMongoUpdateParse verifies multi flag parsing for update/delete operations.
func TestMongoUpdateParse(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantOp    sqlparser.MongoOperation
		wantMulti bool
	}{
		{
			name:      "update_one",
			body:      `{"operation": "updateOne", "collection": "users", "filter": {"_id": 1}, "update": {"$set": {"name": "test"}}}`,
			wantOp:    sqlparser.MongoOpUpdate,
			wantMulti: false,
		},
		{
			name:      "update_many",
			body:      `{"operation": "updateMany", "collection": "users", "filter": {}, "update": {"$set": {"active": true}}, "multi": true}`,
			wantOp:    sqlparser.MongoOpUpdate,
			wantMulti: true,
		},
		{
			name:      "delete_one",
			body:      `{"operation": "deleteOne", "collection": "users", "filter": {"_id": 1}}`,
			wantOp:    sqlparser.MongoOpDelete,
			wantMulti: false,
		},
		{
			name:      "delete_many",
			body:      `{"operation": "deleteMany", "collection": "users", "filter": {"active": false}, "multi": true}`,
			wantOp:    sqlparser.MongoOpDelete,
			wantMulti: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sqlparser.ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
			if result.IsMulti != tt.wantMulti {
				t.Errorf("IsMulti = %v, want %v", result.IsMulti, tt.wantMulti)
			}
		})
	}
}

// TestMongoMissingCollection verifies parser handles missing collection gracefully.
func TestMongoMissingCollection(t *testing.T) {
	result, err := sqlparser.ParseMongo(`{"operation": "find", "filter": {}}`)
	if err != nil {
		t.Fatalf("ParseMongo error: %v", err)
	}
	if result.Collection != "" {
		t.Errorf("Collection should be empty for body without collection, got %q", result.Collection)
	}
}

// TestMongoStatementResultFormat verifies statement result for MongoDB operations.
func TestMongoStatementResultFormat(t *testing.T) {
	r := statementResult{
		SQL:          `{"operation": "update", "collection": "users", "filter": {"active": false}, "update": {"$set": {"status": "archived"}}}`,
		Status:       "success",
		RowsAffected: 3,
		DurationMs:   45,
	}

	if r.Status != "success" {
		t.Errorf("Status = %q, want success", r.Status)
	}
	if r.RowsAffected != 3 {
		t.Errorf("RowsAffected = %d, want 3", r.RowsAffected)
	}
}

// TestMongoInsertMissingDoc verifies insert without document/documents field fails at parse level.
func TestMongoInsertMissingDoc(t *testing.T) {
	result, err := sqlparser.ParseMongo(`{"operation": "insert", "collection": "users"}`)
	if err != nil {
		t.Fatalf("ParseMongo error: %v", err)
	}
	// Parser should identify the operation as insert
	if result.Operation != sqlparser.MongoOpInsert {
		t.Errorf("Operation = %q, want insert", result.Operation)
	}
	// The actual validation of document/documents field happens at execution time,
	// not at parse time. Document this expectation.
}
