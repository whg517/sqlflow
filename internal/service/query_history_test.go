package service

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

func TestQueryHistoryService_CreateAndList(t *testing.T) {
	testDB := setupIntegrationDB(t)
	svc := NewQueryHistoryService(mustWrapDB(testDB))

	userID := seedIntegrationUser(t, testDB, "qh_user", "developer")
	dsID := seedIntegrationDatasource(t, testDB, "qh-ds")

	t.Run("create_and_list_single", func(t *testing.T) {
		h := &model.QueryHistory{
			UserID:        userID,
			DatasourceID:  dsID,
			Database:      "testdb",
			SQLContent:    "SELECT * FROM users LIMIT 10",
			SQLSummary:    "SELECT * FROM users",
			DBType:        "mysql",
			ExecutionTime: 25,
			ResultRows:    10,
			AffectedRows:  0,
		}
		if err := svc.CreateHistory(context.Background(), h); err != nil {
			t.Fatalf("CreateHistory: %v", err)
		}

		list, total, err := svc.ListHistory(context.Background(), userID, 1, 10, "")
		if err != nil {
			t.Fatalf("ListHistory: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(list) != 1 {
			t.Fatalf("len(list) = %d, want 1", len(list))
		}
		if list[0].SQLContent != "SELECT * FROM users LIMIT 10" {
			t.Errorf("SQLContent = %q, want original", list[0].SQLContent)
		}
		if list[0].UserID != userID {
			t.Errorf("UserID = %d, want %d", list[0].UserID, userID)
		}
		if list[0].ExecutionTime != 25 {
			t.Errorf("ExecutionTime = %d, want 25", list[0].ExecutionTime)
		}
	})

	t.Run("list_pagination", func(t *testing.T) {
		// Insert 5 more records
		for i := 0; i < 5; i++ {
			h := &model.QueryHistory{
				UserID:        userID,
				DatasourceID:  dsID,
				Database:      "testdb",
				SQLContent:    "SELECT 1",
				SQLSummary:    "SELECT 1",
				DBType:        "mysql",
				ExecutionTime: int64(i),
				ResultRows:    1,
				AffectedRows:  0,
			}
			if err := svc.CreateHistory(context.Background(), h); err != nil {
				t.Fatalf("CreateHistory %d: %v", i, err)
			}
		}

		// Total should be 6 (1 from previous + 5 new)
		_, total, err := svc.ListHistory(context.Background(), userID, 1, 10, "")
		if err != nil {
			t.Fatalf("ListHistory total: %v", err)
		}
		if total != 6 {
			t.Errorf("total = %d, want 6", total)
		}

		// Page 1 with size 3 => 3 items
		page1, total, err := svc.ListHistory(context.Background(), userID, 1, 3, "")
		if err != nil {
			t.Fatalf("ListHistory page1: %v", err)
		}
		if total != 6 {
			t.Errorf("total = %d, want 6", total)
		}
		if len(page1) != 3 {
			t.Errorf("len(page1) = %d, want 3", len(page1))
		}

		// Page 2 with size 3 => 3 items
		page2, _, err := svc.ListHistory(context.Background(), userID, 2, 3, "")
		if err != nil {
			t.Fatalf("ListHistory page2: %v", err)
		}
		if len(page2) != 3 {
			t.Errorf("len(page2) = %d, want 3", len(page2))
		}

		// Page 3 with size 3 => 0 items (6 records, page3 starts at offset 6)
		page3, _, err := svc.ListHistory(context.Background(), userID, 3, 3, "")
		if err != nil {
			t.Fatalf("ListHistory page3: %v", err)
		}
		if len(page3) != 0 {
			t.Errorf("len(page3) = %d, want 0", len(page3))
		}
	})

	t.Run("list_orders_by_id_desc", func(t *testing.T) {
		list, _, err := svc.ListHistory(context.Background(), userID, 1, 2, "")
		if err != nil {
			t.Fatalf("ListHistory: %v", err)
		}
		if len(list) < 2 {
			t.Fatalf("need at least 2 records, got %d", len(list))
		}
		if list[0].ID <= list[1].ID {
			t.Errorf("expected DESC order: id[0]=%d should be > id[1]=%d", list[0].ID, list[1].ID)
		}
	})

	t.Run("list_empty_for_other_user", func(t *testing.T) {
		otherID := seedIntegrationUser(t, testDB, "qh_other", "developer")
		list, total, err := svc.ListHistory(context.Background(), otherID, 1, 10, "")
		if err != nil {
			t.Fatalf("ListHistory other: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0 for other user", total)
		}
		if len(list) != 0 {
			t.Errorf("len(list) = %d, want 0 for other user", len(list))
		}
	})
}

func TestQueryHistoryService_DeleteHistory(t *testing.T) {
	testDB := setupIntegrationDB(t)
	svc := NewQueryHistoryService(mustWrapDB(testDB))

	userID := seedIntegrationUser(t, testDB, "qh_del_user", "developer")
	dsID := seedIntegrationDatasource(t, testDB, "qh-del-ds")

	t.Run("delete_own_record", func(t *testing.T) {
		h := &model.QueryHistory{
			UserID: userID, DatasourceID: dsID, Database: "db",
			SQLContent: "SELECT 1", SQLSummary: "SELECT 1", DBType: "mysql",
		}
		if err := svc.CreateHistory(context.Background(), h); err != nil {
			t.Fatalf("CreateHistory: %v", err)
		}

		_, total, _ := svc.ListHistory(context.Background(), userID, 1, 10, "")
		if total != 1 {
			t.Fatalf("total before delete = %d, want 1", total)
		}

		// Get the ID
		list, _, _ := svc.ListHistory(context.Background(), userID, 1, 10, "")
		recordID := list[0].ID

		if err := svc.DeleteHistory(context.Background(), recordID, userID); err != nil {
			t.Fatalf("DeleteHistory: %v", err)
		}

		_, total, _ = svc.ListHistory(context.Background(), userID, 1, 10, "")
		if total != 0 {
			t.Errorf("total after delete = %d, want 0", total)
		}
	})

	t.Run("delete_other_user_record_fails", func(t *testing.T) {
		otherID := seedIntegrationUser(t, testDB, "qh_del_other", "developer")

		h := &model.QueryHistory{
			UserID: userID, DatasourceID: dsID, Database: "db",
			SQLContent: "SELECT 2", SQLSummary: "SELECT 2", DBType: "mysql",
		}
		svc.CreateHistory(context.Background(), h)

		list, _, _ := svc.ListHistory(context.Background(), userID, 1, 10, "")
		recordID := list[0].ID

		err := svc.DeleteHistory(context.Background(), recordID, otherID)
		if err == nil {
			t.Error("expected error when deleting other user's record")
		}
	})

	t.Run("delete_nonexistent_record", func(t *testing.T) {
		err := svc.DeleteHistory(context.Background(), 99999, userID)
		if err == nil {
			t.Error("expected error for nonexistent record")
		}
	})
}

func TestQueryHistoryService_ClearHistory(t *testing.T) {
	testDB := setupIntegrationDB(t)
	svc := NewQueryHistoryService(mustWrapDB(testDB))

	userID := seedIntegrationUser(t, testDB, "qh_clear_user", "developer")
	dsID := seedIntegrationDatasource(t, testDB, "qh-clear-ds")

	// Insert 3 records
	for i := 0; i < 3; i++ {
		h := &model.QueryHistory{
			UserID: userID, DatasourceID: dsID, Database: "db",
			SQLContent: "SELECT 1", SQLSummary: "SELECT 1", DBType: "mysql",
		}
		if err := svc.CreateHistory(context.Background(), h); err != nil {
			t.Fatalf("CreateHistory %d: %v", i, err)
		}
	}

	_, total, _ := svc.ListHistory(context.Background(), userID, 1, 10, "")
	if total != 3 {
		t.Fatalf("total before clear = %d, want 3", total)
	}

	t.Run("clear_all", func(t *testing.T) {
		if err := svc.ClearHistory(context.Background(), userID); err != nil {
			t.Fatalf("ClearHistory: %v", err)
		}

		_, total, err := svc.ListHistory(context.Background(), userID, 1, 10, "")
		if err != nil {
			t.Fatalf("ListHistory after clear: %v", err)
		}
		if total != 0 {
			t.Errorf("total after clear = %d, want 0", total)
		}
	})

	t.Run("clear_does_not_affect_other_users", func(t *testing.T) {
		otherID := seedIntegrationUser(t, testDB, "qh_clear_other", "developer")
		for i := 0; i < 2; i++ {
			h := &model.QueryHistory{
				UserID: otherID, DatasourceID: dsID, Database: "db",
				SQLContent: "SELECT 2", SQLSummary: "SELECT 2", DBType: "mysql",
			}
			svc.CreateHistory(context.Background(), h)
		}

		// Clear first user (already cleared, but idempotent)
		svc.ClearHistory(context.Background(), userID)

		_, total, _ := svc.ListHistory(context.Background(), otherID, 1, 10, "")
		if total != 2 {
			t.Errorf("other user total = %d, want 2 (should not be affected)", total)
		}
	})
}

func TestQueryHistoryService_CreateHistoryWithZeroValues(t *testing.T) {
	testDB := setupIntegrationDB(t)
	svc := NewQueryHistoryService(mustWrapDB(testDB))

	userID := seedIntegrationUser(t, testDB, "qh_zero_user", "developer")
	dsID := seedIntegrationDatasource(t, testDB, "qh-zero-ds")

	t.Run("zero_optional_fields", func(t *testing.T) {
		h := &model.QueryHistory{
			UserID:        userID,
			DatasourceID:  dsID,
			Database:      "",
			SQLContent:    "SELECT 1",
			SQLSummary:    "",
			DBType:        "mysql",
			ExecutionTime: 0,
			ResultRows:    0,
			AffectedRows:  0,
		}
		if err := svc.CreateHistory(context.Background(), h); err != nil {
			t.Fatalf("CreateHistory: %v", err)
		}

		list, total, err := svc.ListHistory(context.Background(), userID, 1, 10, "")
		if err != nil {
			t.Fatalf("ListHistory: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(list) != 1 {
			t.Fatalf("len(list) = %d, want 1", len(list))
		}
		if list[0].Database != "" {
			t.Errorf("Database = %q, want empty", list[0].Database)
		}
		if list[0].ExecutionTime != 0 {
			t.Errorf("ExecutionTime = %d, want 0", list[0].ExecutionTime)
		}
	})
}
