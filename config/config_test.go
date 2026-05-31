package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// minimalValidConfig returns a minimal valid config YAML.
const minimalValidConfig = `
jwt:
  secret: "a-very-long-secret-key-for-testing!!"
  expiry: "1h"
admin:
  username: "admin"
  password: "strongpassword"
encryption_key: "0123456789abcdef0123456789abcdef"
`

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
	// Use minimal config WITHOUT jwt.expiry to test default
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
		if cfg.JWT.Expiry.String() != "15m0s" {
			t.Errorf("JWT.Expiry = %v, want default 15m", cfg.JWT.Expiry)
		}
	})
	t.Run("default_query_history_max", func(t *testing.T) {
		if cfg.QueryHistoryMax != 200 {
			t.Errorf("QueryHistoryMax = %d, want default 200", cfg.QueryHistoryMax)
		}
	})
	t.Run("default_admin_username", func(t *testing.T) {
		dir2 := writeTestConfig(t, minimalValidConfig)
		cfg2, err := Load(dir2)
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}
		if cfg2.Admin.Username != "admin" {
			t.Errorf("Admin.Username = %q, want default %q", cfg2.Admin.Username, "admin")
		}
	})
	t.Run("default_backup", func(t *testing.T) {
		if cfg.Backup.Interval != 6*time.Hour {
			t.Errorf("Backup.Interval = %v, want default 6h", cfg.Backup.Interval)
		}
		if cfg.Backup.Dir != "./data/backups" {
			t.Errorf("Backup.Dir = %q, want default ./data/backups", cfg.Backup.Dir)
		}
		if cfg.Backup.Keep != 10 {
			t.Errorf("Backup.Keep = %d, want default 10", cfg.Backup.Keep)
		}
	})
	t.Run("default_ai", func(t *testing.T) {
		if cfg.AI.Provider != "openai" {
			t.Errorf("AI.Provider = %q, want default openai", cfg.AI.Provider)
		}
		if cfg.AI.Model != "gpt-4" {
			t.Errorf("AI.Model = %q, want default gpt-4", cfg.AI.Model)
		}
		if cfg.AI.Timeout != 10*time.Second {
			t.Errorf("AI.Timeout = %v, want default 10s", cfg.AI.Timeout)
		}
	})
	t.Run("default_metrics", func(t *testing.T) {
		if cfg.Metrics.Port != 9090 {
			t.Errorf("Metrics.Port = %d, want default 9090", cfg.Metrics.Port)
		}
	})
}

func TestLoad_NoConfigFile(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("Load() should fail when config file is missing")
	}
}

// --- Environment Variable Override Tests ---

func TestLoad_EnvOverride_Port(t *testing.T) {
	t.Setenv("SQLFLOW_SERVER_PORT", "9999")
	dir := writeTestConfig(t, `
server:
  port: 8080
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
	if cfg.Server.Port != 9999 {
		t.Errorf("Server.Port = %d, want 9999 (from env)", cfg.Server.Port)
	}
}

func TestLoad_EnvOverride_DBPath(t *testing.T) {
	t.Setenv("SQLFLOW_DB_PATH", "/tmp/envtest.db")
	dir := writeTestConfig(t, minimalValidConfig)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.DB.Path != "/tmp/envtest.db" {
		t.Errorf("DB.Path = %q, want /tmp/envtest.db (from env)", cfg.DB.Path)
	}
}

func TestLoad_EnvOverride_JWTSecret(t *testing.T) {
	t.Setenv("SQLFLOW_JWT_SECRET", "env-override-secret-that-is-long-enough!!")
	dir := writeTestConfig(t, `
jwt:
  secret: "config-file-secret-value!!"
admin:
  password: "strongpassword"
encryption_key: "0123456789abcdef0123456789abcdef"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.JWT.Secret != "env-override-secret-that-is-long-enough!!" {
		t.Errorf("JWT.Secret = %q, want env override value", cfg.JWT.Secret)
	}
}

func TestLoad_EnvOverride_EncryptionKey(t *testing.T) {
	t.Setenv("SQLFLOW_ENCRYPTION_KEY", "abcdef0123456789abcdef0123456789")
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
	if cfg.EncryptionKey != "abcdef0123456789abcdef0123456789" {
		t.Errorf("EncryptionKey = %q, want env override value", cfg.EncryptionKey)
	}
}

func TestLoad_EnvOverride_AdminPassword(t *testing.T) {
	t.Setenv("SQLFLOW_ADMIN_PASSWORD", "env-override-password-123")
	dir := writeTestConfig(t, `
jwt:
  secret: "a-very-long-secret-key-for-testing!!"
admin:
  password: "config-password-here"
encryption_key: "0123456789abcdef0123456789abcdef"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Admin.Password != "env-override-password-123" {
		t.Errorf("Admin.Password = %q, want env override value", cfg.Admin.Password)
	}
}

func TestLoad_EnvOverride_AIConfig(t *testing.T) {
	t.Setenv("SQLFLOW_AI_PROVIDER", "zhipu")
	t.Setenv("SQLFLOW_AI_MODEL", "glm-4-flash")
	t.Setenv("SQLFLOW_AI_API_KEY", "test-api-key-from-env")
	t.Setenv("SQLFLOW_AI_BASE_URL", "https://custom.api.endpoint/v1")
	dir := writeTestConfig(t, minimalValidConfig)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.AI.Provider != "zhipu" {
		t.Errorf("AI.Provider = %q, want zhipu (from env)", cfg.AI.Provider)
	}
	if cfg.AI.Model != "glm-4-flash" {
		t.Errorf("AI.Model = %q, want glm-4-flash (from env)", cfg.AI.Model)
	}
	if cfg.AI.APIKey != "test-api-key-from-env" {
		t.Errorf("AI.APIKey = %q, want env override value", cfg.AI.APIKey)
	}
	if cfg.AI.BaseURL != "https://custom.api.endpoint/v1" {
		t.Errorf("AI.BaseURL = %q, want env override value", cfg.AI.BaseURL)
	}
}

func TestLoad_EnvOverride_BackupConfig(t *testing.T) {
	t.Setenv("SQLFLOW_BACKUP_ENABLED", "true")
	t.Setenv("SQLFLOW_BACKUP_DIR", "/tmp/backups")
	t.Setenv("SQLFLOW_BACKUP_KEEP", "5")
	t.Setenv("SQLFLOW_BACKUP_COMPRESS", "true")
	dir := writeTestConfig(t, minimalValidConfig)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if !cfg.Backup.Enabled {
		t.Error("Backup.Enabled = false, want true (from env)")
	}
	if cfg.Backup.Dir != "/tmp/backups" {
		t.Errorf("Backup.Dir = %q, want /tmp/backups (from env)", cfg.Backup.Dir)
	}
	if cfg.Backup.Keep != 5 {
		t.Errorf("Backup.Keep = %d, want 5 (from env)", cfg.Backup.Keep)
	}
	if !cfg.Backup.Compress {
		t.Error("Backup.Compress = false, want true (from env)")
	}
}

func TestLoad_EnvOverride_MetricsConfig(t *testing.T) {
	t.Setenv("SQLFLOW_METRICS_ENABLED", "true")
	t.Setenv("SQLFLOW_METRICS_PORT", "9191")
	dir := writeTestConfig(t, minimalValidConfig)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if !cfg.Metrics.Enabled {
		t.Error("Metrics.Enabled = false, want true (from env)")
	}
	if cfg.Metrics.Port != 9191 {
		t.Errorf("Metrics.Port = %d, want 9191 (from env)", cfg.Metrics.Port)
	}
}

func TestLoad_EnvOverride_NotifyConfig(t *testing.T) {
	t.Setenv("SQLFLOW_NOTIFY_WEBHOOK_URL", "https://hooks.example.com/notify")
	t.Setenv("SQLFLOW_NOTIFY_SECRET", "webhook-secret-from-env")
	dir := writeTestConfig(t, minimalValidConfig)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Notify.WebhookURL != "https://hooks.example.com/notify" {
		t.Errorf("Notify.WebhookURL = %q, want env override value", cfg.Notify.WebhookURL)
	}
	if cfg.Notify.Secret != "webhook-secret-from-env" {
		t.Errorf("Notify.Secret = %q, want env override value", cfg.Notify.Secret)
	}
}

func TestLoad_EnvOverride_FeishuConfig(t *testing.T) {
	t.Setenv("SQLFLOW_FEISHU_WEBHOOK_URL", "https://open.feishu.cn/open-apis/bot/v2/hook/test")
	dir := writeTestConfig(t, minimalValidConfig)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Feishu.WebhookURL != "https://open.feishu.cn/open-apis/bot/v2/hook/test" {
		t.Errorf("Feishu.WebhookURL = %q, want env override value", cfg.Feishu.WebhookURL)
	}
}

func TestLoad_EnvOverride_QueryHistoryMax(t *testing.T) {
	t.Setenv("SQLFLOW_QUERY_HISTORY_MAX", "500")
	dir := writeTestConfig(t, minimalValidConfig)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.QueryHistoryMax != 500 {
		t.Errorf("QueryHistoryMax = %d, want 500 (from env)", cfg.QueryHistoryMax)
	}
}

func TestLoad_EnvPriority_OverConfigFile(t *testing.T) {
	// Env var should override config.yaml value
	t.Setenv("SQLFLOW_SERVER_PORT", "7777")
	t.Setenv("SQLFLOW_JWT_SECRET", "env-wins-over-config-file-secret!!")
	dir := writeTestConfig(t, `
server:
  port: 9090
jwt:
  secret: "config-file-secret-that-is-very-long!!"
admin:
  password: "strongpassword"
encryption_key: "0123456789abcdef0123456789abcdef"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Server.Port != 7777 {
		t.Errorf("Server.Port = %d, want 7777 (env > config)", cfg.Server.Port)
	}
	if cfg.JWT.Secret != "env-wins-over-config-file-secret!!" {
		t.Errorf("JWT.Secret should be env value, got %q", cfg.JWT.Secret)
	}
}

func TestLoad_EnvOverride_TLSCertificate(t *testing.T) {
	t.Setenv("SQLFLOW_TLS_ENABLE", "true")
	t.Setenv("SQLFLOW_TLS_CERT_FILE", "/tmp/cert.pem")
	t.Setenv("SQLFLOW_TLS_KEY_FILE", "/tmp/key.pem")
	t.Setenv("SQLFLOW_TLS_REDIRECT_HTTP", "true")
	t.Setenv("SQLFLOW_TLS_HTTP_PORT", "80")
	dir := writeTestConfig(t, `
server:
  port: 8443
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
	if !cfg.Server.TLS.Enable {
		t.Error("TLS.Enable = false, want true (from env)")
	}
	if cfg.Server.TLS.CertFile != "/tmp/cert.pem" {
		t.Errorf("TLS.CertFile = %q, want /tmp/cert.pem (from env)", cfg.Server.TLS.CertFile)
	}
	if cfg.Server.TLS.KeyFile != "/tmp/key.pem" {
		t.Errorf("TLS.KeyFile = %q, want /tmp/key.pem (from env)", cfg.Server.TLS.KeyFile)
	}
	if !cfg.Server.TLS.RedirectHTTP {
		t.Error("TLS.RedirectHTTP = false, want true (from env)")
	}
	if cfg.Server.TLS.HTTPPort != 80 {
		t.Errorf("TLS.HTTPPort = %d, want 80 (from env)", cfg.Server.TLS.HTTPPort)
	}
}
