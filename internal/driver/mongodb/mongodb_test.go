package mongodb

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

func TestMongoDBDriver_Type(t *testing.T) {
	d := &MongoDBDriver{}
	if got := d.Type(); got != "mongodb" {
		t.Errorf("Type() = %q, want %q", got, "mongodb")
	}
}

func TestMongoDBDriver_Capabilities(t *testing.T) {
	d := &MongoDBDriver{}
	caps := d.Capabilities()

	// MongoDB supports these
	have := []driver.Capability{
		driver.CapQuery, driver.CapTicketExec, driver.CapMetadata,
		driver.CapTableLevelPermission, driver.CapFieldMasking,
	}
	// MongoDB does NOT support these
	lack := []driver.Capability{
		driver.CapSQLParse, driver.CapExport,
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

func TestMongoDBDriver_Parse(t *testing.T) {
	d := &MongoDBDriver{}

	tests := []struct {
		name        string
		body        string
		wantOp      string
		wantBlocked bool
		wantRisk    string
		wantTargets []string
	}{
		{
			name:   "find safe",
			body:   `{"operation":"find","collection":"users","filter":{"active":true}}`,
			wantOp: "select", wantRisk: "low", wantTargets: []string{"users"},
		},
		{
			name:   "aggregate safe",
			body:   `{"operation":"aggregate","collection":"orders","pipeline":[{"$match":{"status":"A"}},{"$group":{"_id":"$product","total":{"$sum":"$amount"}}}]}`,
			wantOp: "select", wantRisk: "low", wantTargets: []string{"orders"},
		},
		{
			name:   "insert",
			body:   `{"operation":"insert","collection":"users","document":{"name":"test"}}`,
			wantOp: "dml", wantTargets: []string{"users"},
		},
		{
			name:        "update with empty filter multi blocked",
			body:        `{"operation":"update","collection":"users","multi":true,"filter":{},"update":{"$set":{"x":1}}}`,
			wantOp:      "update", wantBlocked: true, wantRisk: "high",
		},
		{
			name:        "delete empty filter blocked",
			body:        `{"operation":"delete","collection":"logs","filter":{}}`,
			wantOp:      "delete", wantBlocked: true, wantRisk: "high",
		},
		{
			name:        "aggregate with $out blocked",
			body:        `{"operation":"aggregate","collection":"users","pipeline":[{"$out":"backup"}]}`,
			wantOp:      "select", wantBlocked: true, wantRisk: "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := d.Parse(tt.body)
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

func TestMongoDBDriver_NotConnected(t *testing.T) {
	d := &MongoDBDriver{}

	if err := d.Ping(context.TODO()); err == nil {
		t.Error("Ping() should fail when not connected")
	}
	if _, err := d.ListDatabases(context.TODO()); err == nil {
		t.Error("ListDatabases() should fail when not connected")
	}
	if _, err := d.ListTables(context.TODO(), "mydb"); err == nil {
		t.Error("ListTables() should fail when not connected")
	}
	if _, err := d.GetColumns(context.TODO(), "mydb", "mycoll"); err == nil {
		t.Error("GetColumns() should fail when not connected")
	}
	if _, err := d.ExecuteQuery(context.TODO(), "mydb", `{"operation":"find","collection":"c"}`, 10); err == nil {
		t.Error("ExecuteQuery() should fail when not connected")
	}
	if _, err := d.ExecuteStatement(context.TODO(), "mydb", `{"operation":"insert","collection":"c","document":{}}`); err == nil {
		t.Error("ExecuteStatement() should fail when not connected")
	}
}

func TestMongoDBDriver_Registry(t *testing.T) {
	if !driver.IsRegistered("mongodb") {
		t.Error("mongodb should be registered via init()")
	}

	d, err := driver.NewDriver("mongodb")
	if err != nil {
		t.Fatalf("NewDriver(mongodb) error: %v", err)
	}

	if d.Type() != "mongodb" {
		t.Errorf("NewDriver(mongodb).Type() = %q, want %q", d.Type(), "mongodb")
	}
}

func TestInferBSONType(t *testing.T) {
	tests := []struct {
		val  interface{}
		want string
	}{
		{val: "hello", want: "string"},
		{val: 42, want: "int"},
		{val: 3.14, want: "double"},
		{val: true, want: "bool"},
		{val: nil, want: "null"},
		{val: []interface{}{1, 2}, want: "array"},
		{val: map[string]interface{}{"a": 1}, want: "object"},
	}

	for _, tt := range tests {
		got := inferBSONType(tt.val)
		if got != tt.want {
			t.Errorf("inferBSONType(%v) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestExtractField(t *testing.T) {
	body := `{"filter":{"active":true},"collection":"users","pipeline":[{"$match":{"x":1}}]}`

	if s, ok := extractField(body, "collection"); !ok || s != `"users"` {
		t.Errorf("extractField(collection) = %q, %v, want \"users\"", s, ok)
	}

	if s, ok := extractField(body, "missing"); ok {
		t.Errorf("extractField(missing) should not be found, got %q", s)
	}
}

func TestExtractMap(t *testing.T) {
	body := `{"filter":{"age":25},"update":{"$set":{"name":"x"}}}`

	filter := extractMap(body, "filter")
	if filter == nil {
		t.Fatal("extractMap(filter) should not be nil")
	}
	if v, ok := filter["age"]; !ok || v != float64(25) {
		t.Errorf("filter[age] = %v, want 25", v)
	}
}

func TestExtractSlice(t *testing.T) {
	body := `{"documents":[{"a":1},{"b":2}]}`
	docs := extractSlice(body, "documents")
	if docs == nil || len(docs) != 2 {
		t.Errorf("extractSlice(documents) = %v, want 2 items", docs)
	}
}

func TestExtractURI(t *testing.T) {
	tests := []struct {
		name   string
		config *driver.Config
		want   string
	}{
		{
			name: "uri from host field (full URI)",
			config: &driver.Config{Host: "mongodb://user:pass@host:27017/db"},
			want:  "mongodb://user:pass@host:27017/db",
		},
		{
			name: "uri from Extra",
			config: &driver.Config{Extra: map[string]interface{}{"uri": "mongodb://localhost:27017"}},
			want:  "mongodb://localhost:27017",
		},
		{
			name:   "built from components",
			config: &driver.Config{Username: "u", Password: "p", Host: "localhost", Port: 27017, Database: "test"},
			want:   "mongodb://u:p@localhost:27017/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURI(tt.config)
			if got != tt.want {
				t.Errorf("extractURI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsURI(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{s: "mongodb://localhost:27017", want: true},
		{s: "mongodb+srv://cluster.example.com", want: true},
		{s: "localhost", want: false},
		{s: "", want: false},
	}

	for _, tt := range tests {
		got := isURI(tt.s)
		if got != tt.want {
			t.Errorf("isURI(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestMongoDBDriver_Connect_NoURI(t *testing.T) {
	d := &MongoDBDriver{}
	err := d.Connect(context.TODO(), &driver.Config{})
	if err == nil {
		t.Error("Connect() should fail without URI")
	}
	if !strings.Contains(err.Error(), "URI is required") {
		t.Errorf("Connect() error = %v, want URI required", err)
	}
}

func TestMongoDBDriver_ExecuteQuery_NoDatabase(t *testing.T) {
	d := &MongoDBDriver{}
	_, err := d.ExecuteQuery(context.TODO(), "", `{"operation":"find","collection":"c"}`, 10)
	if err == nil {
		t.Error("ExecuteQuery() should fail without database")
	}
}

func TestMongoDBDriver_ExecuteStatement_InvalidJSON(t *testing.T) {
	d := &MongoDBDriver{}
	_, err := d.ExecuteStatement(context.TODO(), "mydb", "not json")
	if err == nil {
		t.Error("ExecuteStatement() should fail for invalid JSON")
	}
}

func TestMongoDBDriver_Parse_InvalidJSON(t *testing.T) {
	d := &MongoDBDriver{}
	_, err := d.Parse("not json")
	if err == nil {
		t.Error("Parse() should fail for invalid JSON")
	}
}

func TestSuccessAndErrorResult(t *testing.T) {
	sr, err := successResult("stmt", 5, 100)
	if err != nil {
		t.Fatalf("successResult() error: %v", err)
	}
	if sr.Status != "success" || sr.RowsAffected != 5 || sr.DurationMs != 100 {
		t.Errorf("successResult() = %+v, want success/5/100", sr)
	}

	errMsg := fmt.Errorf("some error")
	er, err := errorResult("stmt", 200, errMsg)
	if err != errMsg {
		t.Errorf("errorResult() error = %v, want %v", err, errMsg)
	}
	if er.Status != "error" {
		t.Errorf("errorResult().Status = %q, want error", er.Status)
	}
}
