package postgresql

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

func TestPostgreSQLDriver_Type(t *testing.T) {
	d := &PostgreSQLDriver{}
	if got := d.Type(); got != "postgresql" {
		t.Errorf("Type() = %q, want %q", got, "postgresql")
	}
}

func TestPostgreSQLDriver_Capabilities(t *testing.T) {
	d := &PostgreSQLDriver{}
	caps := d.Capabilities()

	required := []driver.Capability{
		driver.CapQuery, driver.CapTicketExec, driver.CapMetadata,
		driver.CapTableLevelPermission, driver.CapFieldMasking,
		driver.CapSQLParse, driver.CapExport,
	}

	for _, cap := range required {
		if !caps.Has(cap) {
			t.Errorf("Capabilities() missing %d", cap)
		}
	}
}

func TestPostgreSQLDriver_Parse(t *testing.T) {
	d := &PostgreSQLDriver{}

	tests := []struct {
		name         string
		query        string
		wantOp       string
		wantBlocked  bool
		wantRiskHigh bool
	}{
		{name: "simple select", query: "SELECT * FROM users", wantOp: "select", wantBlocked: false},
		{name: "select with where", query: "SELECT id, name FROM orders WHERE status = 'active'", wantOp: "select"},
		{name: "insert", query: "INSERT INTO users (name, email) VALUES ('test', 't@t.com')", wantOp: "dml"},
		{name: "update with where", query: "UPDATE users SET name = 'new' WHERE id = 1", wantOp: "update"},
		{name: "delete with where", query: "DELETE FROM logs WHERE created_at < '2024-01-01'", wantOp: "delete"},
		{name: "drop table blocked", query: "DROP TABLE users", wantOp: "ddl", wantBlocked: true, wantRiskHigh: true},
		{name: "create table", query: "CREATE TABLE foo (id INT)", wantOp: "ddl"},
		{name: "update no where blocked", query: "UPDATE users SET name = 'x'", wantOp: "update", wantBlocked: true, wantRiskHigh: true},
		{name: "delete no where blocked", query: "DELETE FROM users", wantOp: "delete", wantBlocked: true, wantRiskHigh: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := d.Parse(tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
			if result.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v", result.IsBlocked, tt.wantBlocked)
			}
			if tt.wantRiskHigh && result.RiskLevel != "high" {
				t.Errorf("RiskLevel = %q, want %q", result.RiskLevel, "high")
			}
		})
	}
}

func TestPostgreSQLDriver_NotConnected(t *testing.T) {
	d := &PostgreSQLDriver{}

	// All operations that require a connection should return "not connected" error.
	err := d.Ping(nil)
	if err == nil {
		t.Error("Ping() should fail when not connected")
	}

	_, err = d.ListDatabases(nil)
	if err == nil {
		t.Error("ListDatabases() should fail when not connected")
	}

	_, err = d.ListTables(nil, "public")
	if err == nil {
		t.Error("ListTables() should fail when not connected")
	}

	_, err = d.GetColumns(nil, "public", "users")
	if err == nil {
		t.Error("GetColumns() should fail when not connected")
	}

	_, err = d.ExecuteQuery(nil, "public", "SELECT 1", 10)
	if err == nil {
		t.Error("ExecuteQuery() should fail when not connected")
	}

	_, err = d.ExecuteStatement(nil, "public", "SELECT 1")
	if err == nil {
		t.Error("ExecuteStatement() should fail when not connected")
	}
}

func TestPostgreSQLDriver_Registry(t *testing.T) {
	if !driver.IsRegistered("postgresql") {
		t.Error("postgresql should be registered via init()")
	}

	d, err := driver.NewDriver("postgresql")
	if err != nil {
		t.Fatalf("NewDriver(postgresql) error: %v", err)
	}

	if d.Type() != "postgresql" {
		t.Errorf("NewDriver(postgresql).Type() = %q, want %q", d.Type(), "postgresql")
	}
}
