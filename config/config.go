package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
)

type Config struct {
	Server          ServerConfig   `mapstructure:"server"`
	JWT             JWTConfig      `mapstructure:"jwt"`
	Admin           AdminConfig    `mapstructure:"admin"`
	DB              DBConfig       `mapstructure:"db"`
	AI              AIConfig       `mapstructure:"ai"`
	DingTalk        DingTalkConfig `mapstructure:"dingtalk"`
	Backup          BackupConfig   `mapstructure:"backup"`
	QueryHistoryMax int            `mapstructure:"query_history_max"`
	EncryptionKey   string         `mapstructure:"encryption_key"`
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

type DBConfig struct {
	Path string `mapstructure:"path"`
}

// AIConfig holds AI review service configuration.
type AIConfig struct {
	Provider string        `mapstructure:"provider"`
	Model    string        `mapstructure:"model"`
	APIKey   string        `mapstructure:"api_key"`
	BaseURL  string        `mapstructure:"base_url"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// DingTalkConfig holds DingTalk notification configuration.
type DingTalkConfig struct {
	WebhookURL string              `mapstructure:"webhook_url"`
	Secret     string              `mapstructure:"secret"`
	OAuth      DingTalkOAuthConfig `mapstructure:"oauth"`
}

// DingTalkOAuthConfig holds DingTalk OAuth2 configuration for login.
type DingTalkOAuthConfig struct {
	AppKey      string `mapstructure:"app_key"`
	AppSecret   string `mapstructure:"app_secret"`
	RedirectURL string `mapstructure:"redirect_url"`
	Enabled     bool   `mapstructure:"enabled"`
}

// BackupConfig holds database backup configuration.
type BackupConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Dir      string        `mapstructure:"dir"`
	Interval time.Duration `mapstructure:"interval"`
	Keep     int           `mapstructure:"keep"`
	Compress bool          `mapstructure:"compress"`
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

	// Environment variable overrides
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SQLFLOW")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w (hint: copy config/config.example.yaml to config/config.yaml and fill in values)", err)
	}
	log.Printf("Loaded config from: %s", viper.ConfigFileUsed())

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// --- Env var overrides for secrets (highest priority) ---
	// These environment variables take precedence over config.yaml values.
	if v := os.Getenv("SQLFLOW_JWT_SECRET"); v != "" {
		cfg.JWT.Secret = v
	}
	if v := os.Getenv("SQLFLOW_ENCRYPTION_KEY"); v != "" {
		cfg.EncryptionKey = v
	}
	if v := os.Getenv("SQLFLOW_AI_API_KEY"); v != "" {
		cfg.AI.APIKey = v
	}
	if v := os.Getenv("SQLFLOW_ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	if v := os.Getenv("SQLFLOW_DINGTALK_SECRET"); v != "" {
		cfg.DingTalk.Secret = v
	}

	// Set defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.JWT.Secret == "" {
		return nil, fmt.Errorf("jwt.secret is required and must not be empty (hint: set a random string of 16+ characters in config.yaml)")
	}
	if len(cfg.JWT.Secret) < 16 {
		return nil, fmt.Errorf("jwt.secret is too short (%d bytes), must be at least 16 bytes for security", len(cfg.JWT.Secret))
	}
	if cfg.JWT.Expiry == 0 {
		cfg.JWT.Expiry = 15 * time.Minute
	}
	// Refresh token expiry defaults to 7 days
	if cfg.JWT.RefreshExpiry == 0 {
		cfg.JWT.RefreshExpiry = 7 * 24 * time.Hour
	}
	if cfg.DB.Path == "" {
		cfg.DB.Path = "./data/sqlflow.db"
	}
	if cfg.QueryHistoryMax == 0 {
		cfg.QueryHistoryMax = 200
	}
	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("encryption_key is required and must not be empty (hint: generate one with `openssl rand -hex 16` and set it in config.yaml)")
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
		return nil, fmt.Errorf("admin.password is required and must not be empty (hint: set a strong password in config.yaml)")
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
			return nil, fmt.Errorf("server.tls.cert_file is required when TLS is enabled")
		}
		if cfg.Server.TLS.KeyFile == "" {
			return nil, fmt.Errorf("server.tls.key_file is required when TLS is enabled")
		}
		if cfg.Server.TLS.RedirectHTTP && cfg.Server.TLS.HTTPPort == 0 {
			cfg.Server.TLS.HTTPPort = 80 // default HTTP redirect port
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
