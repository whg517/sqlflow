package mysql

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

func TestMySQLDriver_Type(t *testing.T) {
	d := &MySQLDriver{}
	if d.Type() != "mysql" {
		t.Errorf("Type() = %q, want %q", d.Type(), "mysql")
	}
}

func TestMySQLDriver_Capabilities(t *testing.T) {
	d := &MySQLDriver{}
	caps := d.Capabilities()

	expected := []driver.Capability{
		driver.CapQuery,
		driver.CapTicketExec,
		driver.CapMetadata,
		driver.CapTableLevelPermission,
		driver.CapFieldMasking,
		driver.CapSQLParse,
		driver.CapExport,
	}

	for _, cap := range expected {
		if !caps.Has(cap) {
			t.Errorf("Capabilities().Has(%v) = false, want true", cap)
		}
	}
}

func TestMySQLDriver_Parse(t *testing.T) {
	d := &MySQLDriver{}

	tests := []struct {
		name      string
		query     string
		wantOp    string
		wantRisk  string
		wantBlock bool
	}{
		{
			name:     "simple select",
			query:    "SELECT * FROM users",
			wantOp:   "select",
			wantRisk: "low",
		},
		{
			name:     "select with where",
			query:    "SELECT id, name FROM users WHERE id = 1",
			wantOp:   "select",
			wantRisk: "low",
		},
		{
			name:     "insert",
			query:    "INSERT INTO users (name) VALUES ('test')",
			wantOp:   "dml",
			wantRisk: "medium",
		},
		{
			name:     "update",
			query:    "UPDATE users SET name = 'test' WHERE id = 1",
			wantOp:   "update",
			wantRisk: "medium",
		},
		{
			name:     "delete",
			query:    "DELETE FROM users WHERE id = 1",
			wantOp:   "delete",
			wantRisk: "medium",
		},
		{
			name:      "drop table blocked",
			query:     "DROP TABLE users",
			wantOp:    "ddl",
			wantRisk:  "high",
			wantBlock: true,
		},
		{
			name:     "create table",
			query:    "CREATE TABLE test (id INT PRIMARY KEY)",
			wantOp:   "ddl",
			wantRisk: "medium", // parser gives DDL→medium for non-DROP/TRUNCATE
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := d.Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.query, err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
			if result.RiskLevel != tt.wantRisk {
				t.Errorf("RiskLevel = %q, want %q", result.RiskLevel, tt.wantRisk)
			}
			if result.IsBlocked != tt.wantBlock {
				t.Errorf("IsBlocked = %v, want %v", result.IsBlocked, tt.wantBlock)
			}
		})
	}
}

func TestMySQLDriver_NotConnected(t *testing.T) {
	d := &MySQLDriver{}

	// All methods should return error when not connected
	if err := d.Ping(nil); err == nil {
		t.Error("Ping should fail when not connected")
	}

	if _, err := d.ListDatabases(nil); err == nil {
		t.Error("ListDatabases should fail when not connected")
	}

	if _, err := d.ListTables(nil, "test"); err == nil {
		t.Error("ListTables should fail when not connected")
	}

	if _, err := d.GetColumns(nil, "test", "users"); err == nil {
		t.Error("GetColumns should fail when not connected")
	}

	if _, err := d.ExecuteQuery(nil, "test", "SELECT 1", 10); err == nil {
		t.Error("ExecuteQuery should fail when not connected")
	}

	if _, err := d.ExecuteStatement(nil, "test", "INSERT INTO t VALUES (1)"); err == nil {
		t.Error("ExecuteStatement should fail when not connected")
	}
}
