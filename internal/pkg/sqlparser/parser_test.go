package sqlparser

import (
	"testing"
)

// ---------------------------------------------------------------------------
// MySQL AST Parsing Tests
// ---------------------------------------------------------------------------

func TestParseMySQL_Select(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantOp     OperationType
		wantTables []string
		wantWhere  bool
		wantLimit  bool
	}{
		{
			name:       "simple select",
			sql:        "SELECT * FROM users",
			wantOp:     OpSelect,
			wantTables: []string{"users"},
			wantWhere:  false,
			wantLimit:  false,
		},
		{
			name:       "select with where",
			sql:        "SELECT id, name FROM orders WHERE status = 'active'",
			wantOp:     OpSelect,
			wantTables: []string{"orders"},
			wantWhere:  true,
			wantLimit:  false,
		},
		{
			name:       "select with limit",
			sql:        "SELECT * FROM users LIMIT 10",
			wantOp:     OpSelect,
			wantTables: []string{"users"},
			wantWhere:  false,
			wantLimit:  true,
		},
		{
			name:       "select with where and limit",
			sql:        "SELECT * FROM users WHERE age > 18 LIMIT 100",
			wantOp:     OpSelect,
			wantTables: []string{"users"},
			wantWhere:  true,
			wantLimit:  true,
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

func TestParseMySQL_Joins(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantTables []string
	}{
		{
			name:       "inner join",
			sql:        "SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id",
			wantTables: []string{"users", "orders"},
		},
		{
			name:       "left join",
			sql:        "SELECT * FROM users LEFT JOIN profiles ON users.id = profiles.user_id",
			wantTables: []string{"users", "profiles"},
		},
		{
			name:       "three-way join",
			sql:        "SELECT * FROM a JOIN b ON a.id = b.a_id JOIN c ON b.id = c.b_id",
			wantTables: []string{"a", "b", "c"},
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

func TestParseMySQL_Subqueries(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantTables []string
	}{
		{
			name:       "subquery in where",
			sql:        "SELECT * FROM orders WHERE user_id IN (SELECT id FROM users WHERE active = 1)",
			wantTables: []string{"orders"}, // TODO: nested subquery table extraction
		},
		{
			name:       "subquery in from",
			sql:        "SELECT * FROM (SELECT id FROM users) AS subq",
			wantTables: []string{"users"},
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

func TestParseMySQL_CTE(t *testing.T) {
	t.Skip("TODO: pingcap/parser CTE support requires MySQL 8.0 mode")
	tests := []struct {
		name       string
		sql        string
		wantOp     OperationType
		wantTables []string
	}{
		{
			name:       "CTE with select",
			sql:        "WITH active_users AS (SELECT * FROM users WHERE active = 1) SELECT * FROM active_users",
			wantOp:     OpSelect,
			wantTables: []string{"users", "active_users"},
		},
		{
			name:       "CTE with insert",
			sql:        "WITH cte AS (SELECT 1) INSERT INTO t SELECT * FROM cte",
			wantOp:     OpDML,
			wantTables: []string{"t"},
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

func TestParseMySQL_Insert(t *testing.T) {
	result, err := ParseMySQL("INSERT INTO users (name, email) VALUES ('test', 'test@example.com')")
	if err != nil {
		t.Fatalf("ParseMySQL error: %v", err)
	}
	if result.Operation != OpDML {
		t.Errorf("Operation = %q, want %q", result.Operation, OpDML)
	}
	if !equalStringSlices(result.Tables, []string{"users"}) {
		t.Errorf("Tables = %v, want [users]", result.Tables)
	}
}

func TestParseMySQL_Update(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		wantWhere bool
	}{
		{
			name:      "update with where",
			sql:       "UPDATE users SET name = 'new' WHERE id = 1",
			wantWhere: true,
		},
		{
			name:      "update without where",
			sql:       "UPDATE users SET name = 'new'",
			wantWhere: false,
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
			if result.HasWhere != tt.wantWhere {
				t.Errorf("HasWhere = %v, want %v", result.HasWhere, tt.wantWhere)
			}
			if !equalStringSlices(result.Tables, []string{"users"}) {
				t.Errorf("Tables = %v, want [users]", result.Tables)
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
			name:      "delete with where",
			sql:       "DELETE FROM logs WHERE created_at < '2024-01-01'",
			wantWhere: true,
		},
		{
			name:      "delete without where",
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

func TestParseMySQL_DDL(t *testing.T) {
	tests := []struct {
		name           string
		sql            string
		wantDropDB     bool
		wantDropTable  bool
		wantTables     []string
	}{
		{
			name:          "drop database",
			sql:           "DROP DATABASE test_db",
			wantDropDB:    true,
			wantDropTable: false,
			wantTables:    []string{"test_db"},
		},
		{
			name:          "drop table",
			sql:           "DROP TABLE users",
			wantDropDB:    false,
			wantDropTable: true,
			wantTables:    []string{"users"},
		},
		{
			name:          "drop table if exists",
			sql:           "DROP TABLE IF EXISTS users",
			wantDropDB:    false,
			wantDropTable: true,
			wantTables:    []string{"users"},
		},
		{
			name:       "create table",
			sql:        "CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(100))",
			wantTables: []string{"users"},
		},
		{
			name:       "alter table",
			sql:        "ALTER TABLE users ADD COLUMN email VARCHAR(255)",
			wantTables: []string{"users"},
		},
		{
			name:       "truncate table",
			sql:        "TRUNCATE TABLE users",
			wantTables: []string{"users"},
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
			if tt.wantTables != nil && !equalStringSlices(result.Tables, tt.wantTables) {
				t.Errorf("Tables = %v, want %v", result.Tables, tt.wantTables)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MongoDB Parsing Tests
// ---------------------------------------------------------------------------

func TestParseMongo_Find(t *testing.T) {
	body := `{"operation": "find", "collection": "users", "filter": {"active": true}}`
	result, err := ParseMongo(body)
	if err != nil {
		t.Fatalf("ParseMongo error: %v", err)
	}
	if result.Operation != MongoOpFind {
		t.Errorf("Operation = %q, want %q", result.Operation, MongoOpFind)
	}
	if result.Collection != "users" {
		t.Errorf("Collection = %q, want %q", result.Collection, "users")
	}
	if !result.HasFilter {
		t.Error("HasFilter = false, want true")
	}
	if result.HasEmptyFilter {
		t.Error("HasEmptyFilter = true, want false")
	}
}

func TestParseMongo_Aggregate(t *testing.T) {
	tests := []struct {
		name               string
		body               string
		wantStages         []string
		wantDangerous      bool
	}{
		{
			name:          "safe aggregation",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$match": {"status": "active"}}, {"$group": {"_id": "$user_id", "total": {"$sum": "$amount"}}}]}`,
			wantStages:    []string{"$match", "$group"},
			wantDangerous: false,
		},
		{
			name:          "dangerous aggregation with $out",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$match": {}}, {"$out": "backup"}]}`,
			wantStages:    []string{"$match", "$out"},
			wantDangerous: true,
		},
		{
			name:          "dangerous aggregation with $merge",
			body:          `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$merge": {"into": "target"}}]}`,
			wantStages:    []string{"$merge"},
			wantDangerous: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != MongoOpAggregate {
				t.Errorf("Operation = %q, want %q", result.Operation, MongoOpAggregate)
			}
			if !equalStringSlices(result.PipelineStages, tt.wantStages) {
				t.Errorf("PipelineStages = %v, want %v", result.PipelineStages, tt.wantStages)
			}
			if result.HasDangerousStage != tt.wantDangerous {
				t.Errorf("HasDangerousStage = %v, want %v", result.HasDangerousStage, tt.wantDangerous)
			}
		})
	}
}

func TestParseMongo_Update(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantIsMulti    bool
		wantEmpty      bool
	}{
		{
			name:        "updateOne with filter",
			body:        `{"operation": "update", "collection": "users", "filter": {"id": 1}, "update": {"$set": {"name": "new"}}}`,
			wantIsMulti: false,
			wantEmpty:   false,
		},
		{
			name:        "updateMany with empty filter",
			body:        `{"operation": "update", "collection": "users", "multi": true, "filter": {}, "update": {"$set": {"active": true}}}`,
			wantIsMulti: true,
			wantEmpty:   true,
		},
		{
			name:        "updateMany without filter",
			body:        `{"operation": "update", "collection": "users", "multi": true, "update": {"$set": {"active": true}}}`,
			wantIsMulti: true,
			wantEmpty:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != MongoOpUpdate {
				t.Errorf("Operation = %q, want %q", result.Operation, MongoOpUpdate)
			}
			if result.IsMulti != tt.wantIsMulti {
				t.Errorf("IsMulti = %v, want %v", result.IsMulti, tt.wantIsMulti)
			}
			if result.HasEmptyFilter != tt.wantEmpty {
				t.Errorf("HasEmptyFilter = %v, want %v", result.HasEmptyFilter, tt.wantEmpty)
			}
		})
	}
}

func TestParseMongo_Delete(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantEmpty  bool
	}{
		{
			name:      "delete with filter",
			body:      `{"operation": "delete", "collection": "logs", "filter": {"created_at": {"$lt": "2024-01-01"}}}`,
			wantEmpty: false,
		},
		{
			name:      "delete without filter",
			body:      `{"operation": "delete", "collection": "logs"}`,
			wantEmpty: true,
		},
		{
			name:      "delete with empty filter",
			body:      `{"operation": "delete", "collection": "logs", "filter": {}}`,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMongo(tt.body)
			if err != nil {
				t.Fatalf("ParseMongo error: %v", err)
			}
			if result.Operation != MongoOpDelete {
				t.Errorf("Operation = %q, want %q", result.Operation, MongoOpDelete)
			}
			if result.HasEmptyFilter != tt.wantEmpty {
				t.Errorf("HasEmptyFilter = %v, want %v", result.HasEmptyFilter, tt.wantEmpty)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unified ParseSQL Tests
// ---------------------------------------------------------------------------

func TestParseSQL_MySQL(t *testing.T) {
	tests := []struct {
		name        string
		sql         string
		wantBlocked bool
		wantRisk    RiskLevel
	}{
		{
			name:        "safe select",
			sql:         "SELECT * FROM users LIMIT 10",
			wantBlocked: false,
			wantRisk:    RiskLow,
		},
		{
			name:        "drop database blocked",
			sql:         "DROP DATABASE production",
			wantBlocked: true,
			wantRisk:    RiskHigh,
		},
		{
			name:        "drop table blocked",
			sql:         "DROP TABLE users",
			wantBlocked: true,
			wantRisk:    RiskHigh,
		},
		{
			name:        "truncate blocked",
			sql:         "TRUNCATE TABLE users",
			wantBlocked: true,
			wantRisk:    RiskHigh,
		},
		{
			name:        "update without where blocked",
			sql:         "UPDATE users SET active = 0",
			wantBlocked: true,
			wantRisk:    RiskHigh,
		},
		{
			name:        "delete without where blocked",
			sql:         "DELETE FROM logs",
			wantBlocked: true,
			wantRisk:    RiskHigh,
		},
		{
			name:        "update with where allowed",
			sql:         "UPDATE users SET name = 'x' WHERE id = 1",
			wantBlocked: false,
			wantRisk:    RiskMedium,
		},
		{
			name:        "delete with where allowed",
			sql:         "DELETE FROM logs WHERE id = 1",
			wantBlocked: false,
			wantRisk:    RiskMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSQL(tt.sql, "mysql")
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

func TestParseSQL_MongoDB(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantBlocked bool
		wantRisk    RiskLevel
	}{
		{
			name:        "safe find",
			body:        `{"operation": "find", "collection": "users", "filter": {"active": true}}`,
			wantBlocked: false,
			wantRisk:    RiskLow,
		},
		{
			name:        "updateMany with empty filter blocked",
			body:        `{"operation": "update", "collection": "users", "multi": true, "filter": {}, "update": {"$set": {"x": 1}}}`,
			wantBlocked: true,
			wantRisk:    RiskHigh,
		},
		{
			name:        "dangerous aggregate blocked",
			body:        `{"operation": "aggregate", "collection": "users", "pipeline": [{"$out": "backup"}]}`,
			wantBlocked: true,
			wantRisk:    RiskHigh,
		},
		{
			name:        "safe aggregate allowed",
			body:        `{"operation": "aggregate", "collection": "orders", "pipeline": [{"$match": {"status": "active"}}, {"$group": {"_id": null, "total": {"$sum": "$amount"}}}]}`,
			wantBlocked: false,
			wantRisk:    RiskLow,
		},
		{
			name:        "delete with empty filter blocked",
			body:        `{"operation": "delete", "collection": "logs", "filter": {}}`,
			wantBlocked: true,
			wantRisk:    RiskHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSQL(tt.body, "mongodb")
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

func TestParseSQL_UnsupportedDBType(t *testing.T) {
	_, err := ParseSQL("SELECT 1", "postgres")
	if err == nil {
		t.Error("expected error for unsupported db type")
	}
}

// ---------------------------------------------------------------------------
// Static Blocking Rule Tests
// ---------------------------------------------------------------------------

func TestStaticBlocking_DropDatabase(t *testing.T) {
	result, err := ParseSQL("DROP DATABASE mydb", "mysql")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if !result.IsBlocked {
		t.Error("DROP DATABASE should be blocked")
	}
	if result.RiskLevel != RiskHigh {
		t.Error("DROP DATABASE should be high risk")
	}
}

func TestStaticBlocking_DropTable(t *testing.T) {
	result, err := ParseSQL("DROP TABLE users", "mysql")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if !result.IsBlocked {
		t.Error("DROP TABLE should be blocked")
	}
}

func TestStaticBlocking_Truncate(t *testing.T) {
	result, err := ParseSQL("TRUNCATE TABLE users", "mysql")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if !result.IsBlocked {
		t.Error("TRUNCATE should be blocked")
	}
}

func TestStaticBlocking_UpdateWithoutWhere(t *testing.T) {
	result, err := ParseSQL("UPDATE users SET name = 'x'", "mysql")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if !result.IsBlocked {
		t.Error("UPDATE without WHERE should be blocked")
	}
}

func TestStaticBlocking_DeleteWithoutWhere(t *testing.T) {
	result, err := ParseSQL("DELETE FROM users", "mysql")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if !result.IsBlocked {
		t.Error("DELETE without WHERE should be blocked")
	}
}

func TestStaticBlocking_MongoUpdateManyEmptyFilter(t *testing.T) {
	body := `{"operation": "update", "collection": "users", "multi": true, "update": {"$set": {"x": 1}}}`
	result, err := ParseSQL(body, "mongodb")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if !result.IsBlocked {
		t.Error("updateMany with empty filter should be blocked")
	}
}

func TestStaticBlocking_MongoDangerousStage(t *testing.T) {
	body := `{"operation": "aggregate", "collection": "users", "pipeline": [{"$out": "target"}]}`
	result, err := ParseSQL(body, "mongodb")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	if !result.IsBlocked {
		t.Error("aggregation with dangerous stage should be blocked")
	}
}

// ---------------------------------------------------------------------------
// Backward-Compatible Legacy Function Tests
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

func TestExtractMongoOperation(t *testing.T) {
	tests := []struct {
		body string
		want MongoOperation
	}{
		{`{"operation": "find", "filter": {}}`, MongoOpFind},
		{`{"operation": "aggregate", "pipeline": []}`, MongoOpAggregate},
		{`{"operation": "update", "update": {"$set": {"x": 1}}}`, MongoOpUpdate},
		{`{"operation": "delete"}`, MongoOpDelete},
		{`{"operation": "updateOne"}`, MongoOpUpdate},
		{`{"operation": "deleteMany"}`, MongoOpDelete},
		{`{"pipeline": [{"$match": {}}]}`, MongoOpAggregate},
	}

	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			got := ExtractMongoOperation(tt.body)
			if got != tt.want {
				t.Errorf("ExtractMongoOperation(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

func TestIsHighRiskMongo(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{`{"operation": "find", "filter": {"x": 1}}`, false},
		{`{"operation": "update", "multi": true, "filter": {}, "update": {"$set": {"x": 1}}}`, true},
		{`{"operation": "delete", "filter": {}}`, true},
		{`{"operation": "aggregate", "pipeline": [{"$out": "t"}]}`, true},
		{`{invalid json}`, true},
	}

	for _, tt := range tests {
		got := IsHighRiskMongo(tt.body)
		if got != tt.want {
			t.Errorf("IsHighRiskMongo(%q) = %v, want %v", tt.body, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge Case Tests
// ---------------------------------------------------------------------------

func TestParseMySQL_EmptyString(t *testing.T) {
	_, err := ParseMySQL("")
	if err == nil {
		t.Error("expected error for empty SQL")
	}
}

func TestParseMySQL_Whitespace(t *testing.T) {
	_, err := ParseMySQL("   ")
	if err == nil {
		t.Error("expected error for whitespace-only SQL")
	}
}

func TestParseMySQL_Comments(t *testing.T) {
	tests := []struct {
		name   string
		sql    string
		wantOp OperationType
	}{
		{
			name:   "single line comment",
			sql:    "-- this is a comment\nSELECT * FROM users",
			wantOp: OpSelect,
		},
		{
			name:   "multi-line comment",
			sql:    "/* comment */ SELECT * FROM users",
			wantOp: OpSelect,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMySQL(tt.sql)
			if err != nil {
				t.Fatalf("ParseMySQL error: %v", err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
		})
	}
}

func TestParseMySQL_MultiStatement(t *testing.T) {
	// Only first statement should be parsed
	result, err := ParseMySQL("SELECT 1; DROP TABLE users;")
	if err != nil {
		t.Fatalf("ParseMySQL error: %v", err)
	}
	if result.Operation != OpSelect {
		t.Errorf("Operation = %q, want %q (should parse first statement only)", result.Operation, OpSelect)
	}
}

func TestParseMySQL_SyntaxError(t *testing.T) {
	_, err := ParseMySQL("SELEC * FORM users")
	if err == nil {
		t.Error("expected error for invalid SQL syntax")
	}
}

func TestParseSQL_NoLimitWarning(t *testing.T) {
	result, err := ParseSQL("SELECT * FROM users", "mysql")
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	hasLimitWarning := false
	for _, w := range result.Warnings {
		if contains(w, "LIMIT") {
			hasLimitWarning = true
		}
	}
	if !hasLimitWarning {
		t.Error("expected LIMIT warning for SELECT without LIMIT")
	}
}

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
// Helpers
// ---------------------------------------------------------------------------

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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
