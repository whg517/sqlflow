package driver

import (
	"encoding/json"
	"fmt"
	"time"
)

// Secrets are a datasource's credentials, in plaintext, ready to hand to a
// driver.
//
// They travel as a struct rather than as loose string parameters so that
// adding one is a compile error at every call site. Two loose strings would
// have let the old `BuildConfigFromDataSource(adapter, password, "")` calls
// keep compiling unchanged, which is exactly how the API key ended up reaching
// Elasticsearch encrypted.
type Secrets struct {
	// Password is the decrypted password. Empty for datasources without one.
	Password string
	// APIKey is the decrypted Elasticsearch API key. Empty for every other type
	// and for ES datasources that authenticate some other way.
	APIKey string
}

// BuildConfigFromDataSource creates a driver.Config from a DataSource-like struct.
// This function is the unified entry point for converting any datasource representation
// to the driver.Config used by the Driver interface.
//
// The ds parameter must implement the DataSourceInfo interface.
// Caller should ensure ds.GetType() returns a valid, registered driver type.
//
// Credentials come from secrets rather than off ds, because ds carries them as
// stored — encrypted. The API key used to be read through
// ds.GetExtra("es_api_key") and sent to Elasticsearch as ciphertext, while the
// third parameter here was named encryptionKey and never used at all.
func BuildConfigFromDataSource(ds DataSourceInfo, secrets Secrets) (*Config, error) {
	dsType := ds.GetType()
	d, err := NewDriver(dsType)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		ID:          ds.GetID(),
		Host:        ds.GetHost(),
		Port:        ds.GetPort(),
		Username:    ds.GetUsername(),
		Password:    secrets.Password,
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

	// Driver-specific settings are decoded by the driver.
	//
	// This was a `switch dsType` with a branch per driver, which made adding a
	// data source type a change to this file — and Elasticsearch went further,
	// into six dedicated columns, the model, the adapter, the request structs
	// and the form. Only the driver knows what its own keys mean, which is the
	// same argument ConfigValidator already makes for checking them.
	if decoder, ok := d.(ConfigDecoder); ok {
		var extra map[string]interface{}
		if raw := ds.GetExtraConfig(); raw != "" {
			if err := json.Unmarshal([]byte(raw), &extra); err != nil {
				return nil, fmt.Errorf("invalid extra_config JSON: %w", err)
			}
		}
		if extra == nil {
			extra = map[string]interface{}{}
		}
		// Secrets never travel through extra_config: it holds what was stored,
		// and the stored copy is encrypted. Only the caller can decrypt.
		if secrets.APIKey != "" {
			cfg.Extra["api_key"] = secrets.APIKey
		}
		if err := decoder.DecodeConfig(cfg, extra); err != nil {
			return nil, fmt.Errorf("%s config: %w", dsType, err)
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
	// GetExtraConfig returns the raw extra_config JSON object. It is opaque
	// here: GetExtra(key) and GetExtraBool(key, default) used to sit beside it,
	// which forced every implementer to know each driver's key names — the
	// adapter answered "es_urls" and "mongo_uri" by hand, and returned "" with
	// a TODO for the keys nobody had wired yet.
	GetExtraConfig() string
}

// ConfigDecoder is implemented by drivers that need settings beyond host,
// port, credentials and database.
//
// It is the symmetric half of ConfigValidator: validation belongs to the driver
// because only it knows what its fields mean, and decoding them is the same
// argument. The map is the datasource's extra_config object, already parsed;
// the driver takes what it recognizes and applies its own defaults for what is
// missing.
//
// Errors are for malformed values, not absent ones. A datasource can be saved
// before it is fully configured, and refusing to build a Config would turn that
// into a failure at connect time with a message about JSON.
type ConfigDecoder interface {
	DecodeConfig(cfg *Config, extra map[string]interface{}) error
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
