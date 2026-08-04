package security

import (
	"errors"
	"testing"

	"github.com/whg517/sqlflow/internal/testutil"
)

func TestRoleLifecycleAndGuards(t *testing.T) {
	database := testutil.NewDB(t)

	svc, err := NewService(database)
	if err != nil {
		t.Fatal(err)
	}

	roles, err := svc.ListRoles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 3 {
		t.Fatalf("built-in role count = %d, want 3", len(roles))
	}

	role, err := svc.CreateRole(t.Context(), "data_analyst", "数据分析师", "只读分析")
	if err != nil {
		t.Fatal(err)
	}
	if role.IsBuiltin || role.Status != "active" {
		t.Fatalf("unexpected created role: %#v", role)
	}
	if _, err := svc.CreateRole(t.Context(), "data_analyst", "重复", ""); !errors.Is(err, ErrRoleExists) {
		t.Fatalf("duplicate role error = %v", err)
	}
	if err := svc.AddPolicy("data_analyst", "ds_1", "orders", "select"); err != nil {
		t.Fatal(err)
	}
	role, err = svc.GetRole(t.Context(), "data_analyst")
	if err != nil {
		t.Fatal(err)
	}
	if role.PolicyCount != 1 {
		t.Fatalf("policy count = %d, want 1", role.PolicyCount)
	}

	role, err = svc.UpdateRole(t.Context(), "data_analyst", "分析师", "updated", "disabled")
	if err != nil {
		t.Fatal(err)
	}
	if role.Status != "disabled" {
		t.Fatalf("status = %s", role.Status)
	}
	if _, err := svc.UpdateRole(t.Context(), "data_analyst", "分析师", "updated", "active"); err != nil {
		t.Fatal(err)
	}

	if _, err := database.Exec(
		`INSERT INTO users (username, password_hash, role) VALUES ('analyst1', 'hash', 'data_analyst')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateRole(t.Context(), "data_analyst", "分析师", "", "disabled"); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("disable in-use role error = %v", err)
	}
	if err := svc.DeleteRole(t.Context(), "data_analyst"); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("delete in-use role error = %v", err)
	}
	if _, err := database.Exec(`UPDATE users SET role = 'developer' WHERE username = 'analyst1'`); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteRole(t.Context(), "data_analyst"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetRole(t.Context(), "data_analyst"); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("deleted role error = %v", err)
	}
	if err := svc.DeleteRole(t.Context(), "admin"); !errors.Is(err, ErrBuiltinRole) {
		t.Fatalf("delete built-in role error = %v", err)
	}
}
