package service

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

func setupHistoryService(t *testing.T) *QueryHistoryService {
	t.Helper()
	InvalidateFrequentQueryCache()
	conn := setupTestDB(t)
	database := mustWrapDB(conn)
	return NewQueryHistoryService(database)
}

func TestGetFrequentQueries_EmptyUser(t *testing.T) {
	svc := setupHistoryService(t)

	results, err := svc.GetFrequentQueries(context.Background(), 9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestGetFrequentQueries_WithRecords(t *testing.T) {
	svc := setupHistoryService(t)
	ctx := context.Background()
	userID := int64(1)

	// Insert 3 identical queries
	sqlA := "SELECT * FROM users WHERE id = 1"
	for i := 0; i < 3; i++ {
		_, err := svc.CreateHistory(ctx, &model.QueryHistory{
			UserID:       userID,
			DatasourceID: 1,
			Database:     "testdb",
			SQLContent:   sqlA,
		})
		if err != nil {
			t.Fatalf("CreateHistory: %v", err)
		}
	}

	// Insert 1 different query
	_, err := svc.CreateHistory(ctx, &model.QueryHistory{
		UserID:       userID,
		DatasourceID: 1,
		Database:     "testdb",
		SQLContent:   "SELECT name FROM products LIMIT 10",
	})
	if err != nil {
		t.Fatalf("CreateHistory: %v", err)
	}

	results, err := svc.GetFrequentQueries(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First should be the more frequent one
	if results[0].ExecutionCount != 3 {
		t.Errorf("first result count = %d, want 3", results[0].ExecutionCount)
	}
	if results[0].SQLHash == "" {
		t.Error("sql_hash should not be empty")
	}
	if results[1].ExecutionCount != 1 {
		t.Errorf("second result count = %d, want 1", results[1].ExecutionCount)
	}
}

func TestGetFrequentQueries_SnippetTruncation(t *testing.T) {
	svc := setupHistoryService(t)
	ctx := context.Background()

	// Insert query with >80 chars
	longSQL := "SELECT id, name, email, created_at FROM very_long_table_name WHERE status = 'active' AND created_at > '2024-01-01' ORDER BY id"
	_, err := svc.CreateHistory(ctx, &model.QueryHistory{
		UserID:       1,
		DatasourceID: 1,
		Database:     "testdb",
		SQLContent:   longSQL,
	})
	if err != nil {
		t.Fatalf("CreateHistory: %v", err)
	}

	results, err := svc.GetFrequentQueries(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Snippet) > 80 {
		t.Errorf("snippet length = %d, want <= 80", len(results[0].Snippet))
	}
}

func TestGetFrequentQueries_UserIsolation(t *testing.T) {
	svc := setupHistoryService(t)
	ctx := context.Background()

	// User 1 inserts query
	_, _ = svc.CreateHistory(ctx, &model.QueryHistory{
		UserID:       1,
		DatasourceID: 1,
		Database:     "testdb",
		SQLContent:   "SELECT * FROM secret_table",
	})

	// User 2 should not see user 1's queries
	results, err := svc.GetFrequentQueries(ctx, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("user 2 should see 0 results, got %d", len(results))
	}
}

func TestGetFrequentQueries_Deduplication(t *testing.T) {
	svc := setupHistoryService(t)
	ctx := context.Background()

	// Same SQL with different whitespace should be deduplicated
	queries := []string{
		"SELECT 1",
		"SELECT  1",
		"select 1",
		"  SELECT 1  ",
	}
	for _, sql := range queries {
		_, err := svc.CreateHistory(ctx, &model.QueryHistory{
			UserID:       1,
			DatasourceID: 1,
			Database:     "testdb",
			SQLContent:   sql,
		})
		if err != nil {
			t.Fatalf("CreateHistory: %v", err)
		}
	}

	results, err := svc.GetFrequentQueries(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 deduplicated result, got %d", len(results))
	}
	if results[0].ExecutionCount != 4 {
		t.Errorf("execution count = %d, want 4", results[0].ExecutionCount)
	}
}
