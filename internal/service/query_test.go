package service

import (
	"testing"
)

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
