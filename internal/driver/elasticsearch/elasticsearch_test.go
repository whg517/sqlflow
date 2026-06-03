package elasticsearch

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

func TestESDriver_Type(t *testing.T) {
	d := &ESDriver{}
	if got := d.Type(); got != "elasticsearch" {
		t.Errorf("Type() = %q, want %q", got, "elasticsearch")
	}
}

func TestESDriver_Capabilities(t *testing.T) {
	d := &ESDriver{}
	caps := d.Capabilities()

	have := []driver.Capability{
		driver.CapQuery, driver.CapMetadata,
		driver.CapFieldMasking, driver.CapExport,
	}
	lack := []driver.Capability{
		driver.CapTicketExec, driver.CapSQLParse, driver.CapTableLevelPermission,
	}

	for _, cap := range have {
		if !caps.Has(cap) {
			t.Errorf("Capabilities() missing %d", cap)
		}
	}
	for _, cap := range lack {
		if caps.Has(cap) {
			t.Errorf("Capabilities() should NOT have %d", cap)
		}
	}
}

func TestESDriver_Parse(t *testing.T) {
	d := &ESDriver{}

	tests := []struct {
		name        string
		input       string
		wantOp      string
		wantBlocked bool
		wantRisk    string
		wantTargets []string
	}{
		{
			name:   "search with query",
			input:  `{"index":"logs-*","body":{"query":{"match_all":{}},"size":10}}`,
			wantOp: "select", wantRisk: "low", wantTargets: []string{"logs-*"},
		},
		{
			name:   "search with aggs",
			input:  `{"index":"orders","body":{"query":{"match_all":{}},"aggs":{"by_status":{"terms":{"field":"status"}}}}}`,
			wantOp: "select", wantRisk: "medium",
		},
		{
			name:        "script blocked",
			input:       `{"index":"test","body":{"script":{"source":"doc.price.value * 2"}}}`,
			wantBlocked: true, wantRisk: "high",
		},
		{
			name:        "script_fields blocked",
			input:       `{"index":"test","body":{"query":{"match_all":{}},"script_fields":{"x":{"script":{"source":"1"}}}}}`,
			wantBlocked: true, wantRisk: "high",
		},
		{
			name:        "dangerous endpoint _bulk",
			input:       `{"index":"_bulk","body":{"query":{"match_all":{}}}}`,
			wantBlocked: true, wantRisk: "high",
		},
		{
			name:        "operation delete blocked",
			input:       `{"index":"test","operation":"delete","body":{"query":{"match_all":{}}}}`,
			wantBlocked: true,
		},
		{
			name:   "count operation",
			input:  `{"index":"test","operation":"count","body":{"query":{"match_all":{}}}}`,
			wantOp: "select", wantRisk: "low",
		},
		{
			name:        "arbitrary body field blocked",
			input:       `{"index":"test","body":{"query":{"match_all":{}},"suggest":{"my-sug":{"text":"test"}}}}`,
			wantBlocked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := d.Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", result.Operation, tt.wantOp)
			}
			if result.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v", result.IsBlocked, tt.wantBlocked)
			}
			if tt.wantRisk != "" && result.RiskLevel != tt.wantRisk {
				t.Errorf("RiskLevel = %q, want %q", result.RiskLevel, tt.wantRisk)
			}
			if tt.wantTargets != nil {
				if len(result.Targets) != len(tt.wantTargets) {
					t.Errorf("Targets = %v, want %v", result.Targets, tt.wantTargets)
				}
			}
		})
	}
}

func TestESDriver_NotConnected(t *testing.T) {
	d := &ESDriver{}

	if err := d.Ping(context.TODO()); err == nil {
		t.Error("Ping() should fail when not connected")
	}
	if _, err := d.ListDatabases(context.TODO()); err == nil {
		t.Error("ListDatabases() should fail when not connected")
	}
	if _, err := d.ListTables(context.TODO(), ""); err == nil {
		t.Error("ListTables() should fail when not connected")
	}
	if _, err := d.GetColumns(context.TODO(), "", "test"); err == nil {
		t.Error("GetColumns() should fail when not connected")
	}
	if _, err := d.ExecuteQuery(context.TODO(), "", `{"index":"test","body":{"query":{"match_all":{}}}}`, 10); err == nil {
		t.Error("ExecuteQuery() should fail when not connected")
	}
	if _, err := d.ExecuteStatement(context.TODO(), "", ""); err == nil {
		t.Error("ExecuteStatement() should fail (not supported)")
	}
}

func TestESDriver_ExecuteStatement_NotSupported(t *testing.T) {
	d := &ESDriver{}
	_, err := d.ExecuteStatement(context.TODO(), "test", `{"operation":"delete","index":"test"}`)
	if err == nil {
		t.Error("ExecuteStatement should return error")
	}
}

func TestESDriver_ExecuteQuery_InvalidJSON(t *testing.T) {
	d := &ESDriver{}
	_, err := d.ExecuteQuery(context.TODO(), "", "not json", 10)
	if err == nil {
		t.Error("ExecuteQuery should fail for invalid JSON")
	}
}

func TestESDriver_ExecuteQuery_NoIndex(t *testing.T) {
	d := &ESDriver{}
	_, err := d.ExecuteQuery(context.TODO(), "", `{"body":{"query":{"match_all":{}}}}`, 10)
	if err == nil {
		t.Error("ExecuteQuery should fail without index")
	}
}

func TestESDriver_Connect_NoURLs(t *testing.T) {
	d := &ESDriver{}
	err := d.Connect(context.TODO(), &driver.Config{})
	if err == nil {
		t.Error("Connect() should fail without URLs")
	}
}

func TestESDriver_Parse_InvalidJSON(t *testing.T) {
	d := &ESDriver{}
	_, err := d.Parse("not json")
	if err == nil {
		t.Error("Parse() should fail for invalid JSON")
	}
}

func TestESDriver_Registry(t *testing.T) {
	if !driver.IsRegistered("elasticsearch") {
		t.Error("elasticsearch should be registered via init()")
	}

	d, err := driver.NewDriver("elasticsearch")
	if err != nil {
		t.Fatalf("NewDriver(elasticsearch) error: %v", err)
	}

	if d.Type() != "elasticsearch" {
		t.Errorf("NewDriver(elasticsearch).Type() = %q, want %q", d.Type(), "elasticsearch")
	}
}

func TestESDriver_Close_NilClient(t *testing.T) {
	d := &ESDriver{}
	if err := d.Close(); err != nil {
		t.Errorf("Close() on nil client should not error: %v", err)
	}
}

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		name string
		cfg  *driver.Config
		want []string
	}{
		{
			name: "from Extra urls as []interface{}",
			cfg: &driver.Config{Extra: map[string]interface{}{
				"urls": []interface{}{"http://es1:9200", "http://es2:9200"},
			}},
			want: []string{"http://es1:9200", "http://es2:9200"},
		},
		{
			name: "from Extra urls as string",
			cfg: &driver.Config{Extra: map[string]interface{}{
				"urls": "http://es:9200",
			}},
			want: []string{"http://es:9200"},
		},
		{
			name: "built from host/port http",
			cfg:  &driver.Config{Host: "localhost", Port: 9200},
			want: []string{"http://localhost:9200"},
		},
		{
			name: "built from host/port https",
			cfg:  &driver.Config{Host: "es.example.com", Port: 9200, SSLMode: "require"},
			want: []string{"https://es.example.com:9200"},
		},
		{
			name: "no URLs",
			cfg:  &driver.Config{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURLs(tt.cfg)
			if len(got) != len(tt.want) {
				t.Fatalf("extractURLs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractURLs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		val  interface{}
		want float64
		ok   bool
	}{
		{val: float64(3.14), want: 3.14, ok: true},
		{val: float32(2.5), want: 2.5, ok: true},
		{val: int(42), want: 42, ok: true},
		{val: int64(99), want: 99, ok: true},
		{val: "not a number", ok: false},
		{val: nil, ok: false},
	}

	for _, tt := range tests {
		got, ok := toFloat64(tt.val)
		if ok != tt.ok {
			t.Errorf("toFloat64(%v) ok = %v, want %v", tt.val, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("toFloat64(%v) = %v, want %v", tt.val, got, tt.want)
		}
	}
}
