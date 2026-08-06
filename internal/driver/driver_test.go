// Package driver_test tests the driver abstraction layer.
// It imports all concrete driver implementations to trigger init() registration.
package driver_test

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/elasticsearch"
	_ "github.com/whg517/sqlflow/internal/driver/mongodb"
	_ "github.com/whg517/sqlflow/internal/driver/mysql"
	_ "github.com/whg517/sqlflow/internal/driver/postgresql"
)

type mockDriver struct {
	typ string
}

func (m *mockDriver) Type() string                                          { return m.typ }
func (m *mockDriver) QueryForm() driver.QueryForm                           { return driver.QueryFormSQL }
func (m *mockDriver) Connect(ctx context.Context, cfg *driver.Config) error { return nil }
func (m *mockDriver) Close() error                                          { return nil }
func (m *mockDriver) Ping(ctx context.Context) error                        { return nil }
func (m *mockDriver) ListDatabases(ctx context.Context) ([]string, error)   { return nil, nil }
func (m *mockDriver) ListTables(ctx context.Context, db string) ([]driver.TableInfo, error) {
	return nil, nil
}
func (m *mockDriver) GetColumns(ctx context.Context, db, tbl string) ([]driver.ColumnInfo, error) {
	return nil, nil
}
func (m *mockDriver) ExecuteQuery(ctx context.Context, db, q string, l int) (*driver.QueryResult, error) {
	return nil, nil
}
func (m *mockDriver) ExecuteStatement(ctx context.Context, db, s string) (*driver.StatementResult, error) {
	return nil, nil
}
func (m *mockDriver) ExecuteStatements(ctx context.Context, db string, s []string) ([]driver.StatementResult, error) {
	return nil, nil
}
func (m *mockDriver) Parse(q string) (*driver.ParseResult, error) { return nil, nil }

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
	if _, ok := d.(driver.StatementExecutor); !ok {
		t.Error("mysql should satisfy StatementExecutor")
	}
}

func TestPoolManager_Basic(t *testing.T) {
	pm := driver.NewPoolManager()
	defer pm.Close()

	// Inject a mock driver directly
	mock := &mockDriver{typ: "mock"}
	pm.InjectForTest(1, mock)

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

// TestPoolManagerGetCachesTheConnection checks Get's actual contract: connect
// once, then hand the same driver back.
//
// It replaces a test that called Get against a MySQL that was not running,
// returned early if the call happened to succeed, and otherwise asserted only
// that "mysql" was in the registry — so it passed on every outcome and verified
// nothing about Get at all.
func TestPoolManagerGetCachesTheConnection(t *testing.T) {
	poolTestConnects.Store(0)
	poolTestCloses.Store(0)

	pm := driver.NewPoolManager()
	defer pm.Close()
	cfg := countingConfig(6)

	first, err := pm.Get(t.Context(), cfg)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	second, err := pm.Get(t.Context(), cfg)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}

	if first != second {
		t.Error("the second Get returned a different driver — the connection was not cached")
	}
	if got := poolTestConnects.Load(); got != 1 {
		t.Errorf("Connect called %d times for two Gets, want 1", got)
	}

	pm.Remove(6)
	if got := poolTestCloses.Load(); got != 1 {
		t.Errorf("Close called %d times after Remove, want 1", got)
	}
	if pm.GetCached(6) != nil {
		t.Error("the entry survived Remove")
	}
}

// TestPoolManagerGetReportsUnregisteredType keeps the registry error distinct
// from a connection failure — the two need different messages to the operator.
func TestPoolManagerGetReportsUnregisteredType(t *testing.T) {
	pm := driver.NewPoolManager()
	defer pm.Close()

	_, err := pm.Get(t.Context(), &driver.Config{ID: 7, Extra: map[string]interface{}{"_type": "oracle"}})
	if err == nil {
		t.Fatal("an unregistered driver type was accepted")
	}
}
