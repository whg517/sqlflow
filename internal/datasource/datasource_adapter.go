package datasource

import (
	"fmt"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/crypto"
)

// DecryptSecrets returns a stored datasource's credentials in plaintext.
//
// Both secrets are decrypted together on purpose. Callers used to decrypt the
// password themselves and never think about the API key, which reached the
// Elasticsearch driver still encrypted: the connection test passed (it had its
// own decryption step) while every real query got a 401. Returning them as a
// pair means adding a third secret later shows up as a compile error at every
// call site instead of as another silent 401.
func DecryptSecrets(ds *model.DataSource, encryptionKey string) (driver.Secrets, error) {
	var secrets driver.Secrets
	if ds.PasswordEncrypted != "" {
		password, err := crypto.Decrypt(ds.PasswordEncrypted, encryptionKey)
		if err != nil {
			return driver.Secrets{}, fmt.Errorf("decrypt password: %w", err)
		}
		secrets.Password = password
	}
	if ds.ESApiKey != "" {
		apiKey, err := crypto.Decrypt(ds.ESApiKey, encryptionKey)
		if err != nil {
			return driver.Secrets{}, fmt.Errorf("decrypt es_api_key: %w", err)
		}
		secrets.APIKey = apiKey
	}
	return secrets, nil
}

// Adapter adapts model.DataSource to implement driver.DataSourceInfo.
type Adapter struct {
	ds *model.DataSource
}

// NewAdapter creates a DataSourceInfo adapter from a model.DataSource.
func NewAdapter(ds *model.DataSource) *Adapter {
	return &Adapter{ds: ds}
}

func (a *Adapter) GetID() int64          { return a.ds.ID }
func (a *Adapter) GetType() string       { return a.ds.Type }
func (a *Adapter) GetHost() string       { return a.ds.Host }
func (a *Adapter) GetPort() int          { return a.ds.Port }
func (a *Adapter) GetUsername() string   { return a.ds.Username }
func (a *Adapter) GetDatabase() string   { return a.ds.Database }
func (a *Adapter) GetSSLMode() string    { return a.ds.SSLMode }
func (a *Adapter) GetSchemaName() string { return a.ds.SchemaName }
func (a *Adapter) GetMaxOpen() int       { return a.ds.MaxOpen }
func (a *Adapter) GetMaxIdle() int       { return a.ds.MaxIdle }
func (a *Adapter) GetMaxLifetime() int   { return a.ds.MaxLifetime }
func (a *Adapter) GetMaxIdleTime() int   { return a.ds.MaxIdleTime }

// GetExtraConfig hands the raw JSON object to the driver, which decodes it.
//
// Two methods used to sit here: GetExtra(key) and GetExtraBool(key, default),
// each a switch over driver-specific key names. This package answered "es_urls"
// and "es_verify_certs" without knowing what they meant, returned "" with a
// TODO for "mongo_uri", and GetExtraBool ignored the default it was handed —
// which is how es_verify_certs came to fail open.
func (a *Adapter) GetExtraConfig() string { return a.ds.ExtraConfig }
