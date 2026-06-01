package sqlparser

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ParsePostgreSQL — AST-based parsing
// ---------------------------------------------------------------------------

func TestParsePostgreSQL_Select(t *testing.T) {
	result, err := ParsePostgreSQL("SELECT * FROM users LIMIT 10")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
	if !equalStringSlices(result.Tables, []string{"users"}) {
		t.Errorf("Tables = %v, want [users]", result.Tables)
	}
	if !result.HasLimit {
		t.Error("HasLimit = false, want true")
	}
}

func TestParsePostgreSQL_SelectWithWhere(t *testing.T) {
	result, err := ParsePostgreSQL("SELECT id, name FROM users WHERE active = true")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
	if !result.HasWhere {
		t.Error("HasWhere = false, want true")
	}
}

func TestParsePostgreSQL_Insert(t *testing.T) {
	result, err := ParsePostgreSQL("INSERT INTO orders (id, total) VALUES (1, 99.99)")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDML {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDML)
	}
	if !equalStringSlices(result.Tables, []string{"orders"}) {
		t.Errorf("Tables = %v, want [orders]", result.Tables)
	}
}

func TestParsePostgreSQL_Update(t *testing.T) {
	result, err := ParsePostgreSQL("UPDATE users SET name = 'Alice' WHERE id = 1")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpUpdate {
		t.Errorf("Operation = %q, want %q", result.Operation, OpUpdate)
	}
	if !result.HasWhere {
		t.Error("HasWhere = false, want true")
	}
}

func TestParsePostgreSQL_Delete(t *testing.T) {
	result, err := ParsePostgreSQL("DELETE FROM logs WHERE created_at < '2024-01-01'")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDelete {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDelete)
	}
	if !result.HasWhere {
		t.Error("HasWhere = false, want true")
	}
}

func TestParsePostgreSQL_DropTable(t *testing.T) {
	result, err := ParsePostgreSQL("DROP TABLE users")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDDL {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDDL)
	}
	if !result.IsDropTable {
		t.Error("IsDropTable = false, want true")
	}
}

func TestParsePostgreSQL_DropDatabase(t *testing.T) {
	result, err := ParsePostgreSQL("DROP DATABASE production")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if !result.IsDropDatabase {
		t.Error("IsDropDatabase = false, want true")
	}
}

func TestParsePostgreSQL_Truncate(t *testing.T) {
	result, err := ParsePostgreSQL("TRUNCATE TABLE logs")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if !result.IsTruncate {
		t.Error("IsTruncate = false, want true")
	}
	if !equalStringSlices(result.Tables, []string{"logs"}) {
		t.Errorf("Tables = %v, want [logs]", result.Tables)
	}
}

func TestParsePostgreSQL_CreateTable(t *testing.T) {
	result, err := ParsePostgreSQL("CREATE TABLE products (id SERIAL PRIMARY KEY, name TEXT NOT NULL)")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDDL {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDDL)
	}
	if !equalStringSlices(result.Tables, []string{"products"}) {
		t.Errorf("Tables = %v, want [products]", result.Tables)
	}
}

func TestParsePostgreSQL_AlterTable(t *testing.T) {
	result, err := ParsePostgreSQL("ALTER TABLE users ADD COLUMN email VARCHAR(255)")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDDL {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDDL)
	}
	if !equalStringSlices(result.Tables, []string{"users"}) {
		t.Errorf("Tables = %v, want [users]", result.Tables)
	}
}

// ---------------------------------------------------------------------------
// PG-specific syntax
// ---------------------------------------------------------------------------

func TestParsePostgreSQL_Returning(t *testing.T) {
	result, err := ParsePostgreSQL("INSERT INTO users (name) VALUES ('Bob') RETURNING id")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if !result.HasReturning {
		t.Error("HasReturning = false, want true for RETURNING clause")
	}
}

func TestParsePostgreSQL_UpdateReturning(t *testing.T) {
	result, err := ParsePostgreSQL("UPDATE users SET name = 'Alice' WHERE id = 1 RETURNING *")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if !result.HasReturning {
		t.Error("HasReturning = false, want true")
	}
	if !result.HasWhere {
		t.Error("HasWhere = false, want true")
	}
}

func TestParsePostgreSQL_DeleteReturning(t *testing.T) {
	result, err := ParsePostgreSQL("DELETE FROM users WHERE id = 1 RETURNING id, name")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if !result.HasReturning {
		t.Error("HasReturning = false, want true")
	}
}

func TestParsePostgreSQL_ONCONFLICT(t *testing.T) {
	result, err := ParsePostgreSQL("INSERT INTO users (id, name) VALUES (1, 'Alice') ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDML {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDML)
	}
}

func TestParsePostgreSQL_ILIKE(t *testing.T) {
	// ILIKE is valid PG syntax — should parse
	result, err := ParsePostgreSQL("SELECT * FROM users WHERE name ILIKE '%alice%'")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
}

func TestParsePostgreSQL_JSONBOperators(t *testing.T) {
	// JSONB containment operator @>
	result, err := ParsePostgreSQL("SELECT * FROM orders WHERE metadata @> '{\"status\": \"active\"}'")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
}

func TestParsePostgreSQL_JSONBAccess(t *testing.T) {
	// JSONB -> operator
	result, err := ParsePostgreSQL("SELECT data->>'name' FROM events WHERE data->'type' = '\"click\"'")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
}

func TestParsePostgreSQL_CTE(t *testing.T) {
	result, err := ParsePostgreSQL("WITH active_users AS (SELECT id FROM users WHERE active = true) SELECT * FROM active_users")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
}

func TestParsePostgreSQL_RecursiveCTE(t *testing.T) {
	result, err := ParsePostgreSQL(`WITH RECURSIVE hierarchy AS (SELECT id, parent_id FROM org WHERE parent_id IS NULL UNION ALL SELECT o.id, o.parent_id FROM org o JOIN hierarchy h ON o.parent_id = h.id) SELECT * FROM hierarchy`)
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
}

func TestParsePostgreSQL_LimitOffset(t *testing.T) {
	result, err := ParsePostgreSQL("SELECT * FROM users LIMIT 10 OFFSET 20")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if !result.HasLimit {
		t.Error("HasLimit = false, want true")
	}
}

func TestParsePostgreSQL_OnlyKeyword(t *testing.T) {
	result, err := ParsePostgreSQL("SELECT * FROM ONLY users")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
	if !equalStringSlices(result.Tables, []string{"users"}) {
		t.Errorf("Tables = %v, want [users]", result.Tables)
	}
}

func TestParsePostgreSQL_Join(t *testing.T) {
	result, err := ParsePostgreSQL("SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE u.active = true")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if !equalStringSlices(result.Tables, []string{"users", "orders"}) {
		t.Errorf("Tables = %v, want [users orders]", result.Tables)
	}
}

func TestParsePostgreSQL_CreateIndex(t *testing.T) {
	result, err := ParsePostgreSQL("CREATE INDEX idx_users_email ON users (email)")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDDL {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDDL)
	}
	if !equalStringSlices(result.Tables, []string{"users"}) {
		t.Errorf("Tables = %v, want [users]", result.Tables)
	}
}

func TestParsePostgreSQL_CreateView(t *testing.T) {
	result, err := ParsePostgreSQL("CREATE VIEW active_users AS SELECT * FROM users WHERE active = true")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDDL {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDDL)
	}
}

func TestParsePostgreSQL_DropIndex(t *testing.T) {
	result, err := ParsePostgreSQL("DROP INDEX idx_users_email")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDDL {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDDL)
	}
}

// ---------------------------------------------------------------------------
// Unified ParseSQL — PostgreSQL path
// ---------------------------------------------------------------------------

func TestParseSQL_PostgreSQLUnified(t *testing.T) {
	tests := []struct {
		name        string
		sql         string
		wantBlocked bool
		wantRisk    RiskLevel
		wantOp      OperationType
	}{
		{"safe_select", "SELECT * FROM users LIMIT 10", false, RiskLow, OpSelect},
		{"drop_database_blocked", "DROP DATABASE production", true, RiskHigh, OpDDL},
		{"drop_table_blocked", "DROP TABLE users", true, RiskHigh, OpDDL},
		{"truncate_blocked", "TRUNCATE TABLE logs", true, RiskHigh, OpDDL},
		{"update_no_where_blocked", "UPDATE users SET name = 'x'", true, RiskHigh, OpUpdate},
		{"delete_no_where_blocked", "DELETE FROM logs", true, RiskHigh, OpDelete},
		{"insert_returning_safe", "INSERT INTO users (name) VALUES ('Bob') RETURNING id", false, RiskMedium, OpDML},
		{"update_with_where_safe", "UPDATE users SET name = 'x' WHERE id = 1", false, RiskMedium, OpUpdate},
		{"delete_with_where_safe", "DELETE FROM logs WHERE id = 1", false, RiskMedium, OpDelete},
		{"create_table_medium", "CREATE TABLE t (id SERIAL PRIMARY KEY)", false, RiskMedium, OpDDL},
		{"alter_table_medium", "ALTER TABLE t ADD COLUMN x INT", false, RiskMedium, OpDDL},
		{"on_conflict_insert", "INSERT INTO users (id, name) VALUES (1, 'A') ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name", false, RiskMedium, OpDML},
		{"cte_select", "WITH cte AS (SELECT 1) SELECT * FROM cte", false, RiskLow, OpSelect},
		{"select_no_limit_warning", "SELECT * FROM users", false, RiskLow, OpSelect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSQL(tt.sql, "postgresql")
			if err != nil {
				t.Fatalf("ParseSQL error: %v", err)
			}
			if result.DBType != "postgresql" {
				t.Errorf("DBType = %q, want postgresql", result.DBType)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
			if result.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v (BlockReason: %s)", result.IsBlocked, tt.wantBlocked, result.BlockReason)
			}
			if result.RiskLevel != tt.wantRisk {
				t.Errorf("RiskLevel = %q, want %q", result.RiskLevel, tt.wantRisk)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MySQL regression — ensure nothing broke
// ---------------------------------------------------------------------------

func TestParseSQL_MySQLStillWorksAfterPGChanges(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantOp  OperationType
		wantOK  bool
	}{
		{"select", "SELECT * FROM users LIMIT 10", OpSelect, true},
		{"insert", "INSERT INTO users (id) VALUES (1)", OpDML, true},
		{"update", "UPDATE users SET x = 1 WHERE id = 1", OpUpdate, true},
		{"delete", "DELETE FROM users WHERE id = 1", OpDelete, true},
		{"create", "CREATE TABLE t (id INT)", OpDDL, true},
		{"drop", "DROP TABLE t", OpDDL, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSQL(tt.sql, "mysql")
			if tt.wantOK && err != nil {
				t.Fatalf("ParseSQL(mysql) error: %v", err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Degradation: PG parser fails → UNKNOWN + no block
// ---------------------------------------------------------------------------

func TestParsePostgreSQL_Empty(t *testing.T) {
	_, err := ParsePostgreSQL("")
	if err == nil {
		t.Error("expected error for empty SQL")
	}
}

func TestParsePostgreSQL_Whitespace(t *testing.T) {
	_, err := ParsePostgreSQL("   ")
	if err == nil {
		t.Error("expected error for whitespace SQL")
	}
}

// ---------------------------------------------------------------------------
// Unified ParseSQL PG degradation path
// ---------------------------------------------------------------------------

func TestParseSQL_PostgreSQLDegradation(t *testing.T) {
	// Empty SQL should still error — even through degradation path
	_, err := ParseSQL("", "postgresql")
	if err == nil {
		t.Error("expected error for empty PG SQL via ParseSQL")
	}
}

// ---------------------------------------------------------------------------
// PG dbType aliases
// ---------------------------------------------------------------------------

func TestParseSQL_PGTypeAliases(t *testing.T) {
	aliases := []string{"postgresql", "postgres", "pg", "PostgreSQL", "PG"}
	for _, alias := range aliases {
		t.Run(alias, func(t *testing.T) {
			result, err := ParseSQL("SELECT 1", alias)
			if err != nil {
				t.Fatalf("ParseSQL(%q) error: %v", alias, err)
			}
			if result.DBType != "postgresql" {
				t.Errorf("DBType = %q, want postgresql for alias %q", result.DBType, alias)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// No-LIMIT warning for PG
// ---------------------------------------------------------------------------

func TestParseSQL_PGNoLimitWarning(t *testing.T) {
	result, err := ParseSQL("SELECT * FROM users", "postgresql")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "LIMIT") {
			found = true
		}
	}
	if !found {
		t.Error("expected LIMIT warning for SELECT without LIMIT")
	}
}

func TestParseSQL_PGLimitSuppressedWarning(t *testing.T) {
	result, err := ParseSQL("SELECT * FROM users LIMIT 10", "postgresql")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	for _, w := range result.Warnings {
		if strings.Contains(w, "LIMIT") {
			t.Errorf("did not expect LIMIT warning when LIMIT present, got: %q", w)
		}
	}
}

// ---------------------------------------------------------------------------
// extractTablesRegex — fallback
// ---------------------------------------------------------------------------

func TestExtractTablesRegex(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want []string
	}{
		{"select_from", "SELECT * FROM users", []string{"users"}},
		{"join", "SELECT * FROM users u JOIN orders o ON u.id = o.user_id", []string{"users", "orders"}},
		{"update", "UPDATE users SET x = 1", []string{"users"}},
		{"insert_into", "INSERT INTO orders VALUES (1)", []string{"orders"}},
		{"delete_from", "DELETE FROM logs", []string{"logs"}},
		{"create_table", "CREATE TABLE t (id INT)", []string{"t"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTablesRegex(tt.sql)
			if !equalStringSlices(got, tt.want) {
				t.Errorf("extractTablesRegex(%q) = %v, want %v", tt.sql, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PG-specific: JSONB operators (more cases)
// ---------------------------------------------------------------------------

func TestParsePostgreSQL_JSONBQuestionOperators(t *testing.T) {
	// ? operator: does the JSONB object contain the given key?
	result, err := ParsePostgreSQL("SELECT * FROM data WHERE tags ? 'important'")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
}

func TestParsePostgreSQL_JSONBPipeOperators(t *testing.T) {
	// ?| operator: match any key
	result, err := ParsePostgreSQL("SELECT * FROM data WHERE tags ?| array['a', 'b']")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
}

func TestParsePostgreSQL_GrantStmt(t *testing.T) {
	result, err := ParsePostgreSQL("GRANT SELECT ON users TO analyst")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDDL {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDDL)
	}
}

func TestParsePostgreSQL_BeginTransaction(t *testing.T) {
	result, err := ParsePostgreSQL("BEGIN")
	if err != nil {
		t.Fatalf("ParsePostgreSQL error: %v", err)
	}
	if result.Operation != OpDDL {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDDL)
	}
}
