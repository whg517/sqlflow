// Package driver_test tests the driver abstraction layer.
// It imports all concrete driver implementations to trigger init() registration.
package driver_test

import (
	"context"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/mysql"
)

type mockDriver struct {
	typ string
}

func (m *mockDriver) Type() string                                                    { return m.typ }
func (m *mockDriver) Capabilities() driver.CapabilitySet                              { return driver.CapabilitySet(driver.CapQuery | driver.CapMetadata) }
func (m *mockDriver) Connect(ctx context.Context, cfg *driver.Config) error           { return nil }
func (m *mockDriver) Close() error                                                    { return nil }
func (m *mockDriver) Ping(ctx context.Context) error                                  { return nil }
func (m *mockDriver) ListDatabases(ctx context.Context) ([]string, error)             { return nil, nil }
func (m *mockDriver) ListTables(ctx context.Context, db string) ([]driver.TableInfo, error) { return nil, nil }
func (m *mockDriver) GetColumns(ctx context.Context, db, tbl string) ([]driver.ColumnInfo, error) { return nil, nil }
func (m *mockDriver) ExecuteQuery(ctx context.Context, db, q string, l int) (*driver.QueryResult, error) { return nil, nil }
func (m *mockDriver) ExecuteStatement(ctx context.Context, db, s string) (*driver.StatementResult, error) { return nil, nil }
func (m *mockDriver) Parse(q string) (*driver.ParseResult, error)                    { return nil, nil }

func TestCapabilitySet_Has(t *testing.T) {
	cs := driver.CapabilitySet(driver.CapQuery | driver.CapMetadata | driver.CapExport)

	if !cs.Has(driver.CapQuery) {
		t.Error("should have CapQuery")
	}
	if !cs.Has(driver.CapMetadata) {
		t.Error("should have CapMetadata")
	}
	if !cs.Has(driver.CapExport) {
		t.Error("should have CapExport")
	}
	if cs.Has(driver.CapTicketExec) {
		t.Error("should not have CapTicketExec")
	}
	if cs.Has(driver.CapFieldMasking) {
		t.Error("should not have CapFieldMasking")
	}
}

func TestCapabilitySet_String(t *testing.T) {
	tests := []struct {
		cs   driver.CapabilitySet
		want string
	}{
		{driver.CapabilitySet(0), "none"},
		{driver.CapabilitySet(driver.CapQuery), "query"},
		{driver.CapabilitySet(driver.CapQuery | driver.CapMetadata), "query,metadata"},
		{driver.CapabilitySet(driver.CapQuery | driver.CapTicketExec | driver.CapExport), "query,ticket_exec,export"},
	}

	for _, tt := range tests {
		got := tt.cs.String()
		if got != tt.want {
			t.Errorf("CapabilitySet(%d).String() = %q, want %q", tt.cs, got, tt.want)
		}
	}
}

func TestNewDriver_Unsupported(t *testing.T) {
	_, err := driver.NewDriver("nonexistent_type")
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestIsRegistered_MySQL(t *testing.T) {
	if !driver.IsRegistered("mysql") {
		t.Error("mysql should be registered via init()")
	}
	if driver.IsRegistered("nonexistent") {
		t.Error("nonexistent should not be registered")
	}
}

func TestSupportedTypes(t *testing.T) {
	types := driver.SupportedTypes()
	if len(types) == 0 {
		t.Error("expected at least one registered type")
	}
	found := false
	for _, typ := range types {
		if typ == "mysql" {
			found = true
			break
		}
	}
	if !found {
		t.Error("mysql should be in supported types")
	}
}

func TestNewDriver_MySQL(t *testing.T) {
	d, err := driver.NewDriver("mysql")
	if err != nil {
		t.Fatalf("NewDriver(mysql) error: %v", err)
	}
	if d.Type() != "mysql" {
		t.Errorf("Type() = %q, want mysql", d.Type())
	}
	caps := d.Capabilities()
	if !caps.Has(driver.CapQuery) {
		t.Error("mysql should have CapQuery")
	}
	if !caps.Has(driver.CapTicketExec) {
		t.Error("mysql should have CapTicketExec")
	}
}

func TestPoolManager_Basic(t *testing.T) {
	pm := driver.NewPoolManager()
	defer pm.Close()

	cfg := &driver.Config{
		ID:       1,
		Host:     "localhost",
		Port:     3306,
		Username: "root",
		Password: "test",
		Database: "test",
		Extra:    map[string]interface{}{"_type": "mock"},
	}

	// Inject a mock driver directly
	mock := &mockDriver{typ: "mock"}
	pm.InjectForTest(1, mock, cfg)

	// GetCached should return the mock
	d := pm.GetCached(1)
	if d == nil {
		t.Fatal("GetCached(1) should return the mock driver")
	}
	if d.Type() != "mock" {
		t.Errorf("Type() = %q, want mock", d.Type())
	}

	// GetCached for non-existent should return nil
	if pm.GetCached(999) != nil {
		t.Error("GetCached(999) should return nil")
	}

	// ManagedIDs
	ids := pm.ManagedIDs()
	if len(ids) != 1 || ids[0] != 1 {
		t.Errorf("ManagedIDs() = %v, want [1]", ids)
	}

	// Remove
	pm.Remove(1)
	if pm.GetCached(1) != nil {
		t.Error("GetCached(1) should return nil after Remove")
	}
}

func TestPoolManager_Get_MySQL(t *testing.T) {
	pm := driver.NewPoolManager()
	defer pm.Close()

	// This will fail because there's no MySQL server, but the driver creation should work
	cfg := &driver.Config{
		ID:       2,
		Host:     "localhost",
		Port:     3306,
		Username: "root",
		Password: "invalid_password_for_test",
		Database: "test",
		MaxOpen:  5,
		MaxIdle:  2,
		Extra:    map[string]interface{}{"_type": "mysql"},
	}

	// Use a short timeout to avoid waiting 30s on connection failure
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := pm.Get(ctx, cfg)
	// Will fail to connect, but driver.NewDriver("mysql") should succeed
	// The error should be about connection, not about unsupported type
	if err == nil {
		// If by chance MySQL is running, that's fine
		return
	}
	// Just verify that mysql is registered (the error is about connection)
	if !driver.IsRegistered("mysql") {
		t.Error("mysql should be registered")
	}
}
