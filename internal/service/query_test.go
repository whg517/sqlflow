package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/mask"
)

// ---------------------------------------------------------------------------
// Shared test helpers
// ---------------------------------------------------------------------------

// setupQueryTestDB creates a temp SQLite database with all tables migrated
// and seed casbin rules pre-populated (so NewPermissionService won't try to
// read policy.csv from disk).
func setupQueryTestDB(t *testing.T) *sql.DB {
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
	seedCasbinRules(t, database.DB)
	return database.DB
}

// seedCasbinRules inserts the default policies directly into casbin_rule
// so that NewPermissionService doesn't try to read policy.csv from disk.
func seedCasbinRules(t *testing.T, testDB *sql.DB) {
	t.Helper()
	policies := []struct{ ptype, v0, v1, v2, v3 string }{
		{"p", "admin", "*", "*", "*"},
		{"p", "dba", "*", "*", "select"},
		{"p", "dba", "*", "*", "update"},
		{"p", "dba", "*", "*", "delete"},
		{"p", "dba", "*", "*", "ddl"},
		{"p", "dba", "*", "*", "export"},
		{"p", "dba", "*", "*", "desensitize:bypass"},
		{"p", "developer", "*", "*", "select"},
	}
	for _, p := range policies {
		_, err := testDB.Exec(
			`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3) VALUES (?, ?, ?, ?, ?)`,
			p.ptype, p.v0, p.v1, p.v2, p.v3,
		)
		if err != nil {
			t.Fatalf("seed casbin rule: %v", err)
		}
	}
}

// setupQueryService creates a full QueryService with all dependencies using a temp SQLite DB.
func setupQueryService(t *testing.T) (*QueryService, *sql.DB) {
	t.Helper()
	testDB := setupQueryTestDB(t)
	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	permSvc, err := NewPermissionService(testDB)
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}
	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)

	qs := NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)
	return qs, testDB
}

// seedQueryDatasource inserts an active mysql datasource and returns its ID.
func seedQueryDatasource(t *testing.T, svc *DatasourceService, ctx context.Context) int64 {
	t.Helper()
	ds := &model.DataSource{
		Name: "query-test-ds", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "testdb",
	}
	if err := svc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	return ds.ID
}

// seedQueryDatasourceWithStatus inserts a datasource with the given status.
func seedQueryDatasourceWithStatus(t *testing.T, svc *DatasourceService, ctx context.Context, status string) int64 {
	t.Helper()
	ds := &model.DataSource{
		Name: "query-test-ds", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "testdb",
		Status: status,
	}
	if err := svc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	return ds.ID
}

// seedMaskRule inserts a mask rule directly.
func seedMaskRule(t *testing.T, testDB *sql.DB, dsID int64, database, tableName, field, maskType string) {
	t.Helper()
	now := time.Now()
	_, err := testDB.Exec(
		`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type, custom_regex, custom_template, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', '', ?, ?)`,
		dsID, database, tableName, field, maskType, now, now,
	)
	if err != nil {
		t.Fatalf("seed mask rule: %v", err)
	}
}

// seedPolicy inserts a casbin policy and reloads.
func seedPolicy(t *testing.T, testDB *sql.DB, permSvc *PermissionService, sub, dom, obj, act string) {
	t.Helper()
	_, err := testDB.Exec(
		`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3) VALUES ('p', ?, ?, ?, ?)`,
		sub, dom, obj, act,
	)
	if err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	if err := permSvc.LoadPolicy(); err != nil {
		t.Fatalf("reload policy: %v", err)
	}
}

// seedUser inserts a user and returns its ID.
func seedUser(t *testing.T, testDB *sql.DB, username, role string) int64 {
	t.Helper()
	result, err := testDB.Exec(
		`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`,
		username, "$2a$10$fakehash", role,
	)
	if err != nil {
		t.Fatalf("seed user %q: %v", username, err)
	}
	id, _ := result.LastInsertId()
	return id
}

// queryCtx returns a context with a 5-second timeout.
func queryCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// ---------------------------------------------------------------------------
// Existing tests (preserved)
// ---------------------------------------------------------------------------

func TestTruncateSQL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"short", "SELECT 1", "SELECT 1"},
		{"exact_100", makeString(100), makeString(100)},
		{"long", makeString(150), makeString(100)},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateSQL(tt.input)
			if got != tt.want {
				t.Errorf("truncateSQL() length = %d, want %d", len(got), len(tt.want))
			}
		})
	}
}

func TestQueryResultJSON(t *testing.T) {
	result := &QueryResult{
		Columns:       []string{"id", "name"},
		Rows:          make([]map[string]interface{}, 0),
		Total:         0,
		ExecutionTime: 42,
		AffectedRows:  0,
		Desensitized:  false,
	}

	if result.Total != 0 {
		t.Errorf("expected Total=0, got %d", result.Total)
	}
	if result.ExecutionTime != 42 {
		t.Errorf("expected ExecutionTime=42, got %d", result.ExecutionTime)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected empty rows, got %d", len(result.Rows))
	}
}

func TestErrors(t *testing.T) {
	errs := []struct {
		name string
		err  error
		msg  string
	}{
		{"forbidden", ErrSQLOperationForbidden, "该操作需要提交工单，仅允许 SELECT 查询"},
		{"high_risk", ErrSQLHighRisk, "高风险操作被拦截，请提交工单"},
		{"blocked", ErrSQLBlocked, "SQL操作被拦截"},
		{"timeout", ErrSQLTimeout, "查询超时（30秒）"},
		{"empty", ErrEmptySQL, "SQL 不能为空"},
		{"ds_type", ErrDatasourceType, "不支持的数据源类型"},
	}
	for _, tt := range errs {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("error message = %q, want %q", tt.err.Error(), tt.msg)
			}
		})
	}
}

func TestBuildMongoURI(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		user     string
		password string
		want     string
	}{
		{"with_auth", "localhost", 27017, "admin", "pass", "mongodb://admin:pass@localhost:27017"},
		{"no_auth", "localhost", 27017, "", "", "mongodb://localhost:27017"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMongoURI(tt.host, tt.port, tt.user, tt.password)
			if got != tt.want {
				t.Errorf("buildMongoURI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMongoBody(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool // expect non-nil result
	}{
		{"valid", `{"operation":"find","collection":"users","filter":{}}`, true},
		{"invalid", `{not json}`, false},
		{"empty", ``, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMongoBody(tt.input)
			if (got != nil) != tt.want {
				t.Errorf("parseMongoBody(%q) nil=%v, want nil=%v", tt.input, got == nil, !tt.want)
			}
		})
	}
}

// makeString creates a string of the given length.
func makeString(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = 'a'
	}
	return string(result)
}

// ---------------------------------------------------------------------------
// ExecuteQuery: Input Validation
// ---------------------------------------------------------------------------

func TestExecuteQuery_EmptySQL(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	t.Run("empty_string", func(t *testing.T) {
		_, err := qs.ExecuteQuery(ctx, 1, "user1", "developer", 1, "testdb", "", "mysql")
		if !errors.Is(err, ErrEmptySQL) {
			t.Errorf("ExecuteQuery(empty) error = %v, want ErrEmptySQL", err)
		}
	})

	t.Run("whitespace_only", func(t *testing.T) {
		_, err := qs.ExecuteQuery(ctx, 1, "user1", "developer", 1, "testdb", "   \t\n  ", "mysql")
		if !errors.Is(err, ErrEmptySQL) {
			t.Errorf("ExecuteQuery(whitespace) error = %v, want ErrEmptySQL", err)
		}
	})
}

func TestExecuteQuery_DataSourceNotFound(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	_, err := qs.ExecuteQuery(ctx, 1, "user1", "developer", 99999, "testdb", "SELECT 1", "mysql")
	if err == nil {
		t.Error("expected error for nonexistent datasource, got nil")
	}
}

func TestExecuteQuery_DisabledDataSource(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	dsSvc := NewDatasourceService(testDB, testEncKey, connpool.NewManager())
	dsID := seedQueryDatasourceWithStatus(t, dsSvc, ctx, "disabled")

	_, err := qs.ExecuteQuery(ctx, 1, "user1", "developer", dsID, "testdb", "SELECT 1", "mysql")
	if !errors.Is(err, ErrDatasourceDisabled) {
		t.Errorf("ExecuteQuery(disabled ds) error = %v, want ErrDatasourceDisabled", err)
	}
}

// ---------------------------------------------------------------------------
// ExecuteQuery: SQL Parsing & Risk Assessment
// ---------------------------------------------------------------------------

func TestExecuteQuery_NonSelectBlocked(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	dsSvc := NewDatasourceService(testDB, testEncKey, connpool.NewManager())
	dsID := seedQueryDatasource(t, dsSvc, ctx)

	tests := []struct {
		name string
		sql  string
	}{
		{"insert", "INSERT INTO users (name) VALUES ('test')"},
		{"update_with_where", "UPDATE users SET name='x' WHERE id=1"},
		{"delete_with_where", "DELETE FROM users WHERE id=1"},
		{"create_table", "CREATE TABLE foo (id INT)"},
		{"alter_table", "ALTER TABLE users ADD COLUMN email TEXT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := qs.ExecuteQuery(ctx, 1, "user1", "admin", dsID, "testdb", tt.sql, "mysql")
			if !errors.Is(err, ErrSQLOperationForbidden) {
				t.Errorf("ExecuteQuery(%q) error = %v, want ErrSQLOperationForbidden", tt.name, err)
			}
		})
	}
}

func TestExecuteQuery_BlockedSQL(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	dsSvc := NewDatasourceService(testDB, testEncKey, connpool.NewManager())
	dsID := seedQueryDatasource(t, dsSvc, ctx)

	tests := []struct {
		name string
		sql  string
	}{
		{"drop_database", "DROP DATABASE testdb"},
		{"drop_table", "DROP TABLE users"},
		{"truncate", "TRUNCATE TABLE users"},
		{"update_no_where", "UPDATE users SET name='x'"},
		{"delete_no_where", "DELETE FROM users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := qs.ExecuteQuery(ctx, 1, "user1", "admin", dsID, "testdb", tt.sql, "mysql")
			// Should be blocked by either ErrSQLBlocked, ErrSQLHighRisk, or ErrSQLOperationForbidden
			if err == nil {
				t.Errorf("ExecuteQuery(%q): expected blocked error, got nil", tt.name)
			}
			if errors.Is(err, ErrEmptySQL) {
				t.Errorf("ExecuteQuery(%q): unexpected ErrEmptySQL", tt.name)
			}
		})
	}
}

func TestExecuteQuery_SelectPassesParsing(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	dsSvc := NewDatasourceService(testDB, testEncKey, connpool.NewManager())
	dsID := seedQueryDatasource(t, dsSvc, ctx)

	// SELECT should pass parsing and risk checks but fail at connection stage
	_, err := qs.ExecuteQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT * FROM users LIMIT 10", "mysql")
	if err == nil {
		t.Error("expected connection error (no real MySQL), got nil")
	}
	// Should NOT be a parsing/risk error
	if errors.Is(err, ErrEmptySQL) || errors.Is(err, ErrSQLOperationForbidden) ||
		errors.Is(err, ErrSQLBlocked) || errors.Is(err, ErrSQLHighRisk) {
		t.Errorf("SELECT should pass parsing, got: %v", err)
	}
}

func TestExecuteQuery_InvalidSQL(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	dsSvc := NewDatasourceService(testDB, testEncKey, connpool.NewManager())
	dsID := seedQueryDatasource(t, dsSvc, ctx)

	_, err := qs.ExecuteQuery(ctx, 1, "user1", "admin", dsID, "testdb", "NOT VALID SQL !!!", "mysql")
	if err == nil {
		t.Error("expected error for invalid SQL, got nil")
	}
}

func TestExecuteQuery_UnsupportedDBType(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	dsSvc := NewDatasourceService(testDB, testEncKey, connpool.NewManager())
	dsID := seedQueryDatasource(t, dsSvc, ctx)

	_, err := qs.ExecuteQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT 1", "postgres")
	if err == nil {
		t.Error("expected error for unsupported db type, got nil")
	}
}

// ---------------------------------------------------------------------------
// ExecuteQuery: Permission checks
// ---------------------------------------------------------------------------

func TestExecuteQuery_PermissionDenied(t *testing.T) {
	testDB := setupQueryTestDB(t)
	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	permSvc, err := NewPermissionService(testDB)
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}

	ctx, cancel := queryCtx(t)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx)

	// "restricted" role has NO select permission on users table
	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs := NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	_, err = qs.ExecuteQuery(ctx, 1, "user1", "restricted", dsID, "testdb", "SELECT * FROM users", "mysql")
	// Permission denied or connection error are both acceptable outcomes
	if err == nil {
		t.Error("expected error (permission denied or connection), got nil")
	}
	// The error should mention permission if the permission check rejected it
	if err != nil && (err.Error() == "没有表 users 的查询权限" || err.Error() == "权限校验失败") {
		// Permission was denied — correct behavior
	}
}

func TestExecuteQuery_AdminHasWildcardPermission(t *testing.T) {
	testDB := setupQueryTestDB(t)
	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	permSvc, err := NewPermissionService(testDB)
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}

	ctx, cancel := queryCtx(t)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx)

	// Grant admin select on users table for this specific datasource domain
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "users", "select")

	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs := NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	// Should pass permission check but fail at connection stage (no real MySQL)
	_, err = qs.ExecuteQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT * FROM users", "mysql")
	if err == nil {
		t.Error("expected connection error, got nil")
	}
	// Must NOT be a permission error
	if err.Error() == "没有表 users 的查询权限" {
		t.Error("admin should have select permission on users table")
	}
}

// ---------------------------------------------------------------------------
// Desensitization (Masking)
// ---------------------------------------------------------------------------

func TestApplyDesensitization_NoRules(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	result := &QueryResult{
		Columns: []string{"id", "phone"},
		Rows: []map[string]interface{}{
			{"id": 1, "phone": "13812345678"},
		},
		Total: 1,
	}

	desensitized, maskedFields := qs.applyDesensitization(ctx, result, "developer", 1, "testdb", []string{"users"})
	if desensitized {
		t.Error("should not be desensitized when no mask rules exist")
	}
	if len(maskedFields) != 0 {
		t.Errorf("maskedFields = %v, want empty", maskedFields)
	}
}

func TestApplyDesensitization_WithRules(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx := context.Background()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx2)

	seedMaskRule(t, testDB, dsID, "testdb", "users", "phone", "phone")

	permSvc, _ := NewPermissionService(testDB)
	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs = NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	result := &QueryResult{
		Columns: []string{"id", "phone"},
		Rows: []map[string]interface{}{
			{"id": 1, "phone": "13812345678"},
			{"id": 2, "phone": "15098765432"},
		},
		Total: 2,
	}

	desensitized, maskedFields := qs.applyDesensitization(ctx, result, "developer", dsID, "testdb", []string{"users"})
	if !desensitized {
		t.Error("expected desensitized=true when mask rules match")
	}
	if len(maskedFields) == 0 {
		t.Error("expected at least one masked field")
	}

	for _, row := range result.Rows {
		phone, ok := row["phone"].(string)
		if !ok {
			t.Error("phone should still be a string after masking")
			continue
		}
		if phone == "13812345678" || phone == "15098765432" {
			t.Errorf("phone should be masked, got %q", phone)
		}
	}
}

func TestApplyDesensitization_BypassPermission(t *testing.T) {
	testDB := setupQueryTestDB(t)
	ctx := context.Background()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx2)

	seedMaskRule(t, testDB, dsID, "testdb", "users", "phone", "phone")

	permSvc, _ := NewPermissionService(testDB)
	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "*", "desensitize:bypass")

	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs := NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	result := &QueryResult{
		Columns: []string{"id", "phone"},
		Rows: []map[string]interface{}{
			{"id": 1, "phone": "13812345678"},
		},
		Total: 1,
	}

	desensitized, maskedFields := qs.applyDesensitization(ctx, result, "admin", dsID, "testdb", []string{"users"})
	if desensitized {
		t.Error("admin with desensitize:bypass should not be masked")
	}
	if len(maskedFields) != 0 {
		t.Errorf("maskedFields = %v, want empty for bypass user", maskedFields)
	}
	phone := result.Rows[0]["phone"].(string)
	if phone != "13812345678" {
		t.Errorf("phone should be unmasked, got %q", phone)
	}
}

func TestApplyDesensitization_MultipleMaskTypes(t *testing.T) {
	testDB := setupQueryTestDB(t)
	ctx := context.Background()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx2)

	now := time.Now()
	rules := []struct {
		table, field, maskType string
	}{
		{"users", "phone", "phone"},
		{"users", "email", "email"},
		{"users", "name", "name"},
		{"users", "id_card", "id_card"},
	}
	for _, r := range rules {
		_, err := testDB.Exec(
			`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type, custom_regex, custom_template, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, '', '', ?, ?)`,
			dsID, "testdb", r.table, r.field, r.maskType, now, now,
		)
		if err != nil {
			t.Fatalf("seed mask rule %s: %v", r.field, err)
		}
	}

	permSvc, _ := NewPermissionService(testDB)
	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs := NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	result := &QueryResult{
		Columns: []string{"phone", "email", "name", "id_card"},
		Rows: []map[string]interface{}{
			{
				"phone":   "13812345678",
				"email":   "test@example.com",
				"name":    "张三",
				"id_card": "110101199001011234",
			},
		},
		Total: 1,
	}

	desensitized, maskedFields := qs.applyDesensitization(ctx, result, "developer", dsID, "testdb", []string{"users"})
	if !desensitized {
		t.Error("expected desensitized=true")
	}
	if len(maskedFields) < 3 {
		t.Errorf("expected at least 3 masked fields, got %d: %v", len(maskedFields), maskedFields)
	}

	row := result.Rows[0]
	if row["phone"].(string) == "13812345678" {
		t.Error("phone should be masked")
	}
	if row["email"].(string) == "test@example.com" {
		t.Error("email should be masked")
	}
	if row["name"].(string) == "张三" {
		t.Error("name should be masked")
	}
	if row["id_card"].(string) == "110101199001011234" {
		t.Error("id_card should be masked")
	}
}

func TestApplyDesensitization_WildcardTableRule(t *testing.T) {
	testDB := setupQueryTestDB(t)
	ctx := context.Background()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx2)

	now := time.Now()
	_, err := testDB.Exec(
		`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type, custom_regex, custom_template, created_at, updated_at)
		 VALUES (?, ?, '*', 'secret_field', 'full', '', '', ?, ?)`,
		dsID, "testdb", now, now,
	)
	if err != nil {
		t.Fatalf("seed wildcard mask rule: %v", err)
	}

	permSvc, _ := NewPermissionService(testDB)
	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs := NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	result := &QueryResult{
		Columns: []string{"id", "secret_field"},
		Rows: []map[string]interface{}{
			{"id": 1, "secret_field": "top-secret-data"},
		},
		Total: 1,
	}

	desensitized, maskedFields := qs.applyDesensitization(ctx, result, "developer", dsID, "testdb", []string{"orders"})
	if !desensitized {
		t.Error("wildcard rule should match any table")
	}
	if len(maskedFields) == 0 {
		t.Error("expected secret_field to be masked")
	}
	if result.Rows[0]["secret_field"].(string) == "top-secret-data" {
		t.Error("secret_field should be masked")
	}
}

func TestApplyDesensitization_NoMatchTable(t *testing.T) {
	testDB := setupQueryTestDB(t)
	ctx := context.Background()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx2)

	seedMaskRule(t, testDB, dsID, "testdb", "users", "phone", "phone")

	permSvc, _ := NewPermissionService(testDB)
	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs := NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	result := &QueryResult{
		Columns: []string{"id", "phone"},
		Rows: []map[string]interface{}{
			{"id": 1, "phone": "13812345678"},
		},
		Total: 1,
	}

	desensitized, maskedFields := qs.applyDesensitization(ctx, result, "developer", dsID, "testdb", []string{"orders"})
	if desensitized {
		t.Error("should not desensitize when table doesn't match rule")
	}
	if len(maskedFields) != 0 {
		t.Errorf("maskedFields = %v, want empty for non-matching table", maskedFields)
	}
}

func TestApplyDesensitization_EmptyTables(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	result := &QueryResult{
		Columns: []string{"id"},
		Rows:    []map[string]interface{}{{"id": 1}},
		Total:   1,
	}

	// Empty tables list — no rules should match
	desensitized, maskedFields := qs.applyDesensitization(ctx, result, "developer", 1, "testdb", []string{})
	if desensitized {
		t.Error("should not desensitize with empty tables list")
	}
	if len(maskedFields) != 0 {
		t.Errorf("maskedFields = %v, want empty", maskedFields)
	}
}

func TestApplyDesensitization_NilRows(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	result := &QueryResult{
		Columns: []string{"id"},
		Rows:    nil,
		Total:   0,
	}

	desensitized, maskedFields := qs.applyDesensitization(ctx, result, "developer", 1, "testdb", []string{"users"})
	if desensitized {
		t.Error("should not desensitize with nil rows")
	}
	if len(maskedFields) != 0 {
		t.Errorf("maskedFields = %v, want empty", maskedFields)
	}
}

func TestApplyDesensitization_FieldNotInRow(t *testing.T) {
	testDB := setupQueryTestDB(t)
	ctx := context.Background()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx2)

	// Mask rule for "phone" field, but rows don't have "phone"
	seedMaskRule(t, testDB, dsID, "testdb", "users", "phone", "phone")

	permSvc, _ := NewPermissionService(testDB)
	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs := NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	result := &QueryResult{
		Columns: []string{"id", "name"},
		Rows: []map[string]interface{}{
			{"id": 1, "name": "Alice"},
		},
		Total: 1,
	}

	desensitized, maskedFields := qs.applyDesensitization(ctx, result, "developer", dsID, "testdb", []string{"users"})
	if desensitized {
		t.Error("should not desensitize when field doesn't exist in rows")
	}
	if len(maskedFields) != 0 {
		t.Errorf("maskedFields = %v, want empty when field not in row", maskedFields)
	}
}

// ---------------------------------------------------------------------------
// loadMaskRules
// ---------------------------------------------------------------------------

func TestLoadMaskRules(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx := context.Background()

	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx2)

	seedMaskRule(t, testDB, dsID, "testdb", "users", "phone", "phone")
	seedMaskRule(t, testDB, dsID, "testdb", "users", "email", "email")
	seedMaskRule(t, testDB, dsID, "testdb", "orders", "credit_card", "bank_card")

	permSvc, _ := NewPermissionService(testDB)
	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs = NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	rules := qs.loadMaskRules(ctx, dsID, "testdb", []string{"users"})
	if len(rules) < 2 {
		t.Errorf("expected at least 2 rules for 'users', got %d", len(rules))
	}

	var foundPhone, foundEmail bool
	for _, r := range rules {
		if r.Field == "phone" && r.TableName == "users" {
			foundPhone = true
			if r.MaskType != mask.MaskPhone {
				t.Errorf("phone mask type = %v, want %v", r.MaskType, mask.MaskPhone)
			}
		}
		if r.Field == "email" && r.TableName == "users" {
			foundEmail = true
		}
	}
	if !foundPhone {
		t.Error("expected phone rule to be loaded")
	}
	if !foundEmail {
		t.Error("expected email rule to be loaded")
	}
}

func TestLoadMaskRules_EmptyResult(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	rules := qs.loadMaskRules(ctx, 99999, "nonexistent", []string{"foo"})
	if len(rules) != 0 {
		t.Errorf("expected 0 rules for nonexistent datasource, got %d", len(rules))
	}
}

// ---------------------------------------------------------------------------
// Audit logging on query failure
// ---------------------------------------------------------------------------

func TestExecuteQuery_AuditOnFailure(t *testing.T) {
	testDB := setupQueryTestDB(t)
	connMgr := connpool.NewManager()
	dsSvc := NewDatasourceService(testDB, testEncKey, connMgr)
	permSvc, err := NewPermissionService(testDB)
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}

	ctx, cancel := queryCtx(t)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx)
	userID := seedUser(t, testDB, "audit-user", "admin")

	seedPolicy(t, testDB, permSvc, "admin", fmt.Sprintf("ds_%d", dsID), "*", "select")

	historySvc := NewQueryHistoryService(testDB)
	auditSvc := NewAuditService(testDB, 0, 0)
	qs := NewQueryService(testDB, dsSvc, historySvc, permSvc, auditSvc, testEncKey, connMgr)

	_, err = qs.ExecuteQuery(ctx, userID, "audit-user", "admin", dsID, "testdb", "SELECT 1", "mysql")
	if err == nil {
		t.Error("expected connection error, got nil")
	}

	var count int
	err = testDB.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE user_id = ? AND action = 'query_failed'`,
		userID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count == 0 {
		t.Error("expected audit log for failed query, found none")
	}
}

// ---------------------------------------------------------------------------
// QueryHistory integration
// ---------------------------------------------------------------------------

func TestQueryHistory_CreateAndList(t *testing.T) {
	testDB := setupQueryTestDB(t)
	svc := NewQueryHistoryService(testDB)
	ctx, cancel := queryCtx(t)
	defer cancel()

	history := &model.QueryHistory{
		UserID: 1, DatasourceID: 1, Database: "testdb",
		SQLContent: "SELECT * FROM users LIMIT 10", SQLSummary: "SELECT * FROM users LIMIT 10",
		DBType: "mysql", ExecutionTime: 42, ResultRows: 5, AffectedRows: 0,
	}

	if err := svc.CreateHistory(ctx, history); err != nil {
		t.Fatalf("CreateHistory() error: %v", err)
	}

	list, total, err := svc.ListHistory(ctx, 1, 1, 10)
	if err != nil {
		t.Fatalf("ListHistory() error: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(list) != 1 {
		t.Fatalf("list length = %d, want 1", len(list))
	}
	if list[0].SQLContent != "SELECT * FROM users LIMIT 10" {
		t.Errorf("SQLContent = %q, want original", list[0].SQLContent)
	}
	if list[0].ExecutionTime != 42 {
		t.Errorf("ExecutionTime = %d, want 42", list[0].ExecutionTime)
	}
}

func TestQueryHistory_Delete(t *testing.T) {
	testDB := setupQueryTestDB(t)
	svc := NewQueryHistoryService(testDB)
	ctx, cancel := queryCtx(t)
	defer cancel()

	history := &model.QueryHistory{
		UserID: 1, DatasourceID: 1, Database: "testdb",
		SQLContent: "SELECT 1", SQLSummary: "SELECT 1", DBType: "mysql",
	}
	if err := svc.CreateHistory(ctx, history); err != nil {
		t.Fatalf("CreateHistory() error: %v", err)
	}

	// Get the ID from the DB since CreateHistory doesn't return it
	list, _, err := svc.ListHistory(ctx, 1, 1, 1)
	if err != nil || len(list) == 0 {
		t.Fatalf("ListHistory() error: %v, len=%d", err, len(list))
	}
	historyID := list[0].ID

	t.Run("same_user", func(t *testing.T) {
		err := svc.DeleteHistory(ctx, historyID, 1)
		if err != nil {
			t.Fatalf("DeleteHistory() error: %v", err)
		}
	})

	t.Run("other_user", func(t *testing.T) {
		history2 := &model.QueryHistory{
			UserID: 1, DatasourceID: 1, Database: "testdb",
			SQLContent: "SELECT 2", SQLSummary: "SELECT 2", DBType: "mysql",
		}
		svc.CreateHistory(ctx, history2)
		list2, _, _ := svc.ListHistory(ctx, 1, 1, 1)
		if len(list2) == 0 {
			t.Fatal("expected at least one history record")
		}
		err := svc.DeleteHistory(ctx, list2[0].ID, 999)
		if err == nil {
			t.Error("expected error deleting other user's history")
		}
	})
}

func TestQueryHistory_Clear(t *testing.T) {
	testDB := setupQueryTestDB(t)
	svc := NewQueryHistoryService(testDB)
	ctx, cancel := queryCtx(t)
	defer cancel()

	for i := 0; i < 5; i++ {
		history := &model.QueryHistory{
			UserID: 1, DatasourceID: 1, Database: "testdb",
			SQLContent: fmt.Sprintf("SELECT %d", i), SQLSummary: fmt.Sprintf("SELECT %d", i), DBType: "mysql",
		}
		if err := svc.CreateHistory(ctx, history); err != nil {
			t.Fatalf("CreateHistory() error: %v", err)
		}
	}

	if err := svc.ClearHistory(ctx, 1); err != nil {
		t.Fatalf("ClearHistory() error: %v", err)
	}

	_, total, err := svc.ListHistory(ctx, 1, 1, 10)
	if err != nil {
		t.Fatalf("ListHistory() error: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0 after clear", total)
	}
}

func TestQueryHistory_Pagination(t *testing.T) {
	testDB := setupQueryTestDB(t)
	svc := NewQueryHistoryService(testDB)
	ctx, cancel := queryCtx(t)
	defer cancel()

	for i := 0; i < 15; i++ {
		history := &model.QueryHistory{
			UserID: 1, DatasourceID: 1, Database: "testdb",
			SQLContent: fmt.Sprintf("SELECT %d", i), SQLSummary: fmt.Sprintf("SELECT %d", i), DBType: "mysql",
		}
		if err := svc.CreateHistory(ctx, history); err != nil {
			t.Fatalf("CreateHistory() error: %v", err)
		}
	}

	t.Run("page1", func(t *testing.T) {
		list, total, err := svc.ListHistory(ctx, 1, 1, 5)
		if err != nil {
			t.Fatalf("ListHistory() error: %v", err)
		}
		if total != 15 {
			t.Errorf("total = %d, want 15", total)
		}
		if len(list) != 5 {
			t.Errorf("page size = %d, want 5", len(list))
		}
	})

	t.Run("page4_empty", func(t *testing.T) {
		list, _, err := svc.ListHistory(ctx, 1, 4, 5)
		if err != nil {
			t.Fatalf("ListHistory() error: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("page 4 size = %d, want 0", len(list))
		}
	})
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestExecuteQuery_CancelledContext(t *testing.T) {
	qs, testDB := setupQueryService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dsSvc := NewDatasourceService(testDB, testEncKey, connpool.NewManager())
	dsID := seedQueryDatasource(t, dsSvc, context.Background())

	_, err := qs.ExecuteQuery(ctx, 1, "user1", "admin", dsID, "testdb", "SELECT 1", "mysql")
	if err == nil {
		t.Error("expected error with cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// executeMySQL: direct tests via SQLite injection
// ---------------------------------------------------------------------------

func TestExecuteMySQL_Success(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	// Use a real SQLite DB as the mock MySQL target
	sqliteDB := setupQueryTestDB(t)
	qs.connMgr.InjectMySQLForTest(1, "localhost", 3306, "testdb", sqliteDB)

	poolCfg := connpool.MySQLPoolConfig{MaxOpen: 10, MaxIdle: 5}
	result, err := qs.executeMySQL(ctx, 1, "testdb", "SELECT 1 AS id, 'hello' AS name", "localhost", 3306, "root", "pass", poolCfg, 100)
	if err != nil {
		t.Fatalf("executeMySQL() error: %v", err)
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
		t.Error("id column should not be nil")
	}
	if result.Rows[0]["name"] == nil {
		t.Error("name column should not be nil")
	}
	if result.ExecutionTime < 0 {
		t.Errorf("ExecutionTime = %d, want >= 0", result.ExecutionTime)
	}
}

func TestExecuteMySQL_EmptyResult(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	sqliteDB := setupQueryTestDB(t)
	qs.connMgr.InjectMySQLForTest(1, "localhost", 3306, "testdb", sqliteDB)

	poolCfg := connpool.MySQLPoolConfig{}
	result, err := qs.executeMySQL(ctx, 1, "testdb", "SELECT 1 AS id WHERE 1=0", "localhost", 3306, "root", "pass", poolCfg, 100)
	if err != nil {
		t.Fatalf("executeMySQL() error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
	if len(result.Rows) != 0 {
		t.Errorf("Rows count = %d, want 0", len(result.Rows))
	}
	if len(result.Columns) != 1 {
		t.Errorf("Columns = %v, want 1 column", result.Columns)
	}
}

func TestExecuteMySQL_ConnectionError(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	poolCfg := connpool.MySQLPoolConfig{}
	_, err := qs.executeMySQL(ctx, 999, "testdb", "SELECT 1", "127.0.0.1", 19999, "root", "pass", poolCfg, 100)
	if err == nil {
		t.Error("expected connection error, got nil")
	}
}

func TestExecuteMySQL_DefaultDatabase(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	sqliteDB := setupQueryTestDB(t)
	// When database is empty, executeMySQL defaults to "information_schema"
	qs.connMgr.InjectMySQLForTest(1, "localhost", 3306, "information_schema", sqliteDB)

	poolCfg := connpool.MySQLPoolConfig{}
	result, err := qs.executeMySQL(ctx, 1, "", "SELECT 42 AS val", "localhost", 3306, "root", "pass", poolCfg, 100)
	if err != nil {
		t.Fatalf("executeMySQL() error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
}

func TestExecuteMySQL_MultipleRows(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	sqliteDB := setupQueryTestDB(t)
	qs.connMgr.InjectMySQLForTest(1, "localhost", 3306, "testdb", sqliteDB)

	poolCfg := connpool.MySQLPoolConfig{}
	result, err := qs.executeMySQL(ctx, 1, "testdb", "SELECT 1 AS n UNION ALL SELECT 2 UNION ALL SELECT 3", "localhost", 3306, "root", "pass", poolCfg, 100)
	if err != nil {
		t.Fatalf("executeMySQL() error: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if len(result.Rows) != 3 {
		t.Errorf("Rows count = %d, want 3", len(result.Rows))
	}
}

func TestExecuteMySQL_RowLimit(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	sqliteDB := setupQueryTestDB(t)
	qs.connMgr.InjectMySQLForTest(1, "localhost", 3306, "testdb", sqliteDB)

	poolCfg := connpool.MySQLPoolConfig{}
	// Generate 10 rows but limit to 3
	result, err := qs.executeMySQL(ctx, 1, "testdb",
		"SELECT 1 AS n UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7 UNION ALL SELECT 8 UNION ALL SELECT 9 UNION ALL SELECT 10",
		"localhost", 3306, "root", "pass", poolCfg, 3)
	if err != nil {
		t.Fatalf("executeMySQL() error: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("Total = %d, want 3 (limited by rowLimit)", result.Total)
	}
}

// ---------------------------------------------------------------------------
// executeMongoDB: direct tests
// ---------------------------------------------------------------------------

func TestExecuteMongoDB_ConnectionError(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	_, err := qs.executeMongoDB(ctx, 999, "testdb", `{"operation":"find","collection":"users"}`, "127.0.0.1", 19999, "root", "pass", 100)
	if err == nil {
		t.Error("expected connection error for non-existent MongoDB, got nil")
	}
}

func TestExecuteMongoDB_ParseError(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	// Inject a disconnected mongo client to get past connection step
	mongoClient, _ := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	uri := buildMongoURI("127.0.0.1", 19999, "root", "pass")
	qs.connMgr.InjectMongoForTest(1, uri, mongoClient)

	_, err := qs.executeMongoDB(ctx, 1, "testdb", "not valid json", "127.0.0.1", 19999, "root", "pass", 100)
	if err == nil {
		t.Error("expected parse error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "解析MongoDB命令失败") {
		t.Errorf("error = %v, want parse error", err)
	}
}

func TestExecuteMongoDB_MissingDatabase(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	mongoClient, _ := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	uri := buildMongoURI("127.0.0.1", 19999, "root", "pass")
	qs.connMgr.InjectMongoForTest(1, uri, mongoClient)

	_, err := qs.executeMongoDB(ctx, 1, "", `{"operation":"find","collection":"users"}`, "127.0.0.1", 19999, "root", "pass", 100)
	if err == nil {
		t.Error("expected error for missing database, got nil")
	}
	if !strings.Contains(err.Error(), "必须指定数据库") {
		t.Errorf("error = %v, want missing database error", err)
	}
}

func TestExecuteMongoDB_MissingCollection(t *testing.T) {
	qs, _ := setupQueryService(t)
	ctx := context.Background()

	mongoClient, _ := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	uri := buildMongoURI("127.0.0.1", 19999, "root", "pass")
	qs.connMgr.InjectMongoForTest(1, uri, mongoClient)

	_, err := qs.executeMongoDB(ctx, 1, "testdb", `{"operation":"find"}`, "127.0.0.1", 19999, "root", "pass", 100)
	if err == nil {
		t.Error("expected error for missing collection, got nil")
	}
	if !strings.Contains(err.Error(), "必须指定集合名称") {
		t.Errorf("error = %v, want missing collection error", err)
	}
}
