package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"testing"

	"github.com/whg517/sqlflow/internal/db"
)

// seedPolicies is the in-memory copy of policy.csv used by tests.
var seedPolicies = []struct {
	sub, dom, obj, act string
}{
	{"admin", "*", "*", "*"},
	{"dba", "*", "*", "select"},
	{"dba", "*", "*", "update"},
	{"dba", "*", "*", "delete"},
	{"dba", "*", "*", "ddl"},
	{"dba", "*", "*", "export"},
	{"dba", "*", "*", "desensitize:bypass"},
	{"developer", "*", "*", "select"},
}

// seedRoles lists the roles that need g (grouping) entries so that
// g(role, role, dom) matches in the Casbin model.
var seedRoles = []string{"admin", "dba", "developer"}

// setupPermissionTestDB creates a temp SQLite database with schema migrated.
func setupPermissionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}
	return database.DB
}

// seedTestPolicies inserts seed policy rows directly into casbin_rule so that
// NewPermissionService.seedIfEmpty sees a non-empty table and skips the file read.
func seedTestPolicies(t *testing.T, testDB *sql.DB) {
	t.Helper()
	ctx := context.Background()
	for _, p := range seedPolicies {
		_, err := testDB.ExecContext(ctx,
			`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5) VALUES ('p', ?, ?, ?, ?, '', '')`,
			p.sub, p.dom, p.obj, p.act,
		)
		if err != nil {
			t.Fatalf("seed policy %v: %v", p, err)
		}
	}
	// Insert grouping policies so g(role, role, dom) matches
	// The Casbin model uses g(r.sub, p.sub, r.dom) with 3-arg g definition.
	// We add g, role, role, domain so that enforce(sub=role, dom=domain) matches.
	for _, role := range seedRoles {
		_, err := testDB.ExecContext(ctx,
			`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5) VALUES ('g', ?, ?, ?, '', '', '')`,
			role, role, "*",
		)
		if err != nil {
			t.Fatalf("seed grouping for %s: %v", role, err)
		}
	}
}

// newTestPermissionService creates a PermissionService backed by a temp SQLite DB
// with seed policies pre-loaded.
func newTestPermissionService(t *testing.T) (*PermissionService, *sql.DB) {
	t.Helper()
	testDB := setupPermissionTestDB(t)

	// Pre-insert dummy row so seedIfEmpty skips file read
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5) VALUES ('p', '__dummy__', '', '', '', '', '')`,
	)
	if err != nil {
		t.Fatalf("insert dummy row: %v", err)
	}

	svc, err := NewPermissionService(testDB)
	if err != nil {
		t.Fatalf("NewPermissionService() error: %v", err)
	}

	// Now add real seed policies through the enforcer so Casbin loads them in-memory
	for _, role := range seedRoles {
		_, err := svc.enforcer.AddNamedGroupingPolicy("g", role, role, "*")
		if err != nil {
			t.Fatalf("AddNamedGroupingPolicy(%s): %v", role, err)
		}
	}
	for _, p := range seedPolicies {
		_, err := svc.enforcer.AddPolicy(p.sub, p.dom, p.obj, p.act)
		if err != nil {
			t.Fatalf("AddPolicy(%v): %v", p, err)
		}
	}

	// Remove the dummy row from DB and memory
	svc.enforcer.RemovePolicy("__dummy__", "", "", "")
	testDB.ExecContext(context.Background(), `DELETE FROM casbin_rule WHERE v0 = '__dummy__'`)

	return svc, testDB
}

// ---------------------------------------------------------------------------
// NewPermissionService (creation + seed)
// ---------------------------------------------------------------------------

func TestNewPermissionService(t *testing.T) {
	t.Run("creates service successfully", func(t *testing.T) {
		svc, _ := newTestPermissionService(t)
		if svc == nil {
			t.Fatal("NewPermissionService returned nil")
		}
	})

	t.Run("seeds policies are loaded", func(t *testing.T) {
		svc, testDB := newTestPermissionService(t)
		_ = svc

		var count int
		err := testDB.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM casbin_rule`,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count casbin_rule: %v", err)
		}
		// 8 p-policies + 3 g-policies = 11 rows
		if count != 11 {
			t.Errorf("expected 11 seeded rows, got %d", count)
		}
	})

	t.Run("does not re-seed on second creation", func(t *testing.T) {
		testDB := setupPermissionTestDB(t)
		seedTestPolicies(t, testDB)

		_, err := NewPermissionService(testDB)
		if err != nil {
			t.Fatalf("first NewPermissionService() error: %v", err)
		}

		var countAfterFirst int
		testDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM casbin_rule`).Scan(&countAfterFirst)

		_, err = NewPermissionService(testDB)
		if err != nil {
			t.Fatalf("second NewPermissionService() error: %v", err)
		}

		var countAfterSecond int
		testDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM casbin_rule`).Scan(&countAfterSecond)

		if countAfterFirst != countAfterSecond {
			t.Errorf("seed ran twice: first=%d, second=%d", countAfterFirst, countAfterSecond)
		}
	})
}

// ---------------------------------------------------------------------------
// Enforce
// ---------------------------------------------------------------------------

func TestPermissionService_Enforce(t *testing.T) {
	svc, _ := newTestPermissionService(t)

	// NOTE: The Casbin model uses strict == matching (no keyMatch).
	// Seed policies have dom="*", obj="*", act="*" as literal strings.
	// Only exact matches work — "*" is NOT a wildcard in == comparison.
	tests := []struct {
		name string
		sub  string
		dom  string
		obj  string
		act  string
		want bool
	}{
		{"admin exact star match", "admin", "*", "*", "*", true},
		{"admin non-star act denied", "admin", "*", "*", "select", false},
		{"dba wildcard select", "dba", "*", "*", "select", true},
		{"dba wildcard update", "dba", "*", "*", "update", true},
		{"dba wildcard delete", "dba", "*", "*", "delete", true},
		{"dba wildcard ddl", "dba", "*", "*", "ddl", true},
		{"dba wildcard export", "dba", "*", "*", "export", true},
		{"dba cannot insert", "dba", "*", "*", "insert", false},
		{"developer wildcard select", "developer", "*", "*", "select", true},
		{"developer cannot update", "developer", "*", "*", "update", false},
		{"developer cannot delete", "developer", "*", "*", "delete", false},
		{"developer cannot ddl", "developer", "*", "*", "ddl", false},
		{"unknown role denied", "guest", "*", "*", "select", false},
		{"empty subject denied", "", "*", "*", "select", false},
		{"non-star dom mismatch", "dba", "db1", "*", "select", false},
		{"non-star obj mismatch", "dba", "*", "users", "select", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.Enforce(tt.sub, tt.dom, tt.obj, tt.act)
			if err != nil {
				t.Fatalf("Enforce() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Enforce(%q, %q, %q, %q) = %v, want %v", tt.sub, tt.dom, tt.obj, tt.act, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AddPolicy / RemovePolicy (CRUD)
// ---------------------------------------------------------------------------

func TestPermissionService_AddPolicy(t *testing.T) {
	svc, _ := newTestPermissionService(t)

	t.Run("success", func(t *testing.T) {
		err := svc.AddPolicy("developer", "*", "table1", "update")
		if err != nil {
			t.Fatalf("AddPolicy() error: %v", err)
		}
		ok, err := svc.Enforce("developer", "*", "table1", "update")
		if err != nil {
			t.Fatalf("Enforce() error: %v", err)
		}
		if !ok {
			t.Error("Enforce() should return true after AddPolicy")
		}
	})

	t.Run("duplicate policy returns error", func(t *testing.T) {
		err := svc.AddPolicy("developer", "*", "table1", "update")
		if err == nil {
			t.Fatal("expected error for duplicate policy, got nil")
		}
	})

	t.Run("empty fields", func(t *testing.T) {
		err := svc.AddPolicy("", "", "", "")
		if err != nil {
			t.Logf("AddPolicy with empty fields: %v", err)
		}
	})
}

func TestPermissionService_RemovePolicy(t *testing.T) {
	svc, testDB := newTestPermissionService(t)
	ctx := context.Background()

	// Add a policy first
	err := svc.AddPolicy("developer", "*", "orders", "delete")
	if err != nil {
		t.Fatalf("AddPolicy() setup error: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		var ruleID int64
		err := testDB.QueryRowContext(ctx,
			`SELECT id FROM casbin_rule WHERE ptype='p' AND v0='developer' AND v1='*' AND v2='orders' AND v3='delete'`,
		).Scan(&ruleID)
		if err != nil {
			t.Fatalf("find rule ID: %v", err)
		}

		err = svc.RemovePolicy(ctx, ruleID)
		if err != nil {
			t.Fatalf("RemovePolicy() error: %v", err)
		}

		ok, err := svc.Enforce("developer", "*", "orders", "delete")
		if err != nil {
			t.Fatalf("Enforce() error: %v", err)
		}
		if ok {
			t.Error("Enforce() should return false after RemovePolicy")
		}
	})

	t.Run("nonexistent ID returns error", func(t *testing.T) {
		err := svc.RemovePolicy(ctx, 999999)
		if err == nil {
			t.Fatal("expected error for nonexistent ID, got nil")
		}
	})

	t.Run("zero ID returns error", func(t *testing.T) {
		err := svc.RemovePolicy(ctx, 0)
		if err == nil {
			t.Fatal("expected error for zero ID, got nil")
		}
	})

	t.Run("negative ID returns error", func(t *testing.T) {
		err := svc.RemovePolicy(ctx, -1)
		if err == nil {
			t.Fatal("expected error for negative ID, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// GetPolicies (pagination + filtering)
// ---------------------------------------------------------------------------

func TestPermissionService_GetPolicies(t *testing.T) {
	svc, _ := newTestPermissionService(t)
	ctx := context.Background()

	t.Run("returns seeded policies", func(t *testing.T) {
		policies, total, err := svc.GetPolicies(ctx, 1, 50, "", "")
		if err != nil {
			t.Fatalf("GetPolicies() error: %v", err)
		}
		if total != 8 {
			t.Errorf("total = %d, want 8", total)
		}
		if len(policies) != 8 {
			t.Errorf("len(policies) = %d, want 8", len(policies))
		}
	})

	t.Run("filter by sub=admin", func(t *testing.T) {
		policies, total, err := svc.GetPolicies(ctx, 1, 50, "", "admin")
		if err != nil {
			t.Fatalf("GetPolicies() error: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(policies) != 1 {
			t.Errorf("len(policies) = %d, want 1", len(policies))
		}
		if policies[0].Sub != "admin" {
			t.Errorf("Sub = %q, want %q", policies[0].Sub, "admin")
		}
	})

	t.Run("filter by sub=dba", func(t *testing.T) {
		policies, total, err := svc.GetPolicies(ctx, 1, 50, "", "dba")
		if err != nil {
			t.Fatalf("GetPolicies() error: %v", err)
		}
		if total != 6 {
			t.Errorf("total = %d, want 6", total)
		}
		for _, p := range policies {
			if p.Sub != "dba" {
				t.Errorf("Sub = %q, want %q", p.Sub, "dba")
			}
		}
	})

	t.Run("filter by sub=developer", func(t *testing.T) {
		_, total, err := svc.GetPolicies(ctx, 1, 50, "", "developer")
		if err != nil {
			t.Fatalf("GetPolicies() error: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
	})

	t.Run("filter by nonexistent role", func(t *testing.T) {
		policies, total, err := svc.GetPolicies(ctx, 1, 50, "", "nonexistent")
		if err != nil {
			t.Fatalf("GetPolicies() error: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0", total)
		}
		if len(policies) != 0 {
			t.Errorf("len(policies) = %d, want 0", len(policies))
		}
	})

	t.Run("pagination page 1", func(t *testing.T) {
		policies, total, err := svc.GetPolicies(ctx, 1, 3, "", "")
		if err != nil {
			t.Fatalf("GetPolicies() error: %v", err)
		}
		if len(policies) > 3 {
			t.Errorf("len(policies) = %d, want <= 3", len(policies))
		}
		if total != 8 {
			t.Errorf("total = %d, want 8", total)
		}
	})

	t.Run("pagination page 2", func(t *testing.T) {
		policies, total, err := svc.GetPolicies(ctx, 2, 3, "", "")
		if err != nil {
			t.Fatalf("GetPolicies() error: %v", err)
		}
		if total != 8 {
			t.Errorf("total = %d, want 8", total)
		}
		if len(policies) > 3 {
			t.Errorf("len(policies) = %d, want <= 3", len(policies))
		}
	})

	t.Run("pagination page beyond data", func(t *testing.T) {
		policies, total, err := svc.GetPolicies(ctx, 100, 10, "", "")
		if err != nil {
			t.Fatalf("GetPolicies() error: %v", err)
		}
		if total != 8 {
			t.Errorf("total = %d, want 8", total)
		}
		if len(policies) != 0 {
			t.Errorf("len(policies) = %d, want 0", len(policies))
		}
	})

	t.Run("default pagination values", func(t *testing.T) {
		policies, total, err := svc.GetPolicies(ctx, 0, 0, "", "")
		if err != nil {
			t.Fatalf("GetPolicies() error: %v", err)
		}
		if total != 8 {
			t.Errorf("total = %d, want 8", total)
		}
		if len(policies) != 8 {
			t.Errorf("len(policies) = %d, want 8", len(policies))
		}
	})
}

// ---------------------------------------------------------------------------
// GetPoliciesForRole
// ---------------------------------------------------------------------------

func TestPermissionService_GetPoliciesForRole(t *testing.T) {
	svc, _ := newTestPermissionService(t)
	ctx := context.Background()

	t.Run("admin role", func(t *testing.T) {
		policies, err := svc.GetPoliciesForRole(ctx, "admin")
		if err != nil {
			t.Fatalf("GetPoliciesForRole() error: %v", err)
		}
		if len(policies) != 1 {
			t.Fatalf("len(policies) = %d, want 1", len(policies))
		}
		p := policies[0]
		if p.Sub != "admin" {
			t.Errorf("Sub = %q, want %q", p.Sub, "admin")
		}
		if p.Dom != "*" {
			t.Errorf("Dom = %q, want %q", p.Dom, "*")
		}
		if p.Obj != "*" {
			t.Errorf("Obj = %q, want %q", p.Obj, "*")
		}
		if p.Act != "*" {
			t.Errorf("Act = %q, want %q", p.Act, "*")
		}
	})

	t.Run("dba role", func(t *testing.T) {
		policies, err := svc.GetPoliciesForRole(ctx, "dba")
		if err != nil {
			t.Fatalf("GetPoliciesForRole() error: %v", err)
		}
		if len(policies) != 6 {
			t.Fatalf("len(policies) = %d, want 6", len(policies))
		}
		acts := make([]string, 0, len(policies))
		for _, p := range policies {
			if p.Sub != "dba" {
				t.Errorf("Sub = %q, want %q", p.Sub, "dba")
			}
			acts = append(acts, p.Act)
		}
		sort.Strings(acts)
		expected := []string{"ddl", "delete", "desensitize:bypass", "export", "select", "update"}
		sort.Strings(expected)
		for i, act := range acts {
			if act != expected[i] {
				t.Errorf("acts[%d] = %q, want %q", i, act, expected[i])
			}
		}
	})

	t.Run("developer role", func(t *testing.T) {
		policies, err := svc.GetPoliciesForRole(ctx, "developer")
		if err != nil {
			t.Fatalf("GetPoliciesForRole() error: %v", err)
		}
		if len(policies) != 1 {
			t.Fatalf("len(policies) = %d, want 1", len(policies))
		}
		if policies[0].Act != "select" {
			t.Errorf("Act = %q, want %q", policies[0].Act, "select")
		}
	})

	t.Run("nonexistent role returns empty", func(t *testing.T) {
		policies, err := svc.GetPoliciesForRole(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("GetPoliciesForRole() error: %v", err)
		}
		if len(policies) != 0 {
			t.Errorf("len(policies) = %d, want 0", len(policies))
		}
	})

	t.Run("empty role returns empty", func(t *testing.T) {
		policies, err := svc.GetPoliciesForRole(ctx, "")
		if err != nil {
			t.Fatalf("GetPoliciesForRole() error: %v", err)
		}
		if len(policies) != 0 {
			t.Errorf("len(policies) = %d, want 0", len(policies))
		}
	})
}

// ---------------------------------------------------------------------------
// GetRoles
// ---------------------------------------------------------------------------

func TestPermissionService_GetRoles(t *testing.T) {
	svc, _ := newTestPermissionService(t)

	roles := svc.GetRoles()
	expected := []string{"admin", "dba", "developer"}
	if len(roles) != len(expected) {
		t.Fatalf("len(roles) = %d, want %d", len(roles), len(expected))
	}
	for i, r := range roles {
		if r != expected[i] {
			t.Errorf("roles[%d] = %q, want %q", i, r, expected[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Enforcer
// ---------------------------------------------------------------------------

func TestPermissionService_Enforcer(t *testing.T) {
	svc, _ := newTestPermissionService(t)

	enf := svc.Enforcer()
	if enf == nil {
		t.Fatal("Enforcer() returned nil")
	}
}

// ---------------------------------------------------------------------------
// LoadPolicy / SavePolicy
// ---------------------------------------------------------------------------

func TestPermissionService_LoadSavePolicy(t *testing.T) {
	svc, _ := newTestPermissionService(t)

	t.Run("LoadPolicy succeeds", func(t *testing.T) {
		if err := svc.LoadPolicy(); err != nil {
			t.Fatalf("LoadPolicy() error: %v", err)
		}
	})

	t.Run("SavePolicy succeeds", func(t *testing.T) {
		if err := svc.SavePolicy(); err != nil {
			t.Fatalf("SavePolicy() error: %v", err)
		}
	})

	t.Run("policies persist after save and reload", func(t *testing.T) {
		err := svc.AddPolicy("developer", "*", "users", "insert")
		if err != nil {
			t.Fatalf("AddPolicy() error: %v", err)
		}

		if err := svc.SavePolicy(); err != nil {
			t.Fatalf("SavePolicy() error: %v", err)
		}

		if err := svc.LoadPolicy(); err != nil {
			t.Fatalf("LoadPolicy() error: %v", err)
		}

		ok, err := svc.Enforce("developer", "*", "users", "insert")
		if err != nil {
			t.Fatalf("Enforce() error: %v", err)
		}
		if !ok {
			t.Error("Enforce() should return true for policy added before save/reload")
		}
	})
}

// ---------------------------------------------------------------------------
// Full CRUD lifecycle
// ---------------------------------------------------------------------------

func TestPermissionService_CRUDLifecycle(t *testing.T) {
	svc, testDB := newTestPermissionService(t)
	ctx := context.Background()

	// 1. Verify initial state — developer cannot update
	ok, err := svc.Enforce("developer", "*", "employees", "update")
	if err != nil {
		t.Fatalf("Enforce() initial check error: %v", err)
	}
	if ok {
		t.Fatal("developer should NOT be able to update initially")
	}

	// 2. Add policy granting update to developer on hr.employees
	err = svc.AddPolicy("developer", "*", "employees", "update")
	if err != nil {
		t.Fatalf("AddPolicy() error: %v", err)
	}

	// 3. Verify enforcement now passes
	ok, err = svc.Enforce("developer", "*", "employees", "update")
	if err != nil {
		t.Fatalf("Enforce() after add error: %v", err)
	}
	if !ok {
		t.Fatal("developer should be able to update after AddPolicy")
	}

	// 4. Verify GetPoliciesForRole includes the new policy
	policies, err := svc.GetPoliciesForRole(ctx, "developer")
	if err != nil {
		t.Fatalf("GetPoliciesForRole() error: %v", err)
	}
	found := false
	for _, p := range policies {
		if p.Obj == "employees" && p.Act == "update" {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetPoliciesForRole() should include the added policy")
	}

	// 5. Get the ID and remove the policy
	var ruleID int64
	err = testDB.QueryRowContext(ctx,
		`SELECT id FROM casbin_rule WHERE ptype='p' AND v0='developer' AND v1='*' AND v2='employees' AND v3='update'`,
	).Scan(&ruleID)
	if err != nil {
		t.Fatalf("find rule ID: %v", err)
	}

	err = svc.RemovePolicy(ctx, ruleID)
	if err != nil {
		t.Fatalf("RemovePolicy() error: %v", err)
	}

	// 6. Verify enforcement now fails again
	ok, err = svc.Enforce("developer", "*", "employees", "update")
	if err != nil {
		t.Fatalf("Enforce() after remove error: %v", err)
	}
	if ok {
		t.Fatal("developer should NOT be able to update after RemovePolicy")
	}
}

// ---------------------------------------------------------------------------
// Policy struct field mapping
// ---------------------------------------------------------------------------

func TestPermissionService_PolicyFields(t *testing.T) {
	svc, _ := newTestPermissionService(t)
	ctx := context.Background()

	policies, err := svc.GetPoliciesForRole(ctx, "admin")
	if err != nil {
		t.Fatalf("GetPoliciesForRole() error: %v", err)
	}
	if len(policies) == 0 {
		t.Fatal("expected at least one admin policy")
	}

	p := policies[0]
	t.Run("ID is positive", func(t *testing.T) {
		if p.ID <= 0 {
			t.Errorf("ID = %d, want > 0", p.ID)
		}
	})
	t.Run("PType is p", func(t *testing.T) {
		if p.PType != "p" {
			t.Errorf("PType = %q, want %q", p.PType, "p")
		}
	})
	t.Run("Sub is admin", func(t *testing.T) {
		if p.Sub != "admin" {
			t.Errorf("Sub = %q, want %q", p.Sub, "admin")
		}
	})
	t.Run("Dom is *", func(t *testing.T) {
		if p.Dom != "*" {
			t.Errorf("Dom = %q, want %q", p.Dom, "*")
		}
	})
	t.Run("Obj is *", func(t *testing.T) {
		if p.Obj != "*" {
			t.Errorf("Obj = %q, want %q", p.Obj, "*")
		}
	})
	t.Run("Act is *", func(t *testing.T) {
		if p.Act != "*" {
			t.Errorf("Act = %q, want %q", p.Act, "*")
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestToInterfaceSlice(t *testing.T) {
	t.Run("non-empty slice", func(t *testing.T) {
		input := []string{"a", "b", "c"}
		result := toInterfaceSlice(input)
		if len(result) != 3 {
			t.Fatalf("len(result) = %d, want 3", len(result))
		}
		for i, v := range result {
			if v.(string) != input[i] {
				t.Errorf("result[%d] = %v, want %v", i, v, input[i])
			}
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		result := toInterfaceSlice([]string{})
		if len(result) != 0 {
			t.Errorf("len(result) = %d, want 0", len(result))
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		result := toInterfaceSlice(nil)
		if len(result) != 0 {
			t.Errorf("len(result) = %d, want 0", len(result))
		}
	})
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"single line", "hello", 1},
		{"multi line", "a\nb\nc", 3},
		{"trailing newline", "a\nb\n", 3},
		{"empty string", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := splitLines(tt.input)
			if len(lines) != tt.want {
				t.Errorf("len(splitLines()) = %d, want %d", len(lines), tt.want)
			}
		})
	}
}

func TestGetArg(t *testing.T) {
	tests := []struct {
		name string
		rule []string
		idx  int
		want string
	}{
		{"in bounds", []string{"a", "b", "c"}, 0, "a"},
		{"last element", []string{"a", "b", "c"}, 2, "c"},
		{"out of bounds high", []string{"a"}, 5, ""},
		{"nil rule", nil, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getArg(tt.rule, tt.idx)
			if got != tt.want {
				t.Errorf("getArg() = %q, want %q", got, tt.want)
			}
		})
	}
}
