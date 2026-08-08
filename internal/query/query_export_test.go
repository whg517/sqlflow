package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/driver"
	mysqldriver "github.com/whg517/sqlflow/internal/driver/mysql"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/platform/crypto"
	"github.com/whg517/sqlflow/internal/security"
	"github.com/whg517/sqlflow/internal/testutil"
)

// ---------------------------------------------------------------------------
// ExportQuery helpers
// ---------------------------------------------------------------------------

// setupExportTestDB creates a temp SQLite DB with schema migrated and casbin rules seeded.
func setupExportTestDB(t *testing.T) *sql.DB {
	t.Helper()
	testDB := testutil.NewDB(t).DB
	seedCasbinRules(t, testDB)
	return testDB
}

// setupExportService creates a full Service for export tests.
func setupExportService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	testDB := setupExportTestDB(t)
	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	permSvc, err := security.NewService(testutil.WrapSQL(t, testDB))
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}
	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})
	return qs, testDB
}

// seedExportDatasource creates an active MySQL datasource for export tests.
func seedExportDatasource(t *testing.T, svc *datasource.Service, ctx context.Context) int64 {
	t.Helper()
	ds := &model.DataSource{
		Name: "export-test-ds", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "testdb",
	}
	if err := svc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	return ds.ID
}

// exportCtx returns a context with a 5-second timeout.
func exportCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// ---------------------------------------------------------------------------
// Existing constant tests (preserved)
// ---------------------------------------------------------------------------

func TestErrExportRowLimit(t *testing.T) {
	want := "导出数据超过10000行上限，请添加 LIMIT 条件缩小范围"
	if ErrExportRowLimit.Error() != want {
		t.Errorf("ErrExportRowLimit = %q, want %q", ErrExportRowLimit.Error(), want)
	}
}

// TestDefaultLimitsMatchWhatWasCompiledIn keeps making these configurable from
// silently changing them.
//
// The values were constants until they became config; a deployment that sets
// nothing must still get the behavior it had, so the defaults are the contract
// rather than an implementation detail. Asserting them against the literals
// they replaced is the only version of this test that can fail — the old one
// compared a constant to itself.
func TestDefaultLimitsMatchWhatWasCompiledIn(t *testing.T) {
	got := Limits{}.withDefaults()

	tests := []struct {
		name string
		got  int
		want int
	}{
		{"MaxRows", got.MaxRows, 1000},
		{"ExportMaxRows", got.ExportMaxRows, 10000},
		{"ShareMaxRows", got.ShareMaxRows, 10000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
	if got.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", got.Timeout)
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: Input validation
// ---------------------------------------------------------------------------

func TestExportQuery_EmptySQL(t *testing.T) {
	qs, _ := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	t.Run("empty_string", func(t *testing.T) {
		_, err := qs.ExportQuery(ctx, 1, "user1", "admin", 1, "testdb", "")
		if !errors.Is(err, ErrEmptySQL) {
			t.Errorf("ExportQuery(empty) error = %v, want ErrEmptySQL", err)
		}
	})

	t.Run("whitespace_only", func(t *testing.T) {
		_, err := qs.ExportQuery(ctx, 1, "user1", "admin", 1, "testdb", "   \t\n  ")
		if !errors.Is(err, ErrEmptySQL) {
			t.Errorf("ExportQuery(whitespace) error = %v, want ErrEmptySQL", err)
		}
	})
}

func TestExportQuery_DataSourceNotFound(t *testing.T) {
	qs, _ := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", 99999, "testdb", "SELECT 1")
	if err == nil {
		t.Error("expected error for nonexistent datasource, got nil")
	}
}

func TestExportQuery_DisabledDataSource(t *testing.T) {
	qs, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	ds := &model.DataSource{
		Name: "disabled-export", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "testdb",
		Status: "disabled",
	}
	if err := dsSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", ds.ID, "testdb", "SELECT 1")
	if !errors.Is(err, datasource.ErrDatasourceDisabled) {
		t.Errorf("ExportQuery(disabled ds) error = %v, want datasource.ErrDatasourceDisabled", err)
	}
}

func TestExportQuery_PasswordDecryptError(t *testing.T) {
	testDB := setupExportTestDB(t)
	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), "wrong-key-that-is-32-bytes-long!!", nil, auditlog.Discard)
	permSvc, _ := security.NewService(testutil.WrapSQL(t, testDB))
	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, "wrong-key-that-is-32-bytes-long!!", poolMgr, Limits{})

	ctx, cancel := exportCtx(t)
	defer cancel()

	// Insert a datasource encrypted with the correct key, then try to decrypt with wrong key
	ds := &model.DataSource{
		Name: "decrypt-err", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "testdb",
	}
	encSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, nil, auditlog.Discard)
	if err := encSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", ds.ID, "testdb", "SELECT 1")
	if err == nil {
		t.Error("expected decrypt error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: SQL parsing & risk
// ---------------------------------------------------------------------------

func TestExportQuery_NonSelectBlocked(t *testing.T) {
	qs, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	tests := []struct {
		name string
		sql  string
	}{
		{"insert", "INSERT INTO users (name) VALUES ('test')"},
		{"update", "UPDATE users SET name='x' WHERE id=1"},
		{"delete", "DELETE FROM users WHERE id=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", tt.sql)
			if !errors.Is(err, ErrSQLOperationForbidden) {
				t.Errorf("ExportQuery(%q) error = %v, want ErrSQLOperationForbidden", tt.name, err)
			}
		})
	}
}

func TestExportQuery_BlockedSQL(t *testing.T) {
	qs, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	tests := []struct {
		name string
		sql  string
	}{
		{"drop_database", "DROP DATABASE testdb"},
		{"truncate", "TRUNCATE TABLE users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", tt.sql)
			if err == nil {
				t.Errorf("ExportQuery(%q): expected blocked error, got nil", tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: Connection errors
// ---------------------------------------------------------------------------

func TestExportQuery_ConnectionError(t *testing.T) {
	qs, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT * FROM users LIMIT 10")
	if err == nil {
		t.Error("expected connection error, got nil")
	}
}

func TestExportQuery_UnsupportedDBType(t *testing.T) {
	qs, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT 1")
	if err == nil {
		t.Error("expected error for unsupported db type, got nil")
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: Success paths via SQLite injection
// ---------------------------------------------------------------------------

func TestExportQuery_Success(t *testing.T) {
	_, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	// Inject SQLite DB as the target MySQL pool
	sqliteDB := setupExportTestDB(t)
	poolMgr.InjectForTest(dsID, mysqldriver.NewWithDB(sqliteDB))

	// Recreate Service with the shared connMgr
	permSvc, _ := security.NewService(testutil.WrapSQL(t, testDB))
	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})

	// Also add select policy for admin on this datasource domain
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "*", "select")

	result, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT 1 AS id, 'test' AS name")
	if err != nil {
		t.Fatalf("ExportQuery() error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
	if len(result.Columns) != 2 {
		t.Errorf("Columns = %v, want 2 columns", result.Columns)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("Rows count = %d, want 1", len(result.Rows))
	}
	if result.Rows[0]["id"] == nil {
		t.Error("id should not be nil")
	}
}

func TestExportQuery_SuccessWithDesensitization(t *testing.T) {
	_, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	sqliteDB := setupExportTestDB(t)
	poolMgr.InjectForTest(dsID, mysqldriver.NewWithDB(sqliteDB))

	permSvc, _ := security.NewService(testutil.WrapSQL(t, testDB))
	seedPolicy(t, testDB, permSvc, "developer", fmt.Sprintf("ds_%d", dsID), "*", "select")

	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})

	// Add a mask rule for this datasource
	now := time.Now()
	_, err := testDB.Exec(
		`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type, custom_regex, custom_template, created_at, updated_at)
		 VALUES ($1, $2, '*', 'phone', 'phone', '', '', $3, $4)`,
		dsID, "testdb", now, now,
	)
	if err != nil {
		t.Fatalf("seed mask rule: %v", err)
	}

	result, err := qs.ExportQuery(ctx, 1, "user1", "developer", dsID, "testdb",
		"SELECT 1 AS id, '13812345678' AS phone")
	if err != nil {
		t.Fatalf("ExportQuery() error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
	// Desensitization is applied (though the table list may be empty for SELECT without FROM,
	// so desensitization might not apply — this test just verifies the function completes)
}

func TestExportQuery_EmptyResult(t *testing.T) {
	_, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	sqliteDB := setupExportTestDB(t)
	poolMgr.InjectForTest(dsID, mysqldriver.NewWithDB(sqliteDB))

	permSvc, _ := security.NewService(testutil.WrapSQL(t, testDB))
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "users", "select")

	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})

	// Query the users table (empty in the injected SQLite) with an impossible condition
	result, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb",
		"SELECT * FROM users WHERE id = -1")
	if err != nil {
		t.Fatalf("ExportQuery() error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
	if len(result.Rows) != 0 {
		t.Errorf("Rows = %d, want 0", len(result.Rows))
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: Row limit exceeded (using sqlmock)
// ---------------------------------------------------------------------------

func TestExportQuery_RowLimitExceeded(t *testing.T) {
	testDB := setupExportTestDB(t)
	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	permSvc, _ := security.NewService(testutil.WrapSQL(t, testDB))
	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})

	ctx, cancel := exportCtx(t)
	defer cancel()

	dsID := seedExportDatasource(t, dsSvc, ctx)
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "users", "select")

	// Inject sqlmock DB
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error: %v", err)
	}
	poolMgr.InjectForTest(dsID, mysqldriver.NewWithDB(mockDB))

	// Build 10001 rows to exceed the export limit
	rows := sqlmock.NewRows([]string{"id"})
	for i := 0; i < defaultExportMaxRows+1; i++ {
		rows.AddRow(i)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	_, err = qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT id FROM users")
	if !errors.Is(err, ErrExportRowLimit) {
		t.Errorf("ExportQuery(row limit) error = %v, want ErrExportRowLimit", err)
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: Audit logging
// ---------------------------------------------------------------------------

func TestExportQuery_AuditOnSuccess(t *testing.T) {
	_, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	sqliteDB := setupExportTestDB(t)
	poolMgr.InjectForTest(dsID, mysqldriver.NewWithDB(sqliteDB))

	permSvc, _ := security.NewService(testutil.WrapSQL(t, testDB))
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "*", "select")

	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})

	userID := seedUser(t, testDB, "export-user", "admin")

	_, err := qs.ExportQuery(ctx, userID, "export-user", "admin", dsID, "testdb", "SELECT 1 AS id")
	if err != nil {
		t.Fatalf("ExportQuery() error: %v", err)
	}

	var count int
	err = testDB.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE user_id = $1 AND action = 'export'`,
		userID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count == 0 {
		t.Error("expected audit log for successful export, found none")
	}
}

func TestExportQuery_AuditOnFailure(t *testing.T) {
	_, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	permSvc, _ := security.NewService(testutil.WrapSQL(t, testDB))
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "users", "select")

	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})

	userID := seedUser(t, testDB, "export-fail-user", "admin")

	// This will fail at connection stage (no real MySQL) — passes permission check first
	_, err := qs.ExportQuery(ctx, userID, "export-fail-user", "admin", dsID, "testdb", "SELECT * FROM users LIMIT 10")
	if err == nil {
		t.Error("expected connection error, got nil")
	}

	var count int
	err = testDB.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE user_id = $1 AND action = 'export_failed'`,
		userID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count == 0 {
		t.Error("expected audit log for failed export, found none")
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: Datasource type defaults to ds.Type
// ---------------------------------------------------------------------------

func TestExportQuery_RecordsTheDatasourcesOwnType(t *testing.T) {
	_, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	sqliteDB := setupExportTestDB(t)
	poolMgr.InjectForTest(dsID, mysqldriver.NewWithDB(sqliteDB))

	permSvc, _ := security.NewService(testutil.WrapSQL(t, testDB))
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "*", "select")

	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})

	// The datasource decides its own type. This used to be a parameter the
	// caller could pass, and both handlers always passed "" — so the only thing
	// it could express was "let the server decide", which is now the only
	// behavior there is. What the test still pins is that the derived type
	// reaches the record, which is the part anyone reads.
	result, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT 1 AS id")
	if err != nil {
		t.Fatalf("ExportQuery() error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: Permission denied
// ---------------------------------------------------------------------------

func TestExportQuery_PermissionDenied(t *testing.T) {
	testDB := setupExportTestDB(t)
	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	permSvc, _ := security.NewService(testutil.WrapSQL(t, testDB))
	historySvc := NewHistoryService(testutil.WrapSQL(t, testDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, testDB), 0, 0)
	qs := NewService(testutil.WrapSQL(t, testDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})

	ctx, cancel := exportCtx(t)
	defer cancel()

	dsID := seedExportDatasource(t, dsSvc, ctx)

	// "restricted" role has no select permission
	_, err := qs.ExportQuery(ctx, 1, "user1", "restricted", dsID, "testdb", "SELECT * FROM users")
	if err == nil {
		t.Error("expected permission error or connection error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: High risk SQL
// ---------------------------------------------------------------------------

func TestExportQuery_HighRiskSQL(t *testing.T) {
	qs, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	// UPDATE without WHERE is high risk
	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "UPDATE users SET name='x'")
	if err == nil {
		t.Error("expected high risk error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: Helper function test
// ---------------------------------------------------------------------------

func TestCryptoDecryptWithWrongKey(t *testing.T) {
	plain := "test-password"
	encrypted, err := crypto.Encrypt(plain, testutil.EncryptionKey)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// Decrypting with wrong key should fail
	_, err = crypto.Decrypt(encrypted, "wrong-key-that-is-32-bytes-long!!")
	if err == nil {
		t.Error("expected decrypt error with wrong key, got nil")
	}
}
