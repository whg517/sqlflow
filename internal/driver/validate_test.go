package driver_test

import (
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
)

// Configuration rules belong to the driver. These tests pin the rules that used
// to live as type branches in DatasourceService.

func TestValidateConfigFor_ElasticsearchTransport(t *testing.T) {
	tests := []struct {
		name    string
		urls    []string
		wantErr string
	}{
		{"https allowed", []string{"https://es.example.com:9200"}, ""},
		{"multiple https", []string{"https://es1:9200", "https://es2:9200"}, ""},
		{"https with path", []string{"https://es.example.com:9200/elastic"}, ""},
		{"public http blocked", []string{"http://es.example.com:9200"}, "必须使用 HTTPS"},
		{"private ipv4 http allowed", []string{"http://10.168.106.114:9200"}, ""},
		{"loopback http allowed", []string{"http://127.0.0.1:9200"}, ""},
		{"localhost http allowed", []string{"http://localhost:9200"}, ""},
		{"mixed public http blocked", []string{"https://es1:9200", "http://es.example.com:9200"}, "必须使用 HTTPS"},
		{"unsupported scheme", []string{"ftp://10.0.0.1:9200"}, "必须使用 HTTP 或 HTTPS"},
		{"missing hostname", []string{"http://"}, "连接地址无效"},
		// Behavior change: the service-level check used to accept a datasource
		// with no URLs at all, which produced an Elasticsearch source that could
		// never connect. The driver requires at least one.
		{"no urls rejected", nil, "至少需要一个连接地址"},
		{"blank urls rejected", []string{"", "  "}, "至少需要一个连接地址"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &driver.Config{Extra: map[string]interface{}{"urls": tt.urls}}
			err := driver.ValidateConfigFor("elasticsearch", cfg)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateConfigFor: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected rejection containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfigFor_SQLiteRequiresPath(t *testing.T) {
	if err := driver.ValidateConfigFor("sqlite", &driver.Config{}); err == nil {
		t.Fatal("an empty SQLite path should be rejected")
	}
	// Existence is checked when connecting, not here: a datasource may be
	// configured before its file is populated.
	if err := driver.ValidateConfigFor("sqlite", &driver.Config{Database: "/nonexistent/x.db"}); err != nil {
		t.Errorf("ValidateConfig must not touch the filesystem, got %v", err)
	}
}

// Drivers without extra rules must pass rather than be forced to implement an
// empty method.
func TestValidateConfigFor_DriversWithoutRules(t *testing.T) {
	for _, typeName := range []string{"mysql", "postgresql", "mongodb"} {
		if err := driver.ValidateConfigFor(typeName, &driver.Config{Host: "h", Port: 1}); err != nil {
			t.Errorf("%s: %v", typeName, err)
		}
	}
}

func TestValidateConfigFor_UnknownType(t *testing.T) {
	if err := driver.ValidateConfigFor("oracle", &driver.Config{}); err == nil {
		t.Fatal("an unregistered type should be rejected")
	}
}
