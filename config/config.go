package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server          ServerConfig `mapstructure:"server"`
	JWT             JWTConfig    `mapstructure:"jwt"`
	Admin           AdminConfig  `mapstructure:"admin"`
	DB              DBConfig     `mapstructure:"db"`
	QueryHistoryMax int          `mapstructure:"query_history_max"`
	EncryptionKey   string       `mapstructure:"encryption_key"`
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
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.JWT.Secret == "" {
		log.Println("[WARNING] jwt.secret is not set; generating a random secret (tokens will invalidate on restart). Set jwt.secret in config.yaml for production.")
		key := make([]byte, 32)
		_, _ = rand.Read(key)
		cfg.JWT.Secret = hex.EncodeToString(key)
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
		key := make([]byte, 16)
		_, _ = rand.Read(key)
		cfg.EncryptionKey = hex.EncodeToString(key)
	}

	return &cfg, nil
}
