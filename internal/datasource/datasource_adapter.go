package datasource

import "github.com/whg517/sqlflow/internal/model"

// Adapter adapts model.DataSource to implement driver.DataSourceInfo.
type Adapter struct {
	ds *model.DataSource
}

// NewAdapter creates a DataSourceInfo adapter from a model.DataSource.
func NewAdapter(ds *model.DataSource) *Adapter {
	return &Adapter{ds: ds}
}

func (a *Adapter) GetID() int64           { return a.ds.ID }
func (a *Adapter) GetType() string        { return a.ds.Type }
func (a *Adapter) GetHost() string        { return a.ds.Host }
func (a *Adapter) GetPort() int           { return a.ds.Port }
func (a *Adapter) GetUsername() string    { return a.ds.Username }
func (a *Adapter) GetDatabase() string    { return a.ds.Database }
func (a *Adapter) GetSSLMode() string     { return a.ds.SSLMode }
func (a *Adapter) GetSchemaName() string  { return a.ds.SchemaName }
func (a *Adapter) GetMaxOpen() int        { return a.ds.MaxOpen }
func (a *Adapter) GetMaxIdle() int        { return a.ds.MaxIdle }
func (a *Adapter) GetMaxLifetime() int    { return a.ds.MaxLifetime }
func (a *Adapter) GetMaxIdleTime() int    { return a.ds.MaxIdleTime }
func (a *Adapter) GetExtraConfig() string { return a.ds.ExtraConfig }

// GetExtra returns a driver-specific extra field value.
// First checks ExtraConfig JSON map, then falls back to struct fields.
func (a *Adapter) GetExtra(key string) string {
	switch key {
	case "es_urls":
		return a.ds.ESUrls
	case "es_version":
		return a.ds.ESVersion
	case "es_auth_type":
		return a.ds.ESAuthType
	case "es_api_key":
		return a.ds.ESApiKey
	case "es_index_pattern":
		return a.ds.ESIndexPattern
	case "mongo_uri":
		// Could be stored in ExtraConfig JSON in the future
		return ""
	default:
		return ""
	}
}

// GetExtraBool returns a driver-specific boolean extra field.
func (a *Adapter) GetExtraBool(key string, defaultVal bool) bool {
	switch key {
	case "es_verify_certs":
		return a.ds.ESVerifyCerts
	default:
		return defaultVal
	}
}
