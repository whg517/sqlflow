package driver_test

import (
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/driver"
)

// mockDataSource implements driver.DataSourceInfo for testing.
type mockDataSource struct {
	id           int64
	dsType       string
	host         string
	port         int
	username     string
	database     string
	sslMode      string
	schemaName   string
	maxOpen      int
	maxIdle      int
	maxLifetime  int
	maxIdleTime  int
	extras       map[string]string
	extraBools   map[string]bool
}

func (m *mockDataSource) GetID() int64                  { return m.id }
func (m *mockDataSource) GetType() string               { return m.dsType }
func (m *mockDataSource) GetHost() string                { return m.host }
func (m *mockDataSource) GetPort() int                  { return m.port }
func (m *mockDataSource) GetUsername() string           { return m.username }
func (m *mockDataSource) GetDatabase() string           { return m.database }
func (m *mockDataSource) GetSSLMode() string             { return m.sslMode }
func (m *mockDataSource) GetSchemaName() string          { return m.schemaName }
func (m *mockDataSource) GetMaxOpen() int                { return m.maxOpen }
func (m *mockDataSource) GetMaxIdle() int                { return m.maxIdle }
func (m *mockDataSource) GetMaxLifetime() int            { return m.maxLifetime }
func (m *mockDataSource) GetMaxIdleTime() int             { return m.maxIdleTime }
func (m *mockDataSource) GetExtra(key string) string {
	if m.extras != nil {
		return m.extras[key]
	}
	return ""
}
func (m *mockDataSource) GetExtraBool(key string, defaultVal bool) bool {
	if m.extraBools != nil {
		if v, ok := m.extraBools[key]; ok {
			return v
		}
	}
	return defaultVal
}

func TestBuildConfigFromDataSource_MySQL(t *testing.T) {
	ds := &mockDataSource{
		id:          1,
		dsType:      "mysql",
		host:        "localhost",
		port:        3306,
		username:    "root",
		database:    "testdb",
		maxOpen:     20,
		maxIdle:     10,
		maxLifetime: 1800,
		maxIdleTime: 300,
	}

	cfg, err := driver.BuildConfigFromDataSource(ds, "secret", "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ID != 1 {
		t.Errorf("ID = %d, want 1", cfg.ID)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want localhost", cfg.Host)
	}
	if cfg.Port != 3306 {
		t.Errorf("Port = %d, want 3306", cfg.Port)
	}
	if cfg.Password != "secret" {
		t.Errorf("Password = %q, want secret", cfg.Password)
	}
	if cfg.Database != "testdb" {
		t.Errorf("Database = %q, want testdb", cfg.Database)
	}
	if cfg.MaxOpen != 20 {
		t.Errorf("MaxOpen = %d, want 20", cfg.MaxOpen)
	}
	if cfg.MaxLifetime != 1800*time.Second {
		t.Errorf("MaxLifetime = %v, want %v", cfg.MaxLifetime, 1800*time.Second)
	}
	if cfg.Extra["_type"] != "mysql" {
		t.Errorf("Extra[_type] = %q, want mysql", cfg.Extra["_type"])
	}
}

func TestBuildConfigFromDataSource_Defaults(t *testing.T) {
	ds := &mockDataSource{
		id:     2,
		dsType: "postgresql",
		host:   "pg-host",
		port:   5432,
	}

	cfg, err := driver.BuildConfigFromDataSource(ds, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Defaults should be applied
	if cfg.MaxOpen != 10 {
		t.Errorf("MaxOpen = %d, want 10 (default)", cfg.MaxOpen)
	}
	if cfg.MaxIdle != 5 {
		t.Errorf("MaxIdle = %d, want 5 (default)", cfg.MaxIdle)
	}
	if cfg.MaxLifetime != 3600*time.Second {
		t.Errorf("MaxLifetime = %v, want 3600s (default)", cfg.MaxLifetime)
	}
	if cfg.MaxIdleTime != 600*time.Second {
		t.Errorf("MaxIdleTime = %v, want 600s (default)", cfg.MaxIdleTime)
	}
}

func TestBuildConfigFromDataSource_Elasticsearch(t *testing.T) {
	ds := &mockDataSource{
		id:     3,
		dsType: "elasticsearch",
		host:   "es-host",
		port:   9200,
		extras: map[string]string{
			"es_urls":         "http://es1:9200,http://es2:9200",
			"es_auth_type":    "api_key",
			"es_index_pattern": "logs-*",
		},
		extraBools: map[string]bool{
			"es_verify_certs": false,
		},
	}

	cfg, err := driver.BuildConfigFromDataSource(ds, "pass", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	urls := cfg.Extra["urls"].([]string)
	if len(urls) != 2 || urls[0] != "http://es1:9200" || urls[1] != "http://es2:9200" {
		t.Errorf("Extra[urls] = %v, want [http://es1:9200 http://es2:9200]", urls)
	}
	if cfg.Extra["auth_type"] != "api_key" {
		t.Errorf("Extra[auth_type] = %q, want api_key", cfg.Extra["auth_type"])
	}
	if cfg.Extra["verify_certs"] != false {
		t.Errorf("Extra[verify_certs] = %v, want false", cfg.Extra["verify_certs"])
	}
}

func TestBuildConfigFromDataSource_MongoDB(t *testing.T) {
	ds := &mockDataSource{
		id:     4,
		dsType: "mongodb",
		extras: map[string]string{
			"mongo_uri": "mongodb://user:pass@host:27017/db",
		},
	}

	cfg, err := driver.BuildConfigFromDataSource(ds, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Extra["uri"] != "mongodb://user:pass@host:27017/db" {
		t.Errorf("Extra[uri] = %q, want mongodb://...", cfg.Extra["uri"])
	}
}

func TestValidateDriverType(t *testing.T) {
	tests := []struct {
		name    string
		typeStr string
		wantErr bool
	}{
		{name: "mysql valid", typeStr: "mysql", wantErr: false},
		{name: "postgresql valid", typeStr: "postgresql", wantErr: false},
		{name: "mongodb valid", typeStr: "mongodb", wantErr: false},
		{name: "elasticsearch valid", typeStr: "elasticsearch", wantErr: false},
		{name: "empty", typeStr: "", wantErr: true},
		{name: "invalid", typeStr: "oracle", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := driver.ValidateDriverType(tt.typeStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDriverType(%q) error = %v, wantErr %v", tt.typeStr, err, tt.wantErr)
			}
		})
	}
}

func TestBuildConfigFromDataSource_InvalidType(t *testing.T) {
	ds := &mockDataSource{
		dsType: "oracle",
	}

	_, err := driver.BuildConfigFromDataSource(ds, "", "")
	if err == nil {
		t.Fatal("expected error for unregistered type, got nil")
	}
}
