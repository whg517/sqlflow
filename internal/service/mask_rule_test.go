package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return database.DB
}

func newTestMaskRuleService(t *testing.T) (*MaskRuleService, *sql.DB) {
	t.Helper()
	database := setupTestDB(t)
	svc := NewMaskRuleService(database, nil, nil)
	return svc, database
}

func TestCreateMaskRule(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	t.Run("success", func(t *testing.T) {
		rule, err := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "users", "phone", "phone", "", "")
		if err != nil {
			t.Fatalf("CreateMaskRule: %v", err)
		}
		if rule.ID == 0 {
			t.Error("expected non-zero ID")
		}
		if rule.TableName != "users" {
			t.Errorf("TableName = %q, want users", rule.TableName)
		}
		if rule.Field != "phone" {
			t.Errorf("Field = %q, want phone", rule.Field)
		}
		if rule.MaskType != "phone" {
			t.Errorf("MaskType = %q, want phone", rule.MaskType)
		}
	})

	t.Run("missing table name", func(t *testing.T) {
		_, err := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "", "phone", "phone", "", "")
		if err != ErrMaskRuleTableRequired {
			t.Errorf("expected ErrMaskRuleTableRequired, got %v", err)
		}
	})

	t.Run("missing field name", func(t *testing.T) {
		_, err := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "users", "", "phone", "", "")
		if err != ErrMaskRuleFieldRequired {
			t.Errorf("expected ErrMaskRuleFieldRequired, got %v", err)
		}
	})

	t.Run("invalid mask type", func(t *testing.T) {
		_, err := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "users", "phone", "invalid_type", "", "")
		if err != ErrMaskRuleTypeInvalid {
			t.Errorf("expected ErrMaskRuleTypeInvalid, got %v", err)
		}
	})

	t.Run("custom without regex", func(t *testing.T) {
		_, err := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "users", "phone", "custom", "", "")
		if err != ErrMaskRuleCustomRegexRequired {
			t.Errorf("expected ErrMaskRuleCustomRegexRequired, got %v", err)
		}
	})

	t.Run("custom with regex succeeds", func(t *testing.T) {
		rule, err := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "users", "id_number", "custom", `\d{3}\d+`, "***")
		if err != nil {
			t.Fatalf("CreateMaskRule custom: %v", err)
		}
		if rule.CustomRegex != `\d{3}\d+` {
			t.Errorf("CustomRegex = %q, want \\d{3}\\d+", rule.CustomRegex)
		}
	})

	t.Run("duplicate rule", func(t *testing.T) {
		// Create first
		_, _ = svc.CreateMaskRule(context.Background(), 0, 2, "mydb", "orders", "amount", "full", "", "")
		// Create duplicate
		_, err := svc.CreateMaskRule(context.Background(), 0, 2, "mydb", "orders", "amount", "full", "", "")
		if err != ErrMaskRuleDuplicate {
			t.Errorf("expected ErrMaskRuleDuplicate, got %v", err)
		}
	})
}

func TestGetMaskRule(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	rule, _ := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "users", "phone", "phone", "", "")

	t.Run("existing rule", func(t *testing.T) {
		got, err := svc.GetMaskRule(context.Background(), rule.ID)
		if err != nil {
			t.Fatalf("GetMaskRule: %v", err)
		}
		if got.Field != "phone" {
			t.Errorf("Field = %q, want phone", got.Field)
		}
	})

	t.Run("non-existent rule", func(t *testing.T) {
		_, err := svc.GetMaskRule(context.Background(), 99999)
		if err != ErrMaskRuleNotFound {
			t.Errorf("expected ErrMaskRuleNotFound, got %v", err)
		}
	})
}

func TestListMaskRules(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	svc.CreateMaskRule(context.Background(), 0, 1, "db1", "users", "phone", "phone", "", "")
	svc.CreateMaskRule(context.Background(), 0, 1, "db1", "users", "email", "email", "", "")
	svc.CreateMaskRule(context.Background(), 0, 1, "db1", "orders", "amount", "full", "", "")
	svc.CreateMaskRule(context.Background(), 0, 2, "db2", "products", "price", "full", "", "")

	t.Run("list all", func(t *testing.T) {
		rules, total, err := svc.ListMaskRules(context.Background(), 1, 10, "", "", "")
		if err != nil {
			t.Fatalf("ListMaskRules: %v", err)
		}
		if total != 4 {
			t.Errorf("total = %d, want 4", total)
		}
		if len(rules) != 4 {
			t.Errorf("len = %d, want 4", len(rules))
		}
	})

	t.Run("filter by datasource", func(t *testing.T) {
		rules, total, err := svc.ListMaskRules(context.Background(), 1, 10, "2", "", "")
		if err != nil {
			t.Fatalf("ListMaskRules: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(rules) != 1 {
			t.Errorf("len = %d, want 1", len(rules))
		}
	})

	t.Run("filter by table", func(t *testing.T) {
		_, total, err := svc.ListMaskRules(context.Background(), 1, 10, "", "", "users")
		if err != nil {
			t.Fatalf("ListMaskRules: %v", err)
		}
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		rules, total, err := svc.ListMaskRules(context.Background(), 1, 2, "", "", "")
		if err != nil {
			t.Fatalf("ListMaskRules: %v", err)
		}
		if total != 4 {
			t.Errorf("total = %d, want 4", total)
		}
		if len(rules) != 2 {
			t.Errorf("len = %d, want 2 (page size)", len(rules))
		}
	})
}

func TestUpdateMaskRule(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	rule, _ := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "users", "phone", "phone", "", "")

	t.Run("update mask type", func(t *testing.T) {
		updated, err := svc.UpdateMaskRule(context.Background(), 0, rule.ID, "", "", "full", "", "")
		if err != nil {
			t.Fatalf("UpdateMaskRule: %v", err)
		}
		if updated.MaskType != "full" {
			t.Errorf("MaskType = %q, want full", updated.MaskType)
		}
	})

	t.Run("update non-existent", func(t *testing.T) {
		_, err := svc.UpdateMaskRule(context.Background(), 0, 99999, "", "", "full", "", "")
		if err != ErrMaskRuleNotFound {
			t.Errorf("expected ErrMaskRuleNotFound, got %v", err)
		}
	})

	t.Run("update to invalid type", func(t *testing.T) {
		_, err := svc.UpdateMaskRule(context.Background(), 0, rule.ID, "", "", "invalid", "", "")
		if err != ErrMaskRuleTypeInvalid {
			t.Errorf("expected ErrMaskRuleTypeInvalid, got %v", err)
		}
	})
}

func TestDeleteMaskRule(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	rule, _ := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "users", "phone", "phone", "", "")

	t.Run("delete existing", func(t *testing.T) {
		err := svc.DeleteMaskRule(context.Background(), 0, rule.ID)
		if err != nil {
			t.Fatalf("DeleteMaskRule: %v", err)
		}
		// Verify deleted
		_, err = svc.GetMaskRule(context.Background(), rule.ID)
		if err != ErrMaskRuleNotFound {
			t.Errorf("expected ErrMaskRuleNotFound after delete, got %v", err)
		}
	})

	t.Run("delete non-existent", func(t *testing.T) {
		err := svc.DeleteMaskRule(context.Background(), 0, 99999)
		if err != ErrMaskRuleNotFound {
			t.Errorf("expected ErrMaskRuleNotFound, got %v", err)
		}
	})
}

func TestCreateSensitiveTable(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	t.Run("success", func(t *testing.T) {
		st, err := svc.CreateSensitiveTable(context.Background(), 0, 1, "mydb", "users", "high")
		if err != nil {
			t.Fatalf("CreateSensitiveTable: %v", err)
		}
		if st.ID == 0 {
			t.Error("expected non-zero ID")
		}
		if st.SensitivityLevel != "high" {
			t.Errorf("SensitivityLevel = %q, want high", st.SensitivityLevel)
		}
	})

	t.Run("missing table name", func(t *testing.T) {
		_, err := svc.CreateSensitiveTable(context.Background(), 0, 1, "mydb", "", "medium")
		if err != ErrSensitiveTableRequired {
			t.Errorf("expected ErrSensitiveTableRequired, got %v", err)
		}
	})

	t.Run("invalid sensitivity level", func(t *testing.T) {
		_, err := svc.CreateSensitiveTable(context.Background(), 0, 1, "mydb", "test_table", "critical")
		if err != ErrInvalidSensitivityLevel {
			t.Errorf("expected ErrInvalidSensitivityLevel, got %v", err)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		_, _ = svc.CreateSensitiveTable(context.Background(), 0, 2, "mydb", "orders", "medium")
		_, err := svc.CreateSensitiveTable(context.Background(), 0, 2, "mydb", "orders", "low")
		if err != ErrSensitiveTableDuplicate {
			t.Errorf("expected ErrSensitiveTableDuplicate, got %v", err)
		}
	})
}

func TestListSensitiveTables(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	svc.CreateSensitiveTable(context.Background(), 0, 1, "db1", "users", "high")
	svc.CreateSensitiveTable(context.Background(), 0, 1, "db1", "accounts", "medium")
	svc.CreateSensitiveTable(context.Background(), 0, 2, "db2", "orders", "low")

	t.Run("list all", func(t *testing.T) {
		tables, total, err := svc.ListSensitiveTables(context.Background(), 1, 10, "", "", "")
		if err != nil {
			t.Fatalf("ListSensitiveTables: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(tables) != 3 {
			t.Errorf("len = %d, want 3", len(tables))
		}
	})

	t.Run("filter by datasource", func(t *testing.T) {
		tables, total, err := svc.ListSensitiveTables(context.Background(), 1, 10, "1", "", "")
		if err != nil {
			t.Fatalf("ListSensitiveTables: %v", err)
		}
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
		if len(tables) != 2 {
			t.Errorf("len = %d, want 2", len(tables))
		}
	})

	t.Run("filter by table name fuzzy", func(t *testing.T) {
		_, total, err := svc.ListSensitiveTables(context.Background(), 1, 10, "", "", "user")
		if err != nil {
			t.Fatalf("ListSensitiveTables: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
	})
}

func TestDeleteSensitiveTable(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	st, _ := svc.CreateSensitiveTable(context.Background(), 0, 1, "mydb", "users", "high")

	t.Run("delete existing", func(t *testing.T) {
		err := svc.DeleteSensitiveTable(context.Background(), 0, st.ID)
		if err != nil {
			t.Fatalf("DeleteSensitiveTable: %v", err)
		}
	})

	t.Run("delete non-existent", func(t *testing.T) {
		err := svc.DeleteSensitiveTable(context.Background(), 0, 99999)
		if err != ErrSensitiveTableNotFound {
			t.Errorf("expected ErrSensitiveTableNotFound, got %v", err)
		}
	})
}

func TestGetSensitiveTablesForDatasource(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	svc.CreateSensitiveTable(context.Background(), 0, 1, "db1", "users", "high")
	svc.CreateSensitiveTable(context.Background(), 0, 1, "db1", "accounts", "medium")
	svc.CreateSensitiveTable(context.Background(), 0, 1, "", "global_table", "low")
	svc.CreateSensitiveTable(context.Background(), 0, 2, "db2", "orders", "high")

	t.Run("filter by datasource and database", func(t *testing.T) {
		tables, err := svc.GetSensitiveTablesForDatasource(context.Background(), 1, "db1")
		if err != nil {
			t.Fatalf("GetSensitiveTablesForDatasource: %v", err)
		}
		// Should get db1-specific + empty-database entries
		if len(tables) != 3 {
			t.Errorf("len = %d, want 3", len(tables))
		}
	})

	t.Run("filter by datasource only", func(t *testing.T) {
		tables, err := svc.GetSensitiveTablesForDatasource(context.Background(), 1, "")
		if err != nil {
			t.Fatalf("GetSensitiveTablesForDatasource: %v", err)
		}
		if len(tables) != 3 {
			t.Errorf("len = %d, want 3", len(tables))
		}
	})
}

func TestHasDesensitizeBypass_NoPermSvc(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)
	// permSvc is nil, should always return false
	if svc.HasDesensitizeBypass("admin", 1, []string{"users"}) {
		t.Error("expected false when permSvc is nil")
	}
}

func TestMaskRuleAllTypes(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	maskTypes := []string{"phone", "id_card", "name", "email", "bank_card", "address", "full"}
	for _, mt := range maskTypes {
		t.Run(mt, func(t *testing.T) {
			rule, err := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "test_table", "field_"+mt, mt, "", "")
			if err != nil {
				t.Fatalf("CreateMaskRule(%s): %v", mt, err)
			}
			if rule.MaskType != mt {
				t.Errorf("MaskType = %q, want %q", rule.MaskType, mt)
			}
		})
	}
}

// Ensure timestamp fields are set correctly
func TestMaskRuleTimestamps(t *testing.T) {
	svc, _ := newTestMaskRuleService(t)

	before := time.Now()
	rule, _ := svc.CreateMaskRule(context.Background(), 0, 1, "mydb", "users", "phone", "phone", "", "")
	after := time.Now()

	if rule.CreatedAt.Before(before) || rule.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, expected between %v and %v", rule.CreatedAt, before, after)
	}
	if rule.UpdatedAt.Before(before) || rule.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, expected between %v and %v", rule.UpdatedAt, before, after)
	}
}
