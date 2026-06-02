package service

import "github.com/whg517/sqlflow/internal/model"

// dataSourceAdapter adapts model.DataSource to implement driver.DataSourceInfo.
type dataSourceAdapter struct {
	ds *model.DataSource
}

// newDataSourceAdapter creates a DataSourceInfo adapter from a model.DataSource.
func newDataSourceAdapter(ds *model.DataSource) *dataSourceAdapter {
	return &dataSourceAdapter{ds: ds}
}

func (a *dataSourceAdapter) GetID() int64                   { return a.ds.ID }
func (a *dataSourceAdapter) GetType() string                 { return a.ds.Type }
func (a *dataSourceAdapter) GetHost() string                { return a.ds.Host }
func (a *dataSourceAdapter) GetPort() int                   { return a.ds.Port }
func (a *dataSourceAdapter) GetUsername() string              { return a.ds.Username }
func (a *dataSourceAdapter) GetDatabase() string             { return a.ds.Database }
func (a *dataSourceAdapter) GetSSLMode() string               { return a.ds.SSLMode }
func (a *dataSourceAdapter) GetSchemaName() string            { return a.ds.SchemaName }
func (a *dataSourceAdapter) GetMaxOpen() int                  { return a.ds.MaxOpen }
func (a *dataSourceAdapter) GetMaxIdle() int                  { return a.ds.MaxIdle }
func (a *dataSourceAdapter) GetMaxLifetime() int             { return a.ds.MaxLifetime }
func (a *dataSourceAdapter) GetMaxIdleTime() int              { return a.ds.MaxIdleTime }
func (a *dataSourceAdapter) GetExtraConfig() string          { return a.ds.ExtraConfig }

// GetExtra returns a driver-specific extra field value.
// First checks ExtraConfig JSON map, then falls back to struct fields.
func (a *dataSourceAdapter) GetExtra(key string) string {
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
func (a *dataSourceAdapter) GetExtraBool(key string, defaultVal bool) bool {
	switch key {
	case "es_verify_certs":
		return a.ds.ESVerifyCerts
	default:
		return defaultVal
	}
}
