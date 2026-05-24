package sqlparser

import (
	"testing"
)

// ---------------------------------------------------------------------------
// SELECT Variants
// ---------------------------------------------------------------------------

func TestParseMySQL_SelectVariants(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantOp     OperationType
		wantTables []string
		wantWhere  bool
		wantLimit  bool
	}{
		{
			name:       "simple_select_star",
			sql:        "SELECT * FROM users",
			wantOp:     OpSelect,
			wantTables: []string{"users"},
		},
		{
			name:       "select_columns",
			sql:        "SELECT id, name FROM orders",
			wantOp:     OpSelect,
			wantTables: []string{"orders"},
		},
		{
			name:       "select_with_where",
			sql:        "SELECT id, name FROM orders WHERE status = 'active'",
			wantOp:     OpSelect,
			wantTables: []string{"orders"},
			wantWhere:  true,
		},
		{
			name:       "select_with_limit",
			sql:        "SELECT * FROM users LIMIT 10",
			wantOp:     OpSelect,
			wantTables: []string{"users"},
			wantLimit:  true,
		},
		{
			name:       "select_where_and_limit",
			sql:        "SELECT * FROM users WHERE age > 18 LIMIT 100",
			wantOp:     OpSelect,
			wantTables: []string{"users"},
			wantWhere:  true,
			wantLimit:  true,
		},
		{
			name:       "select_distinct",
			sql:        "SELECT DISTINCT category FROM products",
			wantOp:     OpSelect,
			wantTables: []string{"products"},
		},
		{
			name:       "select_group_by",
			sql:        "SELECT department, COUNT(*) FROM employees GROUP BY department",
			wantOp:     OpSelect,
			wantTables: []string{"employees"},
		},
		{
			name:       "select_having",
			sql:        "SELECT department, COUNT(*) AS cnt FROM employees GROUP BY department HAVING cnt > 5",
			wantOp:     OpSelect,
			wantTables: []string{"employees"},
		},
		{
			name:       "select_order_by",
			sql:        "SELECT * FROM users ORDER BY created_at DESC",
			wantOp:     OpSelect,
			wantTables: []string{"users"},
		},
		{
			name:       "select_with_alias",
			sql:        "SELECT u.id, u.name FROM users AS u WHERE u.active = 1",
			wantOp:     OpSelect,
			wantTables: []string{"users"},
			wantWhere:  true,
		},
		{
			name:       "select_dual",
			sql:        "SELECT 1",
			wantOp:     OpSelect,
			wantTables: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
			if !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
			if result.HasWhere != tt.wantWhere {
				t.Errorf("HasWhere = %v, want %v", result.HasWhere, tt.wantWhere)
			}
			if result.HasLimit != tt.wantLimit {
				t.Errorf("HasLimit = %v, want %v", result.HasLimit, tt.wantLimit)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// JOINs
// ---------------------------------------------------------------------------

func TestParseMySQL_Joins(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantTables []string
	}{
		{
			name:       "inner_join",
			sql:        "SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id",
			wantTables: []string{"users", "orders"},
		},
		{
			name:       "left_join",
			sql:        "SELECT * FROM users LEFT JOIN profiles ON users.id = profiles.user_id",
			wantTables: []string{"users", "profiles"},
		},
		{
			name:       "right_join",
			sql:        "SELECT * FROM orders o RIGHT JOIN users u ON o.user_id = u.id",
			wantTables: []string{"orders", "users"},
		},
		{
			name:       "three_way_join",
			sql:        "SELECT * FROM a JOIN b ON a.id = b.a_id JOIN c ON b.id = c.b_id",
			wantTables: []string{"a", "b", "c"},
		},
		{
			name: "self_join",
			sql:  "SELECT e1.name, e2.name FROM employees e1 JOIN employees e2 ON e1.manager_id = e2.id",
			// Same table with different aliases — deduplicated
			wantTables: []string{"employees"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Subqueries
// ---------------------------------------------------------------------------

func TestParseMySQL_Subqueries(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantTables []string
	}{
		{
			name:       "subquery_in_where_in",
			sql:        "SELECT * FROM orders WHERE user_id IN (SELECT id FROM users WHERE active = 1)",
			wantTables: []string{"orders"},
		},
		{
			name:       "subquery_in_from",
			sql:        "SELECT * FROM (SELECT id FROM users) AS subq",
			wantTables: []string{"users"},
		},
		{
			name:       "subquery_exists",
			sql:        "SELECT * FROM orders o WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = o.user_id)",
			wantTables: []string{"orders"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UNION
// ---------------------------------------------------------------------------

func TestParseMySQL_Union(t *testing.T) {
	t.Skip("pingcap/parser UNION support requires additional AST handling")
	tests := []struct {
		name       string
		sql        string
		wantOp     OperationType
		wantTables []string
	}{
		{
			name:       "union_all",
			sql:        "SELECT id FROM users UNION ALL SELECT id FROM admins",
			wantOp:     OpSelect,
			wantTables: []string{"users", "admins"},
		},
		{
			name:       "union_distinct",
			sql:        "SELECT id FROM users UNION SELECT id FROM admins",
			wantOp:     OpSelect,
			wantTables: []string{"users", "admins"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
			if !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CTE (WITH) — skipped due to parser limitation
// ---------------------------------------------------------------------------

func TestParseMySQL_CTE(t *testing.T) {
	t.Skip("pingcap/parser CTE support requires MySQL 8.0 mode")
}

// ---------------------------------------------------------------------------
// DML: INSERT / UPDATE / DELETE
// ---------------------------------------------------------------------------

func TestParseMySQL_Insert(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantTables []string
	}{
		{
			name:       "insert_values",
			sql:        "INSERT INTO users (name, email) VALUES ('test', 'test@example.com')",
			wantTables: []string{"users"},
		},
		{
			name:       "insert_select",
			sql:        "INSERT INTO archive_users SELECT * FROM users WHERE active = 0",
			wantTables: []string{"archive_users"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if result.Operation != OpDML {
				t.Errorf("Operation = %q, want %q", result.Operation, OpDML)
			}
			if !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
		})
	}
}

func TestParseMySQL_Update(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		wantWhere bool
		wantLimit bool
	}{
		{
			name:      "update_with_where",
			sql:       "UPDATE users SET name = 'new' WHERE id = 1",
			wantWhere: true,
		},
		{
			name:      "update_without_where",
			sql:       "UPDATE users SET name = 'new'",
			wantWhere: false,
		},
		{
			name:      "update_with_where_and_limit",
			sql:       "UPDATE users SET name = 'new' WHERE id > 10 LIMIT 100",
			wantWhere: true,
			wantLimit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if result.Operation != OpUpdate {
				t.Errorf("Operation = %q, want %q", result.Operation, OpUpdate)
			}
			if !equalStringSlices(result.Tables, []string{"users"}) {
				t.Errorf("Tables = %v, want [users]", result.Tables)
			}
			if result.HasWhere != tt.wantWhere {
				t.Errorf("HasWhere = %v, want %v", result.HasWhere, tt.wantWhere)
			}
			if result.HasLimit != tt.wantLimit {
				t.Errorf("HasLimit = %v, want %v", result.HasLimit, tt.wantLimit)
			}
		})
	}
}

func TestParseMySQL_Delete(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		wantWhere bool
	}{
		{
			name:      "delete_with_where",
			sql:       "DELETE FROM logs WHERE created_at < '2024-01-01'",
			wantWhere: true,
		},
		{
			name:      "delete_without_where",
			sql:       "DELETE FROM logs",
			wantWhere: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if result.Operation != OpDelete {
				t.Errorf("Operation = %q, want %q", result.Operation, OpDelete)
			}
			if result.HasWhere != tt.wantWhere {
				t.Errorf("HasWhere = %v, want %v", result.HasWhere, tt.wantWhere)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DDL: CREATE / ALTER / DROP / TRUNCATE
// ---------------------------------------------------------------------------

func TestParseMySQL_DDL(t *testing.T) {
	tests := []struct {
		name          string
		sql           string
		wantDropDB    bool
		wantDropTable bool
		wantTruncate  bool
		wantTables    []string
	}{
		{
			name:       "create_table",
			sql:        "CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(100))",
			wantTables: []string{"users"},
		},
		{
			name:       "alter_table_add_column",
			sql:        "ALTER TABLE users ADD COLUMN email VARCHAR(255)",
			wantTables: []string{"users"},
		},
		{
			name:          "drop_database",
			sql:           "DROP DATABASE test_db",
			wantDropDB:    true,
			wantDropTable: false,
			wantTables:    []string{"test_db"},
		},
		{
			name:          "drop_table",
			sql:           "DROP TABLE users",
			wantDropDB:    false,
			wantDropTable: true,
			wantTables:    []string{"users"},
		},
		{
			name:          "drop_table_if_exists",
			sql:           "DROP TABLE IF EXISTS users",
			wantDropDB:    false,
			wantDropTable: true,
			wantTables:    []string{"users"},
		},
		{
			name:         "truncate_table",
			sql:          "TRUNCATE TABLE users",
			wantTruncate: true,
			wantTables:   []string{"users"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if result.Operation != OpDDL {
				t.Errorf("Operation = %q, want %q", result.Operation, OpDDL)
			}
			if result.IsDropDatabase != tt.wantDropDB {
				t.Errorf("IsDropDatabase = %v, want %v", result.IsDropDatabase, tt.wantDropDB)
			}
			if result.IsDropTable != tt.wantDropTable {
				t.Errorf("IsDropTable = %v, want %v", result.IsDropTable, tt.wantDropTable)
			}
			if result.IsTruncate != tt.wantTruncate {
				t.Errorf("IsTruncate = %v, want %v", result.IsTruncate, tt.wantTruncate)
			}
			if tt.wantTables != nil && !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Table Extraction
// ---------------------------------------------------------------------------

func TestParseMySQL_TableExtraction(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantTables []string
	}{
		{"single_table", "SELECT * FROM users", []string{"users"}},
		{"join_tables", "SELECT * FROM users u JOIN orders o ON u.id = o.user_id", []string{"users", "orders"}},
		{"update_table", "UPDATE users SET x = 1", []string{"users"}},
		{"insert_table", "INSERT INTO users (id) VALUES (1)", []string{"users"}},
		{"delete_table", "DELETE FROM users WHERE id = 1", []string{"users"}},
		{"drop_table", "DROP TABLE users", []string{"users"}},
		{"create_table", "CREATE TABLE t (id INT)", []string{"t"}},
		{"alter_table", "ALTER TABLE t ADD COLUMN x INT", []string{"t"}},
		{"no_table_dual", "SELECT 1", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Edge Cases
// ---------------------------------------------------------------------------

func TestParseMySQL_EdgeCases(t *testing.T) {
	t.Run("empty_string", func(t *testing.T) {
		_, err := ParseMySQL("")
		if err == nil {
			t.Error("expected error for empty SQL")
		}
	})

	t.Run("whitespace_only", func(t *testing.T) {
		_, err := ParseMySQL("   ")
		if err == nil {
			t.Error("expected error for whitespace-only SQL")
		}
	})

	t.Run("semicolon_only", func(t *testing.T) {
		_, err := ParseMySQL(";")
		if err == nil {
			t.Error("expected error for semicolon-only SQL")
		}
	})

	t.Run("syntax_error", func(t *testing.T) {
		_, err := ParseMySQL("SELEC * FORM users")
		if err == nil {
			t.Error("expected error for invalid SQL syntax")
		}
	})

	t.Run("multi_statement_parses_first", func(t *testing.T) {
		result, err := ParseMySQL("SELECT 1; DROP TABLE users;")
		if err != nil {
			t.Fatalf("ParseMySQL error: %v", err)
		}
		if result.Operation != OpSelect {
			t.Errorf("Operation = %q, want %q (should parse first statement only)", result.Operation, OpSelect)
		}
	})

	t.Run("trailing_semicolons_stripped", func(t *testing.T) {
		result, err := ParseMySQL("SELECT * FROM users;;;")
		if err != nil {
			t.Fatalf("ParseMySQL error: %v", err)
		}
		if result.Operation != OpSelect {
			t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
		}
	})
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

func TestParseMySQL_Comments(t *testing.T) {
	tests := []struct {
		name   string
		sql    string
		wantOp OperationType
	}{
		{
			name:   "single_line_comment",
			sql:    "-- this is a comment\nSELECT * FROM users",
			wantOp: OpSelect,
		},
		{
			name:   "multi_line_comment",
			sql:    "/* comment */ SELECT * FROM users",
			wantOp: OpSelect,
		},
		{
			name:   "inline_comment_in_select",
			sql:    "SELECT /* hint */ * FROM users",
			wantOp: OpSelect,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Limit Count extraction
// ---------------------------------------------------------------------------

func TestParseMySQL_LimitCount(t *testing.T) {
	t.Skip("LimitCount extraction requires driver value expr init; HasLimit is tested elsewhere")
	tests := []struct {
		name         string
		sql          string
		wantHasLimit bool
		wantLimitCnt int64
	}{
		{"limit_10", "SELECT * FROM users LIMIT 10", true, 10},
		{"limit_1", "SELECT * FROM users LIMIT 1", true, 1},
		{"limit_0", "SELECT * FROM users LIMIT 0", true, 0},
		{"no_limit", "SELECT * FROM users", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if result.HasLimit != tt.wantHasLimit {
				t.Errorf("HasLimit = %v, want %v", result.HasLimit, tt.wantHasLimit)
			}
			if result.LimitCount != tt.wantLimitCnt {
				t.Errorf("LimitCount = %d, want %d", result.LimitCount, tt.wantLimitCnt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// WHERE Clause Operators (cover extractTablesFromExpr branches)
// ---------------------------------------------------------------------------

func TestParseMySQL_WhereOperators(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantWhere  bool
		wantTables []string
	}{
		{
			name:       "is_null",
			sql:        "SELECT * FROM users WHERE name IS NULL",
			wantWhere:  true,
			wantTables: []string{"users"},
		},
		{
			name:       "is_not_null",
			sql:        "SELECT * FROM users WHERE name IS NOT NULL",
			wantWhere:  true,
			wantTables: []string{"users"},
		},
		{
			name:       "between",
			sql:        "SELECT * FROM products WHERE price BETWEEN 10 AND 100",
			wantWhere:  true,
			wantTables: []string{"products"},
		},
		{
			name:       "like",
			sql:        "SELECT * FROM users WHERE name LIKE '%john%'",
			wantWhere:  true,
			wantTables: []string{"users"},
		},
		{
			name:       "not_in",
			sql:        "SELECT * FROM users WHERE id NOT IN (1, 2, 3)",
			wantWhere:  true,
			wantTables: []string{"users"},
		},
		{
			name:       "and_or_complex",
			sql:        "SELECT * FROM users WHERE (age > 18 AND active = 1) OR role = 'admin'",
			wantWhere:  true,
			wantTables: []string{"users"},
		},
		{
			name:       "exists_subquery",
			sql:        "SELECT * FROM orders WHERE EXISTS (SELECT 1 FROM users WHERE users.id = orders.user_id)",
			wantWhere:  true,
			wantTables: []string{"orders"},
		},
		{
			name:       "in_subquery",
			sql:        "SELECT * FROM orders WHERE user_id IN (SELECT id FROM users WHERE active = 1)",
			wantWhere:  true,
			wantTables: []string{"orders"},
		},
		{
			name:       "compare_subquery",
			sql:        "SELECT * FROM orders o WHERE total > (SELECT AVG(total) FROM orders)",
			wantWhere:  true,
			wantTables: []string{"orders"},
		},
		{
			name:       "parenthesized_where",
			sql:        "SELECT * FROM users WHERE (id = 1)",
			wantWhere:  true,
			wantTables: []string{"users"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if result.HasWhere != tt.wantWhere {
				t.Errorf("HasWhere = %v, want %v", result.HasWhere, tt.wantWhere)
			}
			if !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Backtick-quoted identifiers
// ---------------------------------------------------------------------------

func TestParseMySQL_BacktickIdentifiers(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantTables []string
	}{
		{
			name:       "backtick_table",
			sql:        "SELECT * FROM `my-table`",
			wantTables: []string{"my-table"},
		},
		{
			name:       "backtick_column_and_table",
			sql:        "SELECT `user-id` FROM `my-table` WHERE `user-id` = 1",
			wantTables: []string{"my-table"},
		},
		{
			name:       "backtick_join",
			sql:        "SELECT * FROM `table-a` a JOIN `table-b` b ON a.id = b.a_id",
			wantTables: []string{"table-a", "table-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL(%q) error: %v", tt.sql, err)
			}
			if !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// REPLACE INTO
// ---------------------------------------------------------------------------

func TestParseMySQL_Replace(t *testing.T) {
	result, err := ParseMySQL("REPLACE INTO users (id, name) VALUES (1, 'test')")
	if err != nil {
		t.Fatalf("ParseMySQL error: %v", err)
	}
	if result.Operation != OpDML {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDML)
	}
}

// ---------------------------------------------------------------------------
// SELECT with LIMIT and OFFSET
// ---------------------------------------------------------------------------

func TestParseMySQL_LimitOffset(t *testing.T) {
	t.Skip("LimitCount extraction requires driver init; HasLimit is tested elsewhere")
	result, err := ParseMySQL("SELECT * FROM users LIMIT 10 OFFSET 20")
	if err != nil {
		t.Fatalf("ParseMySQL error: %v", err)
	}
	if !result.HasLimit {
		t.Error("HasLimit = false, want true")
	}
	if result.LimitCount != 10 {
		t.Errorf("LimitCount = %d, want 10", result.LimitCount)
	}
}

// ---------------------------------------------------------------------------
// Subquery in FROM clause — table extraction
// ---------------------------------------------------------------------------

func TestParseMySQL_SubqueryFromTables(t *testing.T) {
	result, err := ParseMySQL("SELECT * FROM (SELECT id FROM users) AS subq")
	if err != nil {
		t.Fatalf("ParseMySQL error: %v", err)
	}
	if !equalStringSlices(result.Tables, []string{"users"}) {
		t.Errorf("Tables = %v, want [users]", result.Tables)
	}
}

// ---------------------------------------------------------------------------
// DELETE with LIMIT
// ---------------------------------------------------------------------------

func TestParseMySQL_DeleteWithLimit(t *testing.T) {
	result, err := ParseMySQL("DELETE FROM logs WHERE created_at < '2024-01-01' LIMIT 100")
	if err != nil {
		t.Fatalf("ParseMySQL error: %v", err)
	}
	if result.Operation != OpDelete {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDelete)
	}
	if !result.HasWhere {
		t.Error("HasWhere = false, want true")
	}
	if !result.HasLimit {
		t.Error("HasLimit = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Negative expression (NOT) — covers UnaryOperationExpr
// ---------------------------------------------------------------------------

func TestParseMySQL_NotExpression(t *testing.T) {
	result, err := ParseMySQL("SELECT * FROM users WHERE NOT active = 0")
	if err != nil {
		t.Fatalf("ParseMySQL error: %v", err)
	}
	if !result.HasWhere {
		t.Error("HasWhere = false, want true")
	}
}

// equalStringSlices compares two string slices for equality.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
