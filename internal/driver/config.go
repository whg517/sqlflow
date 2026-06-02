package driver

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BuildConfigFromDataSource creates a driver.Config from a DataSource-like struct.
// This function is the unified entry point for converting any datasource representation
// to the driver.Config used by the Driver interface.
//
// The ds parameter must implement the DataSourceInfo interface.
// Caller should ensure ds.GetType() returns a valid, registered driver type.
func BuildConfigFromDataSource(ds DataSourceInfo, password string, encryptionKey string) (*Config, error) {
	dsType := ds.GetType()
	if err := ValidateDriverType(dsType); err != nil {
		return nil, err
	}

	cfg := &Config{
		ID:          ds.GetID(),
		Host:        ds.GetHost(),
		Port:        ds.GetPort(),
		Username:    ds.GetUsername(),
		Password:    password,
		Database:    ds.GetDatabase(),
		SSLMode:     ds.GetSSLMode(),
		SchemaName:  ds.GetSchemaName(),
		MaxOpen:     ds.GetMaxOpen(),
		MaxIdle:     ds.GetMaxIdle(),
		MaxLifetime: time.Duration(ds.GetMaxLifetime()) * time.Second,
		MaxIdleTime: time.Duration(ds.GetMaxIdleTime()) * time.Second,
		Extra:       make(map[string]interface{}),
	}

	cfg.Extra["_type"] = dsType

	// Apply sensible defaults for pool settings
	if cfg.MaxOpen <= 0 {
		cfg.MaxOpen = 10
	}
	if cfg.MaxIdle <= 0 {
		cfg.MaxIdle = 5
	}
	if cfg.MaxLifetime <= 0 {
		cfg.MaxLifetime = 3600 * time.Second
	}
	if cfg.MaxIdleTime <= 0 {
		cfg.MaxIdleTime = 600 * time.Second
	}

	// Populate Extra based on datasource type
	// First try ExtraConfig JSON map, then fall back to legacy struct fields
	var extraMap map[string]interface{}
	if ds.GetExtraConfig() != "" {
		if err := json.Unmarshal([]byte(ds.GetExtraConfig()), &extraMap); err != nil {
			return nil, fmt.Errorf("invalid extra_config JSON: %w", err)
		}
	}

	switch dsType {
	case "elasticsearch":
		// ExtraConfig JSON takes priority
		if extraMap != nil {
			if v, ok := extraMap["urls"].(string); ok {
				cfg.Extra["urls"] = parseCSV(v)
			}
			if v, ok := extraMap["auth_type"].(string); ok {
				cfg.Extra["auth_type"] = v
			}
			if v, ok := extraMap["verify_certs"]; ok {
				cfg.Extra["verify_certs"] = v
			}
			if v, ok := extraMap["api_key"].(string); ok {
				cfg.Extra["api_key"] = v
			}
			if v, ok := extraMap["index_pattern"].(string); ok {
				cfg.Extra["index_pattern"] = v
			}
		} else {
			// Legacy fields fallback
			cfg.Extra["urls"] = parseCSV(ds.GetExtra("es_urls"))
			cfg.Extra["auth_type"] = ds.GetExtra("es_auth_type")
			cfg.Extra["verify_certs"] = ds.GetExtraBool("es_verify_certs", true)
			if apiKey := ds.GetExtra("es_api_key"); apiKey != "" {
				cfg.Extra["api_key"] = apiKey
			}
			if indexPattern := ds.GetExtra("es_index_pattern"); indexPattern != "" {
				cfg.Extra["index_pattern"] = indexPattern
			}
		}
	case "mongodb":
		if extraMap != nil {
			if v, ok := extraMap["uri"].(string); ok {
				cfg.Extra["uri"] = v
			}
		}
		if cfg.Extra["uri"] == nil {
			if uri := ds.GetExtra("mongo_uri"); uri != "" {
				cfg.Extra["uri"] = uri
			}
		}
	}

	return cfg, nil
}

// DataSourceInfo is the interface that datasource representations must implement
// to work with BuildConfigFromDataSource. This allows the driver package to remain
// decoupled from the specific model/ent types.
type DataSourceInfo interface {
	GetID() int64
	GetType() string
	GetHost() string
	GetPort() int
	GetUsername() string
	GetDatabase() string
	GetSSLMode() string
	GetSchemaName() string
	GetMaxOpen() int
	GetMaxIdle() int
	GetMaxLifetime() int
	GetMaxIdleTime() int
	GetExtra(key string) string
	GetExtraBool(key string, defaultVal bool) bool
	GetExtraConfig() string
}

// parseCSV splits a comma-separated string into a trimmed, non-empty slice.
func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// ValidateDriverType checks whether the given type string corresponds to a registered driver.
func ValidateDriverType(typeName string) error {
	if typeName == "" {
		return fmt.Errorf("datasource type is required")
	}
	if !IsRegistered(typeName) {
		return fmt.Errorf("unsupported datasource type: %s (supported: %v)", typeName, SupportedTypes())
	}
	return nil
}
