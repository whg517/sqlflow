package config

import (
	"fmt"
	"log"
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
	QueryHistoryMax int            `mapstructure:"query_history_max"`
	EncryptionKey   string         `mapstructure:"encryption_key"`
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type JWTConfig struct {
	Secret string        `mapstructure:"secret"`
	Expiry time.Duration `mapstructure:"expiry"`
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
	WebhookURL string `mapstructure:"webhook_url"`
	Secret     string `mapstructure:"secret"`
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
		return nil, fmt.Errorf("jwt.secret is required and must not be empty (hint: set a random string of 16+ characters in config.yaml)")
	}
	if len(cfg.JWT.Secret) < 16 {
		return nil, fmt.Errorf("jwt.secret is too short (%d bytes), must be at least 16 bytes for security", len(cfg.JWT.Secret))
	}
	if cfg.JWT.Expiry == 0 {
		cfg.JWT.Expiry = 24 * time.Hour
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
		cfg.AI.Model = "gpt-4"
	}
	if cfg.AI.BaseURL == "" {
		cfg.AI.BaseURL = "https://api.openai.com/v1"
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

	return &cfg, nil
}
