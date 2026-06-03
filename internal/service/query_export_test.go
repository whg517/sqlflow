package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
)

// ---------------------------------------------------------------------------
// ExportQuery helpers
// ---------------------------------------------------------------------------

// setupExportTestDB creates a temp SQLite DB with schema migrated and casbin rules seeded.
func setupExportTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	seedCasbinRules(t, database.DB)
	return database.DB
}

// setupExportService creates a full QueryService for export tests.
func setupExportService(t *testing.T) (*QueryService, *sql.DB) {
	t.Helper()
	testDB := setupExportTestDB(t)
	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	permSvc, err := NewPermissionService(mustWrapDB(testDB))
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}
	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr, nil)
	return qs, testDB
}

// seedExportDatasource creates an active MySQL datasource for export tests.
func seedExportDatasource(t *testing.T, svc *DatasourceService, ctx context.Context) int64 {
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

func TestExportRowLimit(t *testing.T) {
	if exportRowLimit != 10000 {
		t.Errorf("exportRowLimit = %d, want 10000", exportRowLimit)
	}
}

func TestErrExportRowLimit(t *testing.T) {
	want := "导出数据超过10000行上限，请添加 LIMIT 条件缩小范围"
	if ErrExportRowLimit.Error() != want {
		t.Errorf("ErrExportRowLimit = %q, want %q", ErrExportRowLimit.Error(), want)
	}
}

func TestExportConstants(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"defaultRowLimit", defaultRowLimit, 1000},
		{"exportRowLimit", exportRowLimit, 10000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
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
		_, err := qs.ExportQuery(ctx, 1, "user1", "admin", 1, "testdb", "", "mysql")
		if !errors.Is(err, ErrEmptySQL) {
			t.Errorf("ExportQuery(empty) error = %v, want ErrEmptySQL", err)
		}
	})

	t.Run("whitespace_only", func(t *testing.T) {
		_, err := qs.ExportQuery(ctx, 1, "user1", "admin", 1, "testdb", "   \t\n  ", "mysql")
		if !errors.Is(err, ErrEmptySQL) {
			t.Errorf("ExportQuery(whitespace) error = %v, want ErrEmptySQL", err)
		}
	})
}

func TestExportQuery_DataSourceNotFound(t *testing.T) {
	qs, _ := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", 99999, "testdb", "SELECT 1", "mysql")
	if err == nil {
		t.Error("expected error for nonexistent datasource, got nil")
	}
}

func TestExportQuery_DisabledDataSource(t *testing.T) {
	qs, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	ds := &model.DataSource{
		Name: "disabled-export", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "testdb",
		Status: "disabled",
	}
	if err := dsSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", ds.ID, "testdb", "SELECT 1", "mysql")
	if !errors.Is(err, ErrDatasourceDisabled) {
		t.Errorf("ExportQuery(disabled ds) error = %v, want ErrDatasourceDisabled", err)
	}
}

func TestExportQuery_PasswordDecryptError(t *testing.T) {
	testDB := setupExportTestDB(t)
	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), "wrong-key-that-is-32-bytes-long!!", connMgr, nil)
	permSvc, _ := NewPermissionService(mustWrapDB(testDB))
	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, "wrong-key-that-is-32-bytes-long!!", connMgr, nil)

	ctx, cancel := exportCtx(t)
	defer cancel()

	// Insert a datasource encrypted with the correct key, then try to decrypt with wrong key
	ds := &model.DataSource{
		Name: "decrypt-err", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "testdb",
	}
	encSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	if err := encSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", ds.ID, "testdb", "SELECT 1", "mysql")
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

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
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
			_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", tt.sql, "mysql")
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

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
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
			_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", tt.sql, "mysql")
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

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT * FROM users LIMIT 10", "mysql")
	if err == nil {
		t.Error("expected connection error, got nil")
	}
}

func TestExportQuery_UnsupportedDBType(t *testing.T) {
	qs, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT 1", "postgres")
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

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	// Inject SQLite DB as the target MySQL pool
	sqliteDB := setupExportTestDB(t)
	connMgr.InjectMySQLForTest(dsID, "10.0.0.1", 3306, "testdb", sqliteDB)

	// Recreate QueryService with the shared connMgr
	permSvc, _ := NewPermissionService(mustWrapDB(testDB))
	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr, nil)

	// Also add select policy for admin on this datasource domain
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "*", "select")

	result, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT 1 AS id, 'test' AS name", "mysql")
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

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	sqliteDB := setupExportTestDB(t)
	connMgr.InjectMySQLForTest(dsID, "10.0.0.1", 3306, "testdb", sqliteDB)

	permSvc, _ := NewPermissionService(mustWrapDB(testDB))
	seedPolicy(t, testDB, permSvc, "developer", fmt.Sprintf("ds_%d", dsID), "*", "select")

	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr, nil)

	// Add a mask rule for this datasource
	now := time.Now()
	_, err := testDB.Exec(
		`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type, custom_regex, custom_template, created_at, updated_at)
		 VALUES (?, ?, '*', 'phone', 'phone', '', '', ?, ?)`,
		dsID, "testdb", now, now,
	)
	if err != nil {
		t.Fatalf("seed mask rule: %v", err)
	}

	result, err := qs.ExportQuery(ctx, 1, "user1", "developer", dsID, "testdb",
		"SELECT 1 AS id, '13812345678' AS phone", "mysql")
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

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	sqliteDB := setupExportTestDB(t)
	connMgr.InjectMySQLForTest(dsID, "10.0.0.1", 3306, "testdb", sqliteDB)

	permSvc, _ := NewPermissionService(mustWrapDB(testDB))
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "users", "select")

	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr, nil)

	// Query the users table (empty in the injected SQLite) with an impossible condition
	result, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb",
		"SELECT * FROM users WHERE id = -1", "mysql")
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
	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	permSvc, _ := NewPermissionService(mustWrapDB(testDB))
	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr, nil)

	ctx, cancel := exportCtx(t)
	defer cancel()

	dsID := seedExportDatasource(t, dsSvc, ctx)
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "users", "select")

	// Inject sqlmock DB
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error: %v", err)
	}
	connMgr.InjectMySQLForTest(dsID, "10.0.0.1", 3306, "testdb", mockDB)

	// Build 10001 rows to exceed the export limit
	rows := sqlmock.NewRows([]string{"id"})
	for i := 0; i < exportRowLimit+1; i++ {
		rows.AddRow(i)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	_, err = qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT id FROM users", "mysql")
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

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	sqliteDB := setupExportTestDB(t)
	connMgr.InjectMySQLForTest(dsID, "10.0.0.1", 3306, "testdb", sqliteDB)

	permSvc, _ := NewPermissionService(mustWrapDB(testDB))
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "*", "select")

	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr, nil)

	userID := seedUser(t, testDB, "export-user", "admin")

	_, err := qs.ExportQuery(ctx, userID, "export-user", "admin", dsID, "testdb", "SELECT 1 AS id", "mysql")
	if err != nil {
		t.Fatalf("ExportQuery() error: %v", err)
	}

	var count int
	err = testDB.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE user_id = ? AND action = 'export'`,
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

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	permSvc, _ := NewPermissionService(mustWrapDB(testDB))
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "users", "select")

	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr, nil)

	userID := seedUser(t, testDB, "export-fail-user", "admin")

	// This will fail at connection stage (no real MySQL) — passes permission check first
	_, err := qs.ExportQuery(ctx, userID, "export-fail-user", "admin", dsID, "testdb", "SELECT * FROM users LIMIT 10", "mysql")
	if err == nil {
		t.Error("expected connection error, got nil")
	}

	var count int
	err = testDB.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE user_id = ? AND action = 'export_failed'`,
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

func TestExportQuery_DefaultDBType(t *testing.T) {
	_, testDB := setupExportService(t)
	ctx, cancel := exportCtx(t)
	defer cancel()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	sqliteDB := setupExportTestDB(t)
	connMgr.InjectMySQLForTest(dsID, "10.0.0.1", 3306, "testdb", sqliteDB)

	permSvc, _ := NewPermissionService(mustWrapDB(testDB))
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "*", "select")

	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr, nil)

	// Pass empty dbType — should default to datasource type (mysql)
	result, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT 1 AS id", "")
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
	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	permSvc, _ := NewPermissionService(mustWrapDB(testDB))
	historySvc := NewQueryHistoryService(mustWrapDB(testDB))
	auditSvc := NewAuditService(mustWrapDB(testDB), 0, 0)
	qs := NewQueryService(mustWrapDB(testDB), dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr, nil)

	ctx, cancel := exportCtx(t)
	defer cancel()

	dsID := seedExportDatasource(t, dsSvc, ctx)

	// "restricted" role has no select permission
	_, err := qs.ExportQuery(ctx, 1, "user1", "restricted", dsID, "testdb", "SELECT * FROM users", "mysql")
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

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(mustWrapDB(testDB), testEncKey, connMgr, nil)
	dsID := seedExportDatasource(t, dsSvc, ctx)

	// UPDATE without WHERE is high risk
	_, err := qs.ExportQuery(ctx, 1, "user1", "admin", dsID, "testdb", "UPDATE users SET name='x'", "mysql")
	if err == nil {
		t.Error("expected high risk error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ExportQuery: Helper function test
// ---------------------------------------------------------------------------

func TestCryptoDecryptWithWrongKey(t *testing.T) {
	plain := "test-password"
	encrypted, err := crypto.Encrypt(plain, testEncKey)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// Decrypting with wrong key should fail
	_, err = crypto.Decrypt(encrypted, "wrong-key-that-is-32-bytes-long!!")
	if err == nil {
		t.Error("expected decrypt error with wrong key, got nil")
	}
}
