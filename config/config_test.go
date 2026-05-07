package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestConfig writes a minimal valid config to a temp dir and returns the dir path.
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return dir
}

func TestLoad_ValidConfig(t *testing.T) {
	dir := writeTestConfig(t, `
server:
  port: 9090
jwt:
  secret: "a-very-long-secret-key-for-testing!!"
  expiry: "1h"
admin:
  username: "admin"
  password: "strongpassword"
db:
  path: "./data/test.db"
encryption_key: "0123456789abcdef0123456789abcdef"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Admin.Username != "admin" {
		t.Errorf("Admin.Username = %q, want %q", cfg.Admin.Username, "admin")
	}
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	dir := writeTestConfig(t, `
jwt:
  secret: ""
admin:
  password: "strongpassword"
encryption_key: "0123456789abcdef0123456789abcdef"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() should fail with empty jwt.secret")
	}
}

func TestLoad_ShortJWTSecret(t *testing.T) {
	dir := writeTestConfig(t, `
jwt:
  secret: "short"
admin:
  password: "strongpassword"
encryption_key: "0123456789abcdef0123456789abcdef"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() should fail with short jwt.secret")
	}
}

func TestLoad_MissingEncryptionKey(t *testing.T) {
	dir := writeTestConfig(t, `
jwt:
  secret: "a-very-long-secret-key-for-testing!!"
admin:
  password: "strongpassword"
encryption_key: ""
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() should fail with empty encryption_key")
	}
}

func TestLoad_InvalidEncryptionKeyLength(t *testing.T) {
	dir := writeTestConfig(t, `
jwt:
  secret: "a-very-long-secret-key-for-testing!!"
admin:
  password: "strongpassword"
encryption_key: "tooshort"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() should fail with invalid encryption_key length")
	}
}

func TestLoad_EmptyAdminPassword(t *testing.T) {
	dir := writeTestConfig(t, `
jwt:
  secret: "a-very-long-secret-key-for-testing!!"
admin:
  password: ""
encryption_key: "0123456789abcdef0123456789abcdef"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() should fail with empty admin.password")
	}
}

func TestLoad_ShortAdminPassword(t *testing.T) {
	dir := writeTestConfig(t, `
jwt:
  secret: "a-very-long-secret-key-for-testing!!"
admin:
  password: "abc"
encryption_key: "0123456789abcdef0123456789abcdef"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() should fail with short admin.password")
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	dir := writeTestConfig(t, `
jwt:
  secret: "a-very-long-secret-key-for-testing!!"
admin:
  password: "strongpassword"
encryption_key: "0123456789abcdef0123456789abcdef"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	t.Run("default_server_port", func(t *testing.T) {
		if cfg.Server.Port != 8080 {
			t.Errorf("Server.Port = %d, want default 8080", cfg.Server.Port)
		}
	})
	t.Run("default_db_path", func(t *testing.T) {
		if cfg.DB.Path != "./data/sqlflow.db" {
			t.Errorf("DB.Path = %q, want default ./data/sqlflow.db", cfg.DB.Path)
		}
	})
	t.Run("default_jwt_expiry", func(t *testing.T) {
		if cfg.JWT.Expiry.String() != "24h0m0s" {
			t.Errorf("JWT.Expiry = %v, want default 24h", cfg.JWT.Expiry)
		}
	})
	t.Run("default_query_history_max", func(t *testing.T) {
		if cfg.QueryHistoryMax != 200 {
			t.Errorf("QueryHistoryMax = %d, want default 200", cfg.QueryHistoryMax)
		}
	})
	t.Run("default_admin_username", func(t *testing.T) {
		// When username is not set, it should default to "admin"
		dir2 := writeTestConfig(t, `
jwt:
  secret: "a-very-long-secret-key-for-testing!!"
admin:
  password: "strongpassword"
encryption_key: "0123456789abcdef0123456789abcdef"
`)
		cfg2, err := Load(dir2)
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}
		if cfg2.Admin.Username != "admin" {
			t.Errorf("Admin.Username = %q, want default %q", cfg2.Admin.Username, "admin")
		}
	})
}

func TestLoad_NoConfigFile(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("Load() should fail when config file is missing")
	}
}
