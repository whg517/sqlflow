package service

import (
	"context"
	"errors"
	"testing"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/mysql"
	_ "github.com/whg517/sqlflow/internal/driver/postgresql"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
)

// testExecDriver 是用于测试 ticket_executor 新路径的 mock driver。
// 它记录 ExecuteStatements 的调用，返回预设结果。
type testExecDriver struct {
	typ          string
	stmts        []string // 记录收到的语句
	mockResults  []driver.StatementResult
	mockErr      error
	connected    bool
}

func (m *testExecDriver) Type() string                     { return m.typ }
func (m *testExecDriver) Capabilities() driver.CapabilitySet { return driver.CapabilitySet(driver.CapTicketExec) }
func (m *testExecDriver) Connect(ctx context.Context, cfg *driver.Config) error { m.connected = true; return nil }
func (m *testExecDriver) Close() error                     { m.connected = false; return nil }
func (m *testExecDriver) Ping(ctx context.Context) error   { return nil }
func (m *testExecDriver) ListDatabases(ctx context.Context) ([]string, error) { return nil, nil }
func (m *testExecDriver) ListTables(ctx context.Context, db string) ([]driver.TableInfo, error) { return nil, nil }
func (m *testExecDriver) GetColumns(ctx context.Context, db, tbl string) ([]driver.ColumnInfo, error) { return nil, nil }
func (m *testExecDriver) ExecuteQuery(ctx context.Context, db, q string, l int) (*driver.QueryResult, error) { return nil, nil }
func (m *testExecDriver) ExecuteStatement(ctx context.Context, db, stmt string) (*driver.StatementResult, error) {
	return nil, nil
}
func (m *testExecDriver) ExecuteStatements(ctx context.Context, db string, statements []string) ([]driver.StatementResult, error) {
	m.stmts = statements
	if m.mockErr != nil {
		// 模拟 PG 事务：首错时返回已执行结果（含 rolled_back 标记）
		return m.mockResults, m.mockErr
	}
	return m.mockResults, nil
}
func (m *testExecDriver) Parse(q string) (*driver.ParseResult, error) { return nil, nil }

// TestExecuteSQL_DriverPath_MySQL 验证 ticket_executor 在 poolMgr 可用时走 driver 层路径（MySQL）。
func TestExecuteSQL_DriverPath_MySQL(t *testing.T) {
	d := setupTxTestDB(t)
	encKey := "test-encryption-key-32byte-len!!"
	connMgr := connpool.NewManager()
	t.Cleanup(func() { connMgr.Close() })
	poolMgr := driver.NewPoolManager()
	t.Cleanup(func() { poolMgr.Close() })

	dsSvc := NewDatasourceService(d, encKey, connMgr, poolMgr)
	svc := &TicketService{
		database:      d,
		client:        d.Client(),
		dsSvc:         dsSvc,
		connMgr:       connMgr,
		poolMgr:       poolMgr,
		encryptionKey: encKey,
	}

	// 注入 mock driver
	mockDrv := &testExecDriver{
		typ: "mysql",
		mockResults: []driver.StatementResult{
			{Statement: "CREATE TABLE t1 (id INT)", Status: "success", RowsAffected: 0, DurationMs: 5},
			{Statement: "INSERT INTO t1 VALUES (1)", Status: "success", RowsAffected: 1, DurationMs: 3},
		},
	}
	cfg := &driver.Config{ID: 100, Host: "localhost", Port: 3306, Database: "testdb", Extra: map[string]interface{}{"_type": "mysql"}}
	poolMgr.InjectForTest(100, mockDrv, cfg)

	// 创建 MySQL 数据源
	encPass, _ := crypto.Encrypt("test", encKey)
	_, err := d.DB.ExecContext(context.Background(),
		`INSERT INTO datasources (id, name, type, host, port, username, password_encrypted, database) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		100, "mysql-test", "mysql", "localhost", 3306, "root", encPass, "testdb")
	if err != nil {
		t.Fatalf("insert datasource: %v", err)
	}

	ds, err := dsSvc.GetDataSource(context.Background(), 100)
	if err != nil {
		t.Fatalf("get datasource: %v", err)
	}

	results, err := svc.executeSQL(context.Background(), ds, "testdb", "mysql", "CREATE TABLE t1 (id INT); INSERT INTO t1 VALUES (1)")
	if err != nil {
		t.Fatalf("executeSQL via driver path: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != "success" || results[0].SQL != "CREATE TABLE t1 (id INT)" {
		t.Errorf("result[0] = %+v, want success/CREATE", results[0])
	}
	if results[1].RowsAffected != 1 {
		t.Errorf("result[1].RowsAffected = %d, want 1", results[1].RowsAffected)
	}

	// 验证 driver 收到了分割后的语句
	if len(mockDrv.stmts) != 2 {
		t.Errorf("driver received %d statements, want 2", len(mockDrv.stmts))
	}
}

// TestExecuteSQL_DriverPath_PostgreSQLRollback 验证 driver 路径下 PG 事务回滚语义：
// 首错时 driver 返回 error + 已执行结果（标记 rolled_back），executeSQL 正确传递。
func TestExecuteSQL_DriverPath_PostgreSQLRollback(t *testing.T) {
	d := setupTxTestDB(t)
	encKey := "test-encryption-key-32byte-len!!"
	connMgr := connpool.NewManager()
	t.Cleanup(func() { connMgr.Close() })
	poolMgr := driver.NewPoolManager()
	t.Cleanup(func() { poolMgr.Close() })

	dsSvc := NewDatasourceService(d, encKey, connMgr, poolMgr)
	svc := &TicketService{
		database:      d,
		client:        d.Client(),
		dsSvc:         dsSvc,
		connMgr:       connMgr,
		poolMgr:       poolMgr,
		encryptionKey: encKey,
	}

	// mock driver 模拟 PG 事务：第2条语句失败，第1条标记 rolled_back
	mockDrv := &testExecDriver{
		typ: "postgresql",
		mockResults: []driver.StatementResult{
			{Statement: "CREATE TABLE t1 (id INT)", Status: "rolled_back", DurationMs: 5},
			{Statement: "INVALID SQL", Status: "error", Error: "syntax error"},
		},
		mockErr: errors.New("statement failed: syntax error"),
	}
	cfg := &driver.Config{ID: 200, Host: "localhost", Port: 5432, Database: "testdb", SSLMode: "disable", Extra: map[string]interface{}{"_type": "postgresql"}}
	poolMgr.InjectForTest(200, mockDrv, cfg)

	encPass, _ := crypto.Encrypt("test", encKey)
	_, err := d.DB.ExecContext(context.Background(),
		`INSERT INTO datasources (id, name, type, host, port, username, password_encrypted, database, sslmode) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		200, "pg-test", "postgresql", "localhost", 5432, "root", encPass, "testdb", "disable")
	if err != nil {
		t.Fatalf("insert datasource: %v", err)
	}

	ds, err := dsSvc.GetDataSource(context.Background(), 200)
	if err != nil {
		t.Fatalf("get datasource: %v", err)
	}

	results, err := svc.executeSQL(context.Background(), ds, "testdb", "postgresql", "CREATE TABLE t1 (id INT); INVALID SQL")
	if err == nil {
		t.Fatal("expected error from failed statement, got nil")
	}

	// 验证回滚标记被正确传递
	if len(results) != 2 {
		t.Fatalf("expected 2 results (including rolled_back), got %d", len(results))
	}
	if results[0].Status != "rolled_back" {
		t.Errorf("result[0].Status = %q, want rolled_back", results[0].Status)
	}
	if results[1].Status != "error" {
		t.Errorf("result[1].Status = %q, want error", results[1].Status)
	}
}
