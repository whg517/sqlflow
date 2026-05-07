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
			result, err := ParseSQL(tt.sql, "mysql")
			if err != nil {
				t.Fatalf("ParseSQL error: %v", err)
			}
			if result.DBType != "mysql" {
				t.Errorf("DBType = %q, want mysql", result.DBType)
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
			result, err := ParseSQL(tt.body, "mongodb")
			if err != nil {
				t.Fatalf("ParseSQL error: %v", err)
			}
			if result.DBType != "mongodb" {
				t.Errorf("DBType = %q, want mongodb", result.DBType)
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

func TestParseSQL_CaseInsensitiveDBType(t *testing.T) {
	tests := []struct {
		name   string
		dbType string
		wantOK bool
	}{
		{"mysql_lower", "mysql", true},
		{"mysql_mixed", "MySQL", true},
		{"mysql_upper", "MYSQL", true},
		{"mongodb_lower", "mongodb", true},
		{"mongo_alias", "mongo", true},
		{"mongodb_mixed", "MongoDB", true},
		{"postgres_unsupported", "postgres", false},
		{"empty_unsupported", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "SELECT 1"
			lower := strings.ToLower(tt.dbType)
			if lower == "mongodb" || lower == "mongo" {
				input = `{"operation": "find"}`
			}
			_, err := ParseSQL(input, tt.dbType)
			if tt.wantOK && err != nil {
				t.Errorf("dbType=%q: unexpected error: %v", tt.dbType, err)
			}
			if !tt.wantOK && err == nil {
				t.Errorf("dbType=%q: expected error, got nil", tt.dbType)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseSQL — Unsupported dbType
// ---------------------------------------------------------------------------

func TestParseSQL_UnsupportedDBType(t *testing.T) {
	_, err := ParseSQL("SELECT 1", "postgres")
	if err == nil {
		t.Error("expected error for unsupported db type")
	}
}

// ---------------------------------------------------------------------------
// ParseSQL — MySQL error inputs
// ---------------------------------------------------------------------------

func TestParseSQL_MySQLErrors(t *testing.T) {
	t.Run("empty_sql", func(t *testing.T) {
		_, err := ParseSQL("", "mysql")
		if err == nil {
			t.Error("expected error for empty SQL")
		}
	})

	t.Run("whitespace_sql", func(t *testing.T) {
		_, err := ParseSQL("   ", "mysql")
		if err == nil {
			t.Error("expected error for whitespace SQL")
		}
	})

	t.Run("invalid_syntax", func(t *testing.T) {
		_, err := ParseSQL("NOT VALID SQL AT ALL !!!", "mysql")
		if err == nil {
			t.Error("expected error for invalid SQL syntax")
		}
	})
}

// ---------------------------------------------------------------------------
// ParseSQL — MySQL tables from JOINs in unified path
// ---------------------------------------------------------------------------

func TestParseSQL_MySQLJoinTables(t *testing.T) {
	result, err := ParseSQL("SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE u.active = 1 LIMIT 10", "mysql")
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
	result, err := ParseSQL("SELECT * FROM users", "mysql")
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
	result, err := ParseSQL("SELECT * FROM users LIMIT 10", "mysql")
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
	tests := []struct {
		name       string
		sql        string
		dbType     string
		wantReason string
	}{
		{"drop_database", "DROP DATABASE db", "mysql", "DROP DATABASE is not allowed"},
		{"drop_table", "DROP TABLE t", "mysql", "DROP TABLE is not allowed"},
		{"truncate", "TRUNCATE TABLE t", "mysql", "TRUNCATE is not allowed"},
		{"update_no_where", "UPDATE t SET x = 1", "mysql", "UPDATE without WHERE clause is not allowed"},
		{"delete_no_where", "DELETE FROM t", "mysql", "DELETE without WHERE clause is not allowed"},
		{"mongo_update_many_empty", `{"operation": "update", "collection": "t", "multi": true, "filter": {}, "update": {"$set": {"x": 1}}}`, "mongodb", "updateMany with empty filter is not allowed"},
		{"mongo_delete_empty", `{"operation": "delete", "collection": "t", "filter": {}}`, "mongodb", "delete with empty filter is not allowed"},
		{"mongo_dangerous_agg", `{"operation": "aggregate", "collection": "t", "pipeline": [{"$out": "x"}]}`, "mongodb", "aggregation contains dangerous pipeline stage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSQL(tt.sql, tt.dbType)
			if err != nil {
				t.Fatalf("ParseSQL error: %v", err)
			}
			if !result.IsBlocked {
				t.Fatalf("expected IsBlocked = true")
			}
			if result.BlockReason != tt.wantReason {
				t.Errorf("BlockReason = %q, want %q", result.BlockReason, tt.wantReason)
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
			result, err := ParseSQL(tt.body, "mongodb")
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
		result, err := ParseSQL(`{"operation": "find", "collection": "users", "filter": {"x": 1}}`, "mongodb")
		if err != nil {
			t.Fatalf("ParseSQL error: %v", err)
		}
		if !equalStringSlices(result.Tables, []string{"users"}) {
			t.Errorf("Tables = %v, want [users]", result.Tables)
		}
	})

	t.Run("no_collection_empty_tables", func(t *testing.T) {
		result, err := ParseSQL(`{"operation": "find", "filter": {"x": 1}}`, "mongodb")
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

func TestExtractOperation(t *testing.T) {
	tests := []struct {
		sql  string
		want OperationType
	}{
		{"SELECT * FROM users", OpSelect},
		{"INSERT INTO users (id) VALUES (1)", OpDML},
		{"UPDATE users SET x = 1 WHERE id = 1", OpUpdate},
		{"DELETE FROM users WHERE id = 1", OpDelete},
		{"DROP TABLE users", OpDDL},
		{"CREATE TABLE t (id INT)", OpDDL},
		{"ALTER TABLE t ADD COLUMN x INT", OpDDL},
		{"TRUNCATE TABLE t", OpDDL},
		{"REPLACE INTO t VALUES (1)", OpDML},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			got := ExtractOperation(tt.sql)
			if got != tt.want {
				t.Errorf("ExtractOperation(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Legacy Functions: ExtractTables
// ---------------------------------------------------------------------------

func TestExtractTables(t *testing.T) {
	tests := []struct {
		sql  string
		want []string
	}{
		{"SELECT * FROM users", []string{"users"}},
		{"SELECT * FROM users u JOIN orders o ON u.id = o.user_id", []string{"users", "orders"}},
		{"UPDATE users SET x = 1", []string{"users"}},
		{"INSERT INTO users (id) VALUES (1)", []string{"users"}},
		{"DELETE FROM users WHERE id = 1", []string{"users"}},
		{"DROP TABLE users", []string{"users"}},
		{"CREATE TABLE t (id INT)", []string{"t"}},
		{"ALTER TABLE t ADD x INT", []string{"t"}},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			got := ExtractTables(tt.sql)
			if !equalStringSlices(got, tt.want) {
				t.Errorf("ExtractTables(%q) = %v, want %v", tt.sql, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Legacy Functions: IsHighRisk
// ---------------------------------------------------------------------------

func TestIsHighRisk(t *testing.T) {
	tests := []struct {
		sql  string
		op   OperationType
		want bool
	}{
		{"DROP TABLE users", OpDDL, true},
		{"DROP DATABASE test", OpDDL, true},
		{"TRUNCATE TABLE users", OpDDL, true},
		{"UPDATE users SET x = 1", OpUpdate, true},
		{"UPDATE users SET x = 1 WHERE id = 1", OpUpdate, false},
		{"DELETE FROM users", OpDelete, true},
		{"DELETE FROM users WHERE id = 1", OpDelete, false},
		{"SELECT * FROM users", OpSelect, false},
		{"INSERT INTO users (id) VALUES (1)", OpDML, false},
		{"CREATE TABLE t (id INT)", OpDDL, false},
		{"ALTER TABLE t ADD x INT", OpDDL, false},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			got := IsHighRisk(tt.sql, tt.op)
			if got != tt.want {
				t.Errorf("IsHighRisk(%q, %q) = %v, want %v", tt.sql, tt.op, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Legacy Functions: GetRiskLevel
// ---------------------------------------------------------------------------

func TestGetRiskLevel(t *testing.T) {
	tests := []struct {
		sql  string
		op   OperationType
		want RiskLevel
	}{
		{"SELECT * FROM users", OpSelect, RiskLow},
		{"DROP TABLE users", OpDDL, RiskHigh},
		{"UPDATE users SET x = 1 WHERE id = 1", OpUpdate, RiskMedium},
		{"INSERT INTO users (id) VALUES (1)", OpDML, RiskMedium},
		{"DELETE FROM users WHERE id = 1", OpDelete, RiskMedium},
		{"CREATE TABLE t (id INT)", OpDDL, RiskMedium},
		{"UPDATE users SET x = 1", OpUpdate, RiskHigh},
		{"DELETE FROM users", OpDelete, RiskHigh},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			got := GetRiskLevel(tt.sql, tt.op)
			if got != tt.want {
				t.Errorf("GetRiskLevel(%q, %q) = %q, want %q", tt.sql, tt.op, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Legacy Functions: ExtractMongoOperation
// ---------------------------------------------------------------------------

func TestExtractMongoOperation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want MongoOperation
	}{
		{"find", `{"operation": "find", "filter": {}}`, MongoOpFind},
		{"aggregate", `{"operation": "aggregate", "pipeline": []}`, MongoOpAggregate},
		{"update", `{"operation": "update", "update": {"$set": {"x": 1}}}`, MongoOpUpdate},
		{"delete", `{"operation": "delete"}`, MongoOpDelete},
		{"updateOne", `{"operation": "updateOne"}`, MongoOpUpdate},
		{"deleteMany", `{"operation": "deleteMany"}`, MongoOpDelete},
		{"inferred_pipeline", `{"pipeline": [{"$match": {}}]}`, MongoOpAggregate},
		{"invalid_json", `{invalid}`, MongoOpUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMongoOperation(tt.body)
			if got != tt.want {
				t.Errorf("ExtractMongoOperation(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Legacy Functions: IsHighRiskMongo
// ---------------------------------------------------------------------------

func TestIsHighRiskMongo(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"safe_find", `{"operation": "find", "filter": {"x": 1}}`, false},
		{"update_many_empty", `{"operation": "update", "multi": true, "filter": {}, "update": {"$set": {"x": 1}}}`, true},
		{"delete_empty", `{"operation": "delete", "filter": {}}`, true},
		{"dangerous_agg", `{"operation": "aggregate", "pipeline": [{"$out": "t"}]}`, true},
		{"invalid_json_high_risk", `{invalid json}`, true},
		{"update_with_filter_safe", `{"operation": "update", "filter": {"id": 1}, "update": {"$set": {"x": 1}}}`, false},
		{"delete_with_filter_safe", `{"operation": "delete", "filter": {"id": 1}}`, false},
		{"safe_agg", `{"operation": "aggregate", "pipeline": [{"$match": {}}]}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHighRiskMongo(tt.body)
			if got != tt.want {
				t.Errorf("IsHighRiskMongo(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractOperationRegex — fallback path for valid SQL
// ---------------------------------------------------------------------------

func TestExtractOperationRegex(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want OperationType
	}{
		{"select_prefix", "SELECT * FROM t", OpSelect},
		{"with_prefix", "WITH cte AS (SELECT 1) SELECT * FROM cte", OpSelect},
		{"insert_prefix", "INSERT INTO t VALUES (1)", OpDML},
		{"replace_prefix", "REPLACE INTO t VALUES (1)", OpDML},
		{"update_prefix", "UPDATE t SET x = 1", OpUpdate},
		{"delete_prefix", "DELETE FROM t", OpDelete},
		{"drop_prefix", "DROP TABLE t", OpDDL},
		{"alter_prefix", "ALTER TABLE t ADD x INT", OpDDL},
		{"create_prefix", "CREATE TABLE t (id INT)", OpDDL},
		{"truncate_prefix", "TRUNCATE TABLE t", OpDDL},
		{"unknown_defaults_select", "UNKNOWN something", OpSelect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractOperation(tt.sql)
			if got != tt.want {
				t.Errorf("ExtractOperation(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// removeComments — direct tests
// ---------------------------------------------------------------------------

func TestRemoveComments(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{"no_comments", "SELECT 1", "SELECT 1"},
		{"single_line_comment", "SELECT 1 -- comment", "SELECT 1 "},
		{"multi_line_comment", "/* header */ SELECT 1", " SELECT 1"},
		{"comment_only", "-- just a comment", ""},
		{"unclosed_block_comment", "/* comment SELECT 1", ""},
		{"multi_line_in_middle", "SELECT /* hint */ 1 FROM t", "SELECT  1 FROM t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeComments(tt.sql)
			if got != tt.want {
				t.Errorf("removeComments(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// isSQLKeyword — direct tests
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
		_, err := ParseSQL("", "mongodb")
		if err == nil {
			t.Error("expected error for empty MongoDB body via ParseSQL")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		_, err := ParseSQL("{invalid}", "mongodb")
		if err == nil {
			t.Error("expected error for invalid JSON via ParseSQL")
		}
	})

	t.Run("whitespace_body", func(t *testing.T) {
		_, err := ParseSQL("   ", "mongodb")
		if err == nil {
			t.Error("expected error for whitespace-only body via ParseSQL")
		}
	})
}

// ---------------------------------------------------------------------------
// Supplementary: ParseSQL "mongo" alias
// ---------------------------------------------------------------------------

func TestParseSQL_MongoAlias(t *testing.T) {
	result, err := ParseSQL(`{"operation": "find", "collection": "users", "filter": {"x": 1}}`, "mongo")
	if err != nil {
		t.Fatalf("ParseSQL with 'mongo' alias error: %v", err)
	}
	if result.DBType != "mongodb" {
		t.Errorf("DBType = %q, want mongodb", result.DBType)
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
			result, err := ParseSQL(tt.sql, "mysql")
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

func TestExtractOperation_FallbackRegex(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want OperationType
	}{
		// These SQL strings won't parse via AST, so ExtractOperation falls back to extractOperationRegex
		{"comment_only_select_fallback", "-- comment\nSELECT 1", OpSelect},
		{"garbage_defaults_to_select", "NOT VALID SQL !!!", OpSelect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractOperation(tt.sql)
			if got != tt.want {
				t.Errorf("ExtractOperation(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Supplementary: ExtractTables with unparseable SQL
// ---------------------------------------------------------------------------

func TestExtractTables_UnparseableSQL(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"garbage", "NOT VALID SQL !!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTables(tt.sql)
			if got != nil {
				t.Errorf("ExtractTables(%q) = %v, want nil", tt.sql, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Supplementary: IsHighRisk fallback path (unparseable SQL)
// ---------------------------------------------------------------------------

func TestIsHighRisk_FallbackRegex(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		op   OperationType
		want bool
	}{
		// These trigger the regex fallback because pingcap/parser can't parse them
		{"drop_prefix_fallback", "DROP something", OpDDL, true},
		{"truncate_prefix_fallback", "TRUNCATE something", OpDDL, true},
		{"update_no_where_fallback", "UPDATE something", OpUpdate, true},
		{"delete_no_where_fallback", "DELETE something", OpDelete, true},
		{"select_safe_fallback", "SELECT something weird !!!", OpSelect, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHighRisk(tt.sql, tt.op)
			if got != tt.want {
				t.Errorf("IsHighRisk(%q, %q) = %v, want %v", tt.sql, tt.op, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Supplementary: GetRiskLevel — unknown/default operation type
// ---------------------------------------------------------------------------

func TestGetRiskLevel_UnknownOperation(t *testing.T) {
	// For a safe SELECT, unknown op type should still be RiskMedium (default)
	got := GetRiskLevel("SELECT 1", OperationType("unknown"))
	if got != RiskMedium {
		t.Errorf("GetRiskLevel with unknown op = %q, want %q", got, RiskMedium)
	}
}

// ---------------------------------------------------------------------------
// Supplementary: appendIfAbsent — direct test
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
	result, err := ParseSQL("INSERT INTO users (id, name) VALUES (1, 'test')", "mysql")
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
	result, err := ParseSQL(`{"operation": "find", "collection": "users", "filter": {"active": true}}`, "mongodb")
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
	result, err := ParseSQL("UPDATE users SET name = 'x' WHERE id = 1", "mysql")
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

func TestGetRiskLevel_Comprehensive(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		op   OperationType
		want RiskLevel
	}{
		{"safe_select", "SELECT * FROM users WHERE id = 1", OpSelect, RiskLow},
		{"safe_insert", "INSERT INTO users (id) VALUES (1)", OpDML, RiskMedium},
		{"safe_update", "UPDATE users SET x = 1 WHERE id = 1", OpUpdate, RiskMedium},
		{"safe_delete", "DELETE FROM users WHERE id = 1", OpDelete, RiskMedium},
		{"safe_create", "CREATE TABLE t (id INT)", OpDDL, RiskMedium},
		{"high_update_no_where", "UPDATE users SET x = 1", OpUpdate, RiskHigh},
		{"high_delete_no_where", "DELETE FROM users", OpDelete, RiskHigh},
		{"high_drop", "DROP TABLE t", OpDDL, RiskHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRiskLevel(tt.sql, tt.op)
			if got != tt.want {
				t.Errorf("GetRiskLevel(%q, %q) = %q, want %q", tt.sql, tt.op, got, tt.want)
			}
		})
	}
}
