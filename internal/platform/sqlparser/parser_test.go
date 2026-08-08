package sqlparser

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Unified ParseSQL Tests — MySQL
// ---------------------------------------------------------------------------

func TestParseSQL_MySQL(t *testing.T) {
	tests := []struct {
		name        string
		sql         string
		wantBlocked bool
		wantRisk    RiskLevel
	}{
		{"safe select", "SELECT * FROM users LIMIT 10", false, RiskLow},
		{"parameterized select", "SELECT * FROM users WHERE id = ?", false, RiskLow},
		{"drop database blocked", "DROP DATABASE production", true, RiskHigh},
		{"drop table blocked", "DROP TABLE users", true, RiskHigh},
		{"truncate blocked", "TRUNCATE TABLE users", true, RiskHigh},
		{"update without where blocked", "UPDATE users SET active = 0", true, RiskHigh},
		{"delete without where blocked", "DELETE FROM logs", true, RiskHigh},
		{"update with where allowed", "UPDATE users SET name = 'x' WHERE id = 1", false, RiskMedium},
		{"delete with where allowed", "DELETE FROM logs WHERE id = 1", false, RiskMedium},
		{"insert_medium_risk", "INSERT INTO users (id) VALUES (1)", false, RiskMedium},
		{"create_table_medium", "CREATE TABLE t (id INT)", false, RiskMedium},
		{"alter_table_medium", "ALTER TABLE t ADD COLUMN x INT", false, RiskMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQLDialect(tt.sql)
			if err != nil {
				t.Fatalf("ParseSQL error: %v", err)
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

func TestParseSQL_PostgreSQLParameterizedSelect(t *testing.T) {
	result, err := ParsePostgreSQLDialect("SELECT * FROM users WHERE id = $1 AND status = $2")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Fatalf("Operation = %q, want %q", result.Operation, OpSelect)
	}
	if len(result.Tables) != 1 || result.Tables[0] != "users" {
		t.Fatalf("Tables = %#v, want users", result.Tables)
	}
}

// ---------------------------------------------------------------------------
// Unified ParseSQL Tests — MongoDB
// ---------------------------------------------------------------------------

func TestParseSQL_MongoDB(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantBlocked bool
		wantRisk    RiskLevel
	}{
		{"safe find", `{"operation": "find", "collection": "users", "filter": {"active": true}}`, false, RiskLow},
		{"updateMany empty blocked", `{"operation": "update", "collection": "users", "multi": true, "filter": {}, "update": {"$set": {"x": 1}}}`, true, RiskHigh},
		{"dangerous aggregate blocked", `{"operation": "aggregate", "collection": "users", "pipeline": [{"$out": "backup"}]}`, true, RiskHigh},
		{"safe aggregate", `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$match": {"status": "active"}}]}`, false, RiskLow},
		{"delete empty blocked", `{"operation": "delete", "collection": "logs", "filter": {}}`, true, RiskHigh},
		{"update_one_safe", `{"operation": "update", "collection": "users", "filter": {"id": 1}, "update": {"$set": {"x": 1}}}`, false, RiskMedium},
		{"delete_with_filter_safe", `{"operation": "delete", "collection": "logs", "filter": {"id": 1}}`, false, RiskMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongoDialect(tt.body)
			if err != nil {
				t.Fatalf("ParseSQL error: %v", err)
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
// ParseSQL — Case-insensitive dbType

// ---------------------------------------------------------------------------
// ParseSQL — Unsupported dbType

// ---------------------------------------------------------------------------
// ParseSQL — MySQL error inputs
// ---------------------------------------------------------------------------

func TestParseSQL_MySQLErrors(t *testing.T) {
	t.Run("empty_sql", func(t *testing.T) {
		_, err := ParseMySQLDialect("")
		if err == nil {
			t.Error("expected error for empty SQL")
		}
	})

	t.Run("whitespace_sql", func(t *testing.T) {
		_, err := ParseMySQLDialect("   ")
		if err == nil {
			t.Error("expected error for whitespace SQL")
		}
	})

	t.Run("invalid_syntax", func(t *testing.T) {
		_, err := ParseMySQLDialect("NOT VALID SQL AT ALL !!!")
		if err == nil {
			t.Error("expected error for invalid SQL syntax")
		}
	})
}

// ---------------------------------------------------------------------------
// ParseSQL — MySQL tables from JOINs in unified path
// ---------------------------------------------------------------------------

func TestParseSQL_MySQLJoinTables(t *testing.T) {
	result, err := ParseMySQLDialect("SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE u.active = 1 LIMIT 10")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if !equalStringSlices(result.Tables, []string{"users", "orders"}) {
		t.Errorf("Tables = %v, want [users orders]", result.Tables)
	}
}

// ---------------------------------------------------------------------------
// ParseSQL — No LIMIT Warning
// ---------------------------------------------------------------------------

func TestParseSQL_NoLimitWarning(t *testing.T) {
	result, err := ParseMySQLDialect("SELECT * FROM users")
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

func TestParseSQL_LimitSuppressedWarning(t *testing.T) {
	result, err := ParseMySQLDialect("SELECT * FROM users LIMIT 10")
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
// ParseSQL — BlockReason messages
// ---------------------------------------------------------------------------

func TestParseSQL_BlockReasonMessages(t *testing.T) {
	// Each row names the parser it belongs to. The table used to carry a dbType
	// string and hand it to the dispatcher; with the dispatcher gone there is no
	// name to dispatch on, and a row that picks its own parser cannot be routed
	// to the wrong one by a typo.
	tests := []struct {
		name       string
		parse      func(string) (*SQLParseResult, error)
		sql        string
		wantReason string
	}{
		{"drop_database", ParseMySQLDialect, "DROP DATABASE db", "DROP DATABASE is not allowed"},
		{"drop_table", ParseMySQLDialect, "DROP TABLE t", "DROP TABLE is not allowed"},
		{"truncate", ParseMySQLDialect, "TRUNCATE TABLE t", "TRUNCATE is not allowed"},
		{"update_no_where", ParseMySQLDialect, "UPDATE t SET x = 1", "UPDATE without WHERE clause is not allowed"},
		{"delete_no_where", ParseMySQLDialect, "DELETE FROM t", "DELETE without WHERE clause is not allowed"},
		{"mongo_update_many_empty", ParseMongoDialect, `{"operation": "update", "collection": "t", "multi": true, "filter": {}, "update": {"$set": {"x": 1}}}`, "updateMany with empty filter is not allowed"},
		{"mongo_delete_empty", ParseMongoDialect, `{"operation": "delete", "collection": "t", "filter": {}}`, "delete with empty filter is not allowed"},
		{"mongo_dangerous_agg", ParseMongoDialect, `{"operation": "aggregate", "collection": "t", "pipeline": [{"$out": "x"}]}`, "aggregation contains dangerous pipeline stage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.parse(tt.sql)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if !result.IsBlocked {
				t.Fatalf("expected the statement to be blocked")
			}
			if !strings.Contains(result.BlockReason, tt.wantReason) {
				t.Errorf("BlockReason = %q, want it to contain %q", result.BlockReason, tt.wantReason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseSQL — MongoDB Operation/Tables mapping
// ---------------------------------------------------------------------------

func TestParseSQL_MongoOperationMapping(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		wantOp OperationType
	}{
		{"find_to_select", `{"operation": "find", "filter": {"x": 1}}`, OpSelect},
		{"update_to_update", `{"operation": "update", "filter": {"x": 1}, "update": {"$set": {"y": 2}}}`, OpUpdate},
		{"delete_to_delete", `{"operation": "delete", "filter": {"x": 1}}`, OpDelete},
		{"aggregate_to_select", `{"operation": "aggregate", "pipeline": [{"$match": {}}]}`, OpSelect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongoDialect(tt.body)
			if err != nil {
				t.Fatalf("ParseSQL error: %v", err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
		})
	}
}

func TestParseSQL_MongoTablesMapping(t *testing.T) {
	t.Run("collection_mapped_to_tables", func(t *testing.T) {
		result, err := ParseMongoDialect(`{"operation": "find", "collection": "users", "filter": {"x": 1}}`)
		if err != nil {
			t.Fatalf("ParseSQL error: %v", err)
		}
		if !equalStringSlices(result.Tables, []string{"users"}) {
			t.Errorf("Tables = %v, want [users]", result.Tables)
		}
	})

	t.Run("no_collection_empty_tables", func(t *testing.T) {
		result, err := ParseMongoDialect(`{"operation": "find", "filter": {"x": 1}}`)
		if err != nil {
			t.Fatalf("ParseSQL error: %v", err)
		}
		if len(result.Tables) != 0 {
			t.Errorf("Tables = %v, want empty", result.Tables)
		}
	})
}

// ---------------------------------------------------------------------------
// Legacy Functions: ExtractOperation
// ---------------------------------------------------------------------------

func TestIsSQLKeyword(t *testing.T) {
	keywords := []string{"select", "from", "where", "and", "or", "not", "in", "exists",
		"between", "like", "is", "null", "true", "false", "as", "on", "join",
		"left", "right", "inner", "outer", "cross", "group", "by", "having",
		"order", "limit", "offset", "union", "all", "distinct", "set", "values",
		"into", "case", "when", "then", "else", "end", "dual"}
	for _, kw := range keywords {
		t.Run(kw, func(t *testing.T) {
			if !isSQLKeyword(kw) {
				t.Errorf("isSQLKeyword(%q) = false, want true", kw)
			}
		})
	}

	nonKeywords := []string{"users", "orders", "products", "mytable", "id", "name"}
	for _, nk := range nonKeywords {
		t.Run("non_"+nk, func(t *testing.T) {
			if isSQLKeyword(nk) {
				t.Errorf("isSQLKeyword(%q) = true, want false", nk)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseMongo — empty/invalid body
// ---------------------------------------------------------------------------

func TestParseMongo_EmptyBody(t *testing.T) {
	_, err := ParseMongo("")
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestParseMongo_InvalidJSON(t *testing.T) {
	_, err := ParseMongo("{invalid}")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// Supplementary: ParseSQL MongoDB error paths via unified entry
// ---------------------------------------------------------------------------

func TestParseSQL_MongoDBErrorPaths(t *testing.T) {
	t.Run("empty_body", func(t *testing.T) {
		_, err := ParseMongoDialect("")
		if err == nil {
			t.Error("expected error for empty MongoDB body via ParseSQL")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		_, err := ParseMongoDialect("{invalid}")
		if err == nil {
			t.Error("expected error for invalid JSON via ParseSQL")
		}
	})

	t.Run("whitespace_body", func(t *testing.T) {
		_, err := ParseMongoDialect("   ")
		if err == nil {
			t.Error("expected error for whitespace-only body via ParseSQL")
		}
	})
}

// ---------------------------------------------------------------------------
// Supplementary: ParseSQL "mongo" alias
// ---------------------------------------------------------------------------

func TestParseSQL_MongoAlias(t *testing.T) {
	result, err := ParseMongoDialect(`{"operation": "find", "collection": "users", "filter": {"x": 1}}`)
	if err != nil {
		t.Fatalf("ParseSQL with 'mongo' alias error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q", result.Operation, OpSelect)
	}
	if !equalStringSlices(result.Tables, []string{"users"}) {
		t.Errorf("Tables = %v, want [users]", result.Tables)
	}
}

// ---------------------------------------------------------------------------
// Supplementary: ParseSQL MySQL tables/operation consistency via unified path
// ---------------------------------------------------------------------------

func TestParseSQL_MySQLTablesAndOperationConsistency(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantOp     OperationType
		wantTables []string
	}{
		{"select_single_table", "SELECT id FROM users", OpSelect, []string{"users"}},
		{"select_no_tables", "SELECT 1", OpSelect, nil},
		{"insert_table", "INSERT INTO orders (id) VALUES (1)", OpDML, []string{"orders"}},
		{"update_table", "UPDATE products SET price = 10 WHERE id = 1", OpUpdate, []string{"products"}},
		{"delete_table", "DELETE FROM logs WHERE id < 100", OpDelete, []string{"logs"}},
		{"create_table", "CREATE TABLE t (id INT)", OpDDL, []string{"t"}},
		{"drop_table", "DROP TABLE t", OpDDL, []string{"t"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQLDialect(tt.sql)
			if err != nil {
				t.Fatalf("ParseSQL error: %v", err)
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
// Supplementary: ExtractOperation fallback (unparseable SQL → regex)
// ---------------------------------------------------------------------------

func TestAppendIfAbsent(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		s     string
		want  []string
	}{
		{"add_to_empty", nil, "users", []string{"users"}},
		{"add_new_element", []string{"users"}, "orders", []string{"users", "orders"}},
		{"skip_duplicate", []string{"users"}, "users", []string{"users"}},
		{"skip_duplicate_case_insensitive", []string{"Users"}, "users", []string{"Users"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendIfAbsent(tt.slice, tt.s)
			if !equalStringSlices(got, tt.want) {
				t.Errorf("appendIfAbsent(%v, %q) = %v, want %v", tt.slice, tt.s, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Supplementary: ParseSQL MySQL default risk for DML operations
// ---------------------------------------------------------------------------

func TestParseSQL_MySQLDefaultRiskDML(t *testing.T) {
	// INSERT is DML — should be RiskMedium and not blocked
	result, err := ParseMySQLDialect("INSERT INTO users (id, name) VALUES (1, 'test')")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if result.RiskLevel != RiskMedium {
		t.Errorf("RiskLevel = %q, want %q", result.RiskLevel, RiskMedium)
	}
	if result.IsBlocked {
		t.Errorf("IsBlocked = true, want false for safe INSERT")
	}
}

// ---------------------------------------------------------------------------
// Supplementary: ParseSQL MongoDB find default risk
// ---------------------------------------------------------------------------

func TestParseSQL_MongoDBFindDefaultRisk(t *testing.T) {
	result, err := ParseMongoDialect(`{"operation": "find", "collection": "users", "filter": {"active": true}}`)
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if result.RiskLevel != RiskLow {
		t.Errorf("RiskLevel = %q, want %q", result.RiskLevel, RiskLow)
	}
	if result.IsBlocked {
		t.Errorf("IsBlocked = true, want false for safe find")
	}
}

// ---------------------------------------------------------------------------
// Supplementary: ParseSQL MySQL warnings not present on non-SELECT
// ---------------------------------------------------------------------------

func TestParseSQL_NoWarningOnNonSelect(t *testing.T) {
	result, err := ParseMySQLDialect("UPDATE users SET name = 'x' WHERE id = 1")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty for UPDATE with WHERE", result.Warnings)
	}
}

// ---------------------------------------------------------------------------
// Supplementary: GetRiskLevel — comprehensive table
// ---------------------------------------------------------------------------
