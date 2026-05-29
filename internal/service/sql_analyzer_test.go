package service

import (
	"testing"
)

func TestSQLAnalyzer_SelectBasic(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("SELECT id, name FROM users WHERE id = 1")

	if r.SQLType != "SELECT" {
		t.Errorf("SQLType = %q, want SELECT", r.SQLType)
	}
	if !r.IsRead {
		t.Error("IsRead should be true for SELECT")
	}
	if !r.IsDML {
		t.Error("IsDML should be true for SELECT")
	}
	if !containsString(r.AffectedTables, "users") {
		t.Errorf("AffectedTables = %v, want contains users", r.AffectedTables)
	}
}

func TestSQLAnalyzer_SelectJoin(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id")

	if r.SQLType != "SELECT" {
		t.Errorf("SQLType = %q, want SELECT", r.SQLType)
	}
	if !containsString(r.AffectedTables, "users") {
		t.Error("missing users in AffectedTables")
	}
	if !containsString(r.AffectedTables, "orders") {
		t.Error("missing orders in AffectedTables")
	}
}

func TestSQLAnalyzer_Insert(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("INSERT INTO products (name, price) VALUES ('widget', 9.99)")

	if r.SQLType != "INSERT" {
		t.Errorf("SQLType = %q, want INSERT", r.SQLType)
	}
	if !r.IsDML {
		t.Error("IsDML should be true for INSERT")
	}
	if r.IsRead {
		t.Error("IsRead should be false for INSERT")
	}
	if !containsString(r.AffectedTables, "products") {
		t.Errorf("AffectedTables = %v, want contains products", r.AffectedTables)
	}
}

func TestSQLAnalyzer_Update(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("UPDATE users SET name = 'Alice' WHERE id = 1")

	if r.SQLType != "UPDATE" {
		t.Errorf("SQLType = %q, want UPDATE", r.SQLType)
	}
	if !r.IsDML {
		t.Error("IsDML should be true for UPDATE")
	}
	if !containsString(r.AffectedTables, "users") {
		t.Errorf("AffectedTables = %v, want contains users", r.AffectedTables)
	}
}

func TestSQLAnalyzer_Delete(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("DELETE FROM logs WHERE created_at < '2024-01-01'")

	if r.SQLType != "DELETE" {
		t.Errorf("SQLType = %q, want DELETE", r.SQLType)
	}
	if !r.IsDML {
		t.Error("IsDML should be true for DELETE")
	}
	if !containsString(r.AffectedTables, "logs") {
		t.Errorf("AffectedTables = %v, want contains logs", r.AffectedTables)
	}
}

func TestSQLAnalyzer_CreateTable(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("CREATE TABLE orders (id INT PRIMARY KEY, user_id INT, total DECIMAL(10,2))")

	if r.SQLType != "CREATE" {
		t.Errorf("SQLType = %q, want CREATE", r.SQLType)
	}
	if !r.IsDDL {
		t.Error("IsDDL should be true for CREATE TABLE")
	}
	if !containsString(r.AffectedTables, "orders") {
		t.Errorf("AffectedTables = %v, want contains orders", r.AffectedTables)
	}
}

func TestSQLAnalyzer_CreateTableIfNotExists(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("CREATE TABLE IF NOT EXISTS sessions (id VARCHAR(36) PRIMARY KEY)")

	if r.SQLType != "CREATE" {
		t.Errorf("SQLType = %q, want CREATE", r.SQLType)
	}
	if !containsString(r.AffectedTables, "sessions") {
		t.Errorf("AffectedTables = %v, want contains sessions", r.AffectedTables)
	}
}

func TestSQLAnalyzer_AlterTable(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("ALTER TABLE users ADD COLUMN email VARCHAR(255)")

	if r.SQLType != "ALTER" {
		t.Errorf("SQLType = %q, want ALTER", r.SQLType)
	}
	if !r.IsDDL {
		t.Error("IsDDL should be true for ALTER TABLE")
	}
	if !containsString(r.AffectedTables, "users") {
		t.Errorf("AffectedTables = %v, want contains users", r.AffectedTables)
	}
}

func TestSQLAnalyzer_DropTable(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("DROP TABLE IF EXISTS temp_data")

	if r.SQLType != "DROP" {
		t.Errorf("SQLType = %q, want DROP", r.SQLType)
	}
	if !r.IsDDL {
		t.Error("IsDDL should be true for DROP TABLE")
	}
	if !containsString(r.AffectedTables, "temp_data") {
		t.Errorf("AffectedTables = %v, want contains temp_data", r.AffectedTables)
	}
}

func TestSQLAnalyzer_Truncate(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("TRUNCATE TABLE cache_entries")

	if r.SQLType != "TRUNCATE" {
		t.Errorf("SQLType = %q, want TRUNCATE", r.SQLType)
	}
	if !r.IsDDL {
		t.Error("IsDDL should be true for TRUNCATE")
	}
	if !containsString(r.AffectedTables, "cache_entries") {
		t.Errorf("AffectedTables = %v, want contains cache_entries", r.AffectedTables)
	}
}

func TestSQLAnalyzer_BacktickTables(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("SELECT * FROM `my-table`")

	if !containsString(r.AffectedTables, "my-table") {
		t.Errorf("AffectedTables = %v, want contains my-table", r.AffectedTables)
	}
}

func TestSQLAnalyzer_SchemaQualified(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("SELECT * FROM public.users")

	if !containsString(r.AffectedTables, "users") {
		t.Errorf("AffectedTables = %v, want contains users (stripped schema)", r.AffectedTables)
	}
}

func TestSQLAnalyzer_Empty(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("")

	if r.SQLType != "OTHER" {
		t.Errorf("SQLType = %q, want OTHER for empty", r.SQLType)
	}
}

func TestSQLAnalyzer_WithComments(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("-- get active users\nSELECT * FROM users /* all columns */ WHERE active = 1")

	if r.SQLType != "SELECT" {
		t.Errorf("SQLType = %q, want SELECT", r.SQLType)
	}
	if !containsString(r.AffectedTables, "users") {
		t.Errorf("AffectedTables = %v, want contains users", r.AffectedTables)
	}
}

func TestSQLAnalyzer_CTESelect(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("WITH active AS (SELECT * FROM users WHERE active = 1) SELECT * FROM active JOIN orders ON active.id = orders.user_id")

	if r.SQLType != "SELECT" {
		t.Errorf("SQLType = %q, want SELECT for CTE", r.SQLType)
	}
	if !r.IsRead {
		t.Error("CTE SELECT should be IsRead")
	}
}

func TestSQLAnalyzer_Merge(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("MERGE INTO target USING source ON target.id = source.id WHEN MATCHED THEN UPDATE SET target.val = source.val")

	if r.SQLType != "MERGE" {
		t.Errorf("SQLType = %q, want MERGE", r.SQLType)
	}
}

func TestSQLAnalyzer_Replace(t *testing.T) {
	a := NewSQLAnalyzer()
	r := a.Analyze("REPLACE INTO users (id, name) VALUES (1, 'Bob')")

	if r.SQLType != "REPLACE" {
		t.Errorf("SQLType = %q, want REPLACE", r.SQLType)
	}
	if !containsString(r.AffectedTables, "users") {
		t.Errorf("AffectedTables = %v, want contains users", r.AffectedTables)
	}
}

func TestRiskEvaluator_Select(t *testing.T) {
	a := NewSQLAnalyzer()
	e := NewRiskEvaluator()

	analysis := a.Analyze("SELECT * FROM users")
	eval := e.Evaluate(analysis)

	if eval.Level != RiskLevelLow {
		t.Errorf("SELECT risk = %q, want low", eval.Level)
	}
}

func TestRiskEvaluator_Insert(t *testing.T) {
	a := NewSQLAnalyzer()
	e := NewRiskEvaluator()

	analysis := a.Analyze("INSERT INTO users (name) VALUES ('test')")
	eval := e.Evaluate(analysis)

	if eval.Level != RiskLevelMedium {
		t.Errorf("INSERT risk = %q, want medium, score=%d", eval.Level, eval.Score)
	}
}

func TestRiskEvaluator_Update(t *testing.T) {
	a := NewSQLAnalyzer()
	e := NewRiskEvaluator()

	analysis := a.Analyze("UPDATE users SET name = 'test'")
	eval := e.Evaluate(analysis)

	if eval.Level != RiskLevelMedium && eval.Level != RiskLevelHigh {
		t.Errorf("UPDATE risk = %q (score=%d), expected medium or high", eval.Level, eval.Score)
	}
}

func TestRiskEvaluator_Delete(t *testing.T) {
	a := NewSQLAnalyzer()
	e := NewRiskEvaluator()

	analysis := a.Analyze("DELETE FROM logs WHERE id < 100")
	eval := e.Evaluate(analysis)

	if eval.Level != RiskLevelHigh && eval.Level != RiskLevelMedium {
		t.Errorf("DELETE risk = %q (score=%d), expected high or medium", eval.Level, eval.Score)
	}
}

func TestRiskEvaluator_DropTable(t *testing.T) {
	a := NewSQLAnalyzer()
	e := NewRiskEvaluator()

	analysis := a.Analyze("DROP TABLE users")
	eval := e.Evaluate(analysis)

	if eval.Level != RiskLevelCritical {
		t.Errorf("DROP TABLE risk = %q (score=%d), want critical", eval.Level, eval.Score)
	}
}

func TestRiskEvaluator_AlterTable(t *testing.T) {
	a := NewSQLAnalyzer()
	e := NewRiskEvaluator()

	analysis := a.Analyze("ALTER TABLE users ADD COLUMN email VARCHAR(255)")
	eval := e.Evaluate(analysis)

	if eval.Level != RiskLevelHigh && eval.Level != RiskLevelCritical {
		t.Errorf("ALTER TABLE risk = %q (score=%d), expected high or critical", eval.Level, eval.Score)
	}
}

func TestRiskEvaluator_Truncate(t *testing.T) {
	a := NewSQLAnalyzer()
	e := NewRiskEvaluator()

	analysis := a.Analyze("TRUNCATE TABLE big_table")
	eval := e.Evaluate(analysis)

	if eval.Level != RiskLevelCritical && eval.Level != RiskLevelHigh {
		t.Errorf("TRUNCATE risk = %q (score=%d), expected critical or high", eval.Level, eval.Score)
	}
}

func TestRiskEvaluator_EvaluateWithSQL(t *testing.T) {
	e := NewRiskEvaluator()
	eval := e.EvaluateWithSQL("DROP TABLE critical_data")

	if eval.Level != RiskLevelCritical {
		t.Errorf("EvaluateWithSQL DROP TABLE = %q (score=%d), want critical", eval.Level, eval.Score)
	}
}

func TestAffectedTablesToJSON(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{nil, "[]"},
		{[]string{}, "[]"},
		{[]string{"users"}, `["users"]`},
		{[]string{"users", "orders"}, `["users","orders"]`},
	}
	for _, tt := range tests {
		got := affectedTablesToJSON(tt.input)
		if got != tt.want {
			t.Errorf("affectedTablesToJSON(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsString(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
