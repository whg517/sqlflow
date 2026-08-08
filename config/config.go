package config

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/viper"
	"github.com/whg517/sqlflow/internal/platform/crypto"
)

type Config struct {
	Server          ServerConfig  `mapstructure:"server"`
	JWT             JWTConfig     `mapstructure:"jwt"`
	Admin           AdminConfig   `mapstructure:"admin"`
	DB              DBConfig      `mapstructure:"db"`
	AI              AIConfig      `mapstructure:"ai"`
	Notify          NotifyConfig  `mapstructure:"notify"`
	Feishu          FeishuConfig  `mapstructure:"feishu"`
	OIDC            OIDCConfig    `mapstructure:"oidc"`
	Backup          BackupConfig  `mapstructure:"backup"`
	Metrics         MetricsConfig `mapstructure:"metrics"`
	Query           QueryConfig   `mapstructure:"query"`
	QueryHistoryMax int           `mapstructure:"query_history_max"`
	EncryptionKey   string        `mapstructure:"encryption_key"`
}

type ServerConfig struct {
	Port int       `mapstructure:"port"`
	TLS  TLSConfig `mapstructure:"tls"`
}

// TLSConfig holds HTTPS/TLS configuration.
// When Enable is true, the server listens with TLS and optionally redirects HTTP traffic.
type TLSConfig struct {
	Enable       bool   `mapstructure:"enable"`
	CertFile     string `mapstructure:"cert_file"`
	KeyFile      string `mapstructure:"key_file"`
	RedirectHTTP bool   `mapstructure:"redirect_http"`
	HTTPPort     int    `mapstructure:"http_port"` // port for HTTP→HTTPS redirect listener
}

type JWTConfig struct {
	Secret        string        `mapstructure:"secret"`
	Expiry        time.Duration `mapstructure:"expiry"`
	RefreshExpiry time.Duration `mapstructure:"refresh_expiry"`
}

type AdminConfig struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// DBConfig configures the platform metadata store (PostgreSQL, see ADR-0009).
type DBConfig struct {
	// DSN is a libpq-style connection string, e.g.
	// postgres://user:pass@host:5432/sqlflow?sslmode=disable
	DSN string `mapstructure:"dsn"`

	// DataDir holds files the platform writes to disk — async export artifacts
	// and backup dumps. It used to be derived from the SQLite file's path, which
	// conflated "where the database lives" with "where our files go"; those were
	// never the same thing, and with PostgreSQL they cannot be.
	DataDir string `mapstructure:"data_dir"`
}

// AIConfig holds AI review service configuration.
type AIConfig struct {
	Provider string        `mapstructure:"provider"`
	Model    string        `mapstructure:"model"`
	APIKey   string        `mapstructure:"api_key"`
	BaseURL  string        `mapstructure:"base_url"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// NotifyConfig holds webhook notification configuration.
type NotifyConfig struct {
	WebhookURL string `mapstructure:"webhook_url"`
	Secret     string `mapstructure:"secret"`
}

// FeishuConfig holds Feishu webhook notification configuration.
type FeishuConfig struct {
	WebhookURL string `mapstructure:"webhook_url"`
}

// OIDCConfig holds OpenID Connect configuration.
type OIDCConfig struct {
	Providers []OIDCProviderConfig `mapstructure:"providers"`
}

// OIDCProviderConfig holds a single OIDC IdP configuration.
type OIDCProviderConfig struct {
	Name         string `mapstructure:"name"`
	Issuer       string `mapstructure:"issuer"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
	Scopes       string `mapstructure:"scopes"`
	Enabled      bool   `mapstructure:"enabled"`
}

// MetricsConfig holds Prometheus metrics configuration.
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Port    int  `mapstructure:"port"`
}

// QueryConfig bounds what a single query may consume.
//
// These are the operator's numbers, not the driver's: how long a user may hold
// a connection and how much they may pull back is a platform policy that has to
// read the same for every data source. Left to the drivers it did not — three
// of five imposed no execution deadline at all.
//
// Every field may be omitted; the query service substitutes the values that
// were previously compiled in, so an existing config keeps its behavior.
type QueryConfig struct {
	Timeout       time.Duration `mapstructure:"timeout"`
	MaxRows       int           `mapstructure:"max_rows"`
	ExportMaxRows int           `mapstructure:"export_max_rows"`
	ShareMaxRows  int           `mapstructure:"share_max_rows"`
}

// BackupConfig holds database backup configuration.
type BackupConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Dir      string        `mapstructure:"dir"`
	Interval time.Duration `mapstructure:"interval"`
	Keep     int           `mapstructure:"keep"`
	Compress bool          `mapstructure:"compress"`
}

// envBindings maps viper config keys to their SQLFLOW_ prefixed environment variables.
// Viper's AutomaticEnv with prefix "SQLFLOW" replaces "." with "_" and uppercases,
// so "server.port" → "SQLFLOW_SERVER_PORT" works automatically for top-level and
// one-level-nested keys. For deeper nesting or explicit aliases, we use BindEnv.
var envBindings = map[string]string{
	// Server
	"server.port":              "SQLFLOW_SERVER_PORT",
	"server.tls.enable":        "SQLFLOW_TLS_ENABLE",
	"server.tls.cert_file":     "SQLFLOW_TLS_CERT_FILE",
	"server.tls.key_file":      "SQLFLOW_TLS_KEY_FILE",
	"server.tls.redirect_http": "SQLFLOW_TLS_REDIRECT_HTTP",
	"server.tls.http_port":     "SQLFLOW_TLS_HTTP_PORT",
	// JWT
	"jwt.secret":         "SQLFLOW_JWT_SECRET",
	"jwt.expiry":         "SQLFLOW_JWT_EXPIRY",
	"jwt.refresh_expiry": "SQLFLOW_JWT_REFRESH_EXPIRY",
	// Admin
	"admin.username": "SQLFLOW_ADMIN_USERNAME",
	"admin.password": "SQLFLOW_ADMIN_PASSWORD",
	// DB
	"db.dsn":      "SQLFLOW_DB_DSN",
	"db.data_dir": "SQLFLOW_DB_DATA_DIR",
	// AI
	"ai.provider": "SQLFLOW_AI_PROVIDER",
	"ai.model":    "SQLFLOW_AI_MODEL",
	"ai.api_key":  "SQLFLOW_AI_API_KEY",
	"ai.base_url": "SQLFLOW_AI_BASE_URL",
	"ai.timeout":  "SQLFLOW_AI_TIMEOUT",
	// Notify
	"notify.webhook_url": "SQLFLOW_NOTIFY_WEBHOOK_URL",
	"notify.secret":      "SQLFLOW_NOTIFY_SECRET",
	// Feishu
	"feishu.webhook_url": "SQLFLOW_FEISHU_WEBHOOK_URL",
	// Backup
	"backup.enabled":  "SQLFLOW_BACKUP_ENABLED",
	"backup.dir":      "SQLFLOW_BACKUP_DIR",
	"backup.interval": "SQLFLOW_BACKUP_INTERVAL",
	"backup.keep":     "SQLFLOW_BACKUP_KEEP",
	"backup.compress": "SQLFLOW_BACKUP_COMPRESS",
	// Metrics
	"metrics.enabled": "SQLFLOW_METRICS_ENABLED",
	"metrics.port":    "SQLFLOW_METRICS_PORT",
	// Query
	"query.timeout":         "SQLFLOW_QUERY_TIMEOUT",
	"query.max_rows":        "SQLFLOW_QUERY_MAX_ROWS",
	"query.export_max_rows": "SQLFLOW_QUERY_EXPORT_MAX_ROWS",
	"query.share_max_rows":  "SQLFLOW_QUERY_SHARE_MAX_ROWS",
	// Top-level
	"query_history_max": "SQLFLOW_QUERY_HISTORY_MAX",
	"encryption_key":    "SQLFLOW_ENCRYPTION_KEY",
}

func Load(configPath string) (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	if configPath != "" {
		viper.AddConfigPath(configPath)
	} else {
		viper.AddConfigPath(".")
		viper.AddConfigPath("./config")
	}

	// Enable environment variable support with SQLFLOW_ prefix.
	// Viper will automatically look for SQLFLOW_<KEY> for each config key,
	// replacing "." with "_" and uppercasing.
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SQLFLOW")

	// Bind explicit env aliases for clarity and discoverability.
	// This also handles any keys where the automatic mapping might not match.
	for key, envVar := range envBindings {
		if err := viper.BindEnv(key, envVar); err != nil {
			return nil, fmt.Errorf("bind env %s: %w", envVar, err)
		}
	}

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w (hint: copy config/config.example.yaml to config/config.yaml and fill in values)", err)
	}
	log.Printf("Loaded config from: %s", viper.ConfigFileUsed())

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.JWT.Secret == "" {
		return nil, fmt.Errorf("jwt.secret is required and must not be empty (hint: set SQLFLOW_JWT_SECRET env var or configure in config.yaml)")
	}
	if len(cfg.JWT.Secret) < 16 {
		return nil, fmt.Errorf("jwt.secret is too short (%d bytes), must be at least 16 bytes for security", len(cfg.JWT.Secret))
	}
	if cfg.JWT.Expiry == 0 {
		cfg.JWT.Expiry = 15 * time.Minute
	}
	if cfg.JWT.RefreshExpiry == 0 {
		cfg.JWT.RefreshExpiry = 7 * 24 * time.Hour
	}
	if cfg.DB.DSN == "" {
		cfg.DB.DSN = "postgres://sqlflow:sqlflow@localhost:5432/sqlflow?sslmode=disable"
	}
	if cfg.DB.DataDir == "" {
		cfg.DB.DataDir = "./data"
	}
	if cfg.QueryHistoryMax == 0 {
		cfg.QueryHistoryMax = 200
	}
	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("encryption_key is required and must not be empty (hint: set SQLFLOW_ENCRYPTION_KEY env var or generate one with `openssl rand -hex 16`)")
	}
	if err := crypto.ValidateKey(cfg.EncryptionKey); err != nil {
		return nil, fmt.Errorf("invalid encryption_key: %w (must be 16, 24, or 32 bytes for AES-128/192/256)", err)
	}
	if cfg.AI.Provider == "" {
		cfg.AI.Provider = "openai"
	}
	if cfg.AI.Model == "" {
		cfg.AI.Model = defaultModelForProvider(cfg.AI.Provider)
	}
	if cfg.AI.BaseURL == "" {
		cfg.AI.BaseURL = defaultBaseURLForProvider(cfg.AI.Provider)
	}
	if cfg.AI.Timeout == 0 {
		cfg.AI.Timeout = 10 * time.Second
	}

	// Admin password validation
	if cfg.Admin.Password == "" {
		return nil, fmt.Errorf("admin.password is required and must not be empty (hint: set SQLFLOW_ADMIN_PASSWORD env var or configure in config.yaml)")
	}
	if len(cfg.Admin.Password) < 8 {
		return nil, fmt.Errorf("admin.password is too short (%d bytes), must be at least 8 characters for security", len(cfg.Admin.Password))
	}
	if cfg.Admin.Password == "admin123" {
		log.Printf("[WARN] admin password is set to default 'admin123', please change it immediately")
	}
	if cfg.Admin.Username == "" {
		cfg.Admin.Username = "admin"
	}
	if cfg.AI.APIKey == "" {
		log.Printf("[WARN] ai.api_key is empty, AI review will be unavailable")
	}

	// TLS validation
	if cfg.Server.TLS.Enable {
		if cfg.Server.TLS.CertFile == "" {
			return nil, fmt.Errorf("server.tls.cert_file is required when TLS is enabled (hint: set SQLFLOW_TLS_CERT_FILE)")
		}
		if cfg.Server.TLS.KeyFile == "" {
			return nil, fmt.Errorf("server.tls.key_file is required when TLS is enabled (hint: set SQLFLOW_TLS_KEY_FILE)")
		}
		if cfg.Server.TLS.RedirectHTTP && cfg.Server.TLS.HTTPPort == 0 {
			cfg.Server.TLS.HTTPPort = 80
		}
		if cfg.Server.TLS.HTTPPort == cfg.Server.Port {
			return nil, fmt.Errorf("server.tls.http_port (%d) must differ from server.port (%d)", cfg.Server.TLS.HTTPPort, cfg.Server.Port)
		}
		log.Printf("[INFO] TLS enabled, cert=%s, key=%s", cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		if cfg.Server.TLS.RedirectHTTP {
			log.Printf("[INFO] HTTP→HTTPS redirect enabled on port %d", cfg.Server.TLS.HTTPPort)
		}
	}

	// Backup defaults
	if cfg.Backup.Interval == 0 {
		cfg.Backup.Interval = 6 * time.Hour
	}
	if cfg.Backup.Dir == "" {
		cfg.Backup.Dir = "./data/backups"
	}
	if cfg.Backup.Keep == 0 {
		cfg.Backup.Keep = 10
	}

	// Metrics defaults
	if cfg.Metrics.Port == 0 {
		cfg.Metrics.Port = 9090
	}

	return &cfg, nil
}

// defaultModelForProvider returns the default model for a given provider.
func defaultModelForProvider(provider string) string {
	switch provider {
	case "zhipu":
		return "glm-4"
	case "azure":
		return "gpt-4"
	default:
		return "gpt-4"
	}
}

// defaultBaseURLForProvider returns the default API base URL for a given provider.
func defaultBaseURLForProvider(provider string) string {
	switch provider {
	case "zhipu":
		return "https://open.bigmodel.cn/paas/v4"
	case "azure":
		return ""
	default:
		return "https://api.openai.com/v1"
	}
}
