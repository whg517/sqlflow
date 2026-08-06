package datasource

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/testutil"
)

const esPlaintextAPIKey = "cGxhaW50ZXh0LWFwaS1rZXk="

// seedESDatasource stores an Elasticsearch datasource authenticating by API key.
func seedESDatasource(t *testing.T, svc *Service) *model.DataSource {
	t.Helper()
	ds := &model.DataSource{
		Name: "logs-es",
		Type: "elasticsearch",
		// The handler fills these for ES because the columns are NOT NULL;
		// connection details come from extra_config.
		Host:        "elasticsearch",
		Port:        9200,
		ExtraConfig: `{"urls":["https://es.example.com:9200"],"auth_type":"api_key","verify_certs":true}`,
		ESApiKey:    esPlaintextAPIKey,
	}
	if err := svc.CreateDataSource(t.Context(), ds); err != nil {
		t.Fatalf("create es datasource: %v", err)
	}
	stored, err := svc.GetDataSource(t.Context(), ds.ID)
	if err != nil {
		t.Fatalf("read back es datasource: %v", err)
	}
	return stored
}

// TestDecryptSecretsReturnsPlaintextAPIKey pins the round trip.
//
// GetDataSource returns the stored ciphertext for both secrets. The password
// had a decryption step at every call site; the API key did not, so it reached
// the driver encrypted and Elasticsearch answered 401. Only TestConnection
// decrypted it, which is why the connection test passed and every real query
// failed — the hardest shape of bug to trace back.
func TestDecryptSecretsReturnsPlaintextAPIKey(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	stored := seedESDatasource(t, svc)

	if stored.ESApiKey == esPlaintextAPIKey {
		t.Fatal("the api key was stored in plaintext — this test cannot prove anything")
	}

	secrets, err := DecryptSecrets(stored, testutil.EncryptionKey)
	if err != nil {
		t.Fatalf("DecryptSecrets: %v", err)
	}
	if secrets.APIKey != esPlaintextAPIKey {
		t.Errorf("api key = %q, want the plaintext %q", secrets.APIKey, esPlaintextAPIKey)
	}
}

// TestBuildConfigCarriesPlaintextAPIKey checks the value the driver actually
// receives, which is the thing Elasticsearch authenticates with.
func TestBuildConfigCarriesPlaintextAPIKey(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	stored := seedESDatasource(t, svc)

	secrets, err := DecryptSecrets(stored, testutil.EncryptionKey)
	if err != nil {
		t.Fatalf("DecryptSecrets: %v", err)
	}
	cfg, err := driver.BuildConfigFromDataSource(NewAdapter(stored), secrets)
	if err != nil {
		t.Fatalf("BuildConfigFromDataSource: %v", err)
	}

	got, _ := cfg.Extra["api_key"].(string)
	if got != esPlaintextAPIKey {
		t.Errorf("cfg.Extra[api_key] = %q, want the plaintext %q — the driver would send ciphertext to Elasticsearch", got, esPlaintextAPIKey)
	}
}

// TestDecryptSecretsOnDatasourceWithoutAPIKey guards the common case: most
// datasources have no API key at all, and an empty field must not be treated as
// ciphertext.
func TestDecryptSecretsOnDatasourceWithoutAPIKey(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ds := &model.DataSource{
		Name: "app-mysql", Type: "mysql", Host: "localhost", Port: 3306,
		Username: "root", PasswordEncrypted: "s3cret", Database: "app",
	}
	if err := svc.CreateDataSource(t.Context(), ds); err != nil {
		t.Fatalf("create mysql datasource: %v", err)
	}
	stored, err := svc.GetDataSource(t.Context(), ds.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	secrets, err := DecryptSecrets(stored, testutil.EncryptionKey)
	if err != nil {
		t.Fatalf("DecryptSecrets: %v", err)
	}
	if secrets.Password != "s3cret" {
		t.Errorf("password = %q, want s3cret", secrets.Password)
	}
	if secrets.APIKey != "" {
		t.Errorf("api key = %q, want empty", secrets.APIKey)
	}
}
