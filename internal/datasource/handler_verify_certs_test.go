package datasource

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
)

// createDatasourceViaHandler posts a create request and returns the stored row.
func createDatasourceViaHandler(t *testing.T, e *echo.Echo, h *Handler, svc *Service, body string) *model.DataSource {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/datasources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.CreateDatasource(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}

	list, err := svc.ListDataSources(t.Context())
	if err != nil {
		t.Fatalf("list datasources: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("datasources = %d, want 1", len(list))
	}
	return &list[0]
}

// TestCreateDatasourceVerifyCertsDefaultsOn pins the default for an omitted
// field.
//
// es_verify_certs was a plain bool, so a request that left it out bound to
// false and was written to the row as false — the column's own DEFAULT TRUE
// never applied, because the insert named the column. The driver then set
// InsecureSkipVerify, which accepts any certificate and hands the basic-auth
// credentials to whoever answers.
//
// The browser always sends the field, so only non-browser clients were
// affected. That is precisely the case the server cannot delegate: a check
// that holds only when the caller is well-behaved is not a check.
func TestCreateDatasourceVerifyCertsDefaultsOn(t *testing.T) {
	e, svc, h := setupDatasourceTest(t)

	stored := createDatasourceViaHandler(t, e, h, svc,
		`{"name":"logs-es","type":"elasticsearch","extra_config":{"urls":["https://es.example.com:9200"],"auth_type":"none"}}`)

	if !decodedVerifyCerts(t, stored) {
		t.Error("verify_certs resolved to false for a request that omitted it — certificate verification was silently disabled")
	}
}

// decodedVerifyCerts asks the driver what the stored configuration means.
//
// The default lives in the Elasticsearch driver's DecodeConfig rather than in a
// storage column, so reading the row is not enough — the question is what the
// connection will be built with.
func decodedVerifyCerts(t *testing.T, ds *model.DataSource) bool {
	t.Helper()
	cfg, err := driver.BuildConfigFromDataSource(NewAdapter(ds), driver.Secrets{})
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	v, ok := cfg.Extra["verify_certs"].(bool)
	if !ok {
		t.Fatal("verify_certs is absent from the built config")
	}
	return v
}

// TestCreateDatasourceVerifyCertsCanBeDisabled checks the default is a default
// and not a lock: self-signed certificates in a lab are a legitimate case, and
// it has to stay expressible.
func TestCreateDatasourceVerifyCertsCanBeDisabled(t *testing.T) {
	e, svc, h := setupDatasourceTest(t)

	stored := createDatasourceViaHandler(t, e, h, svc,
		`{"name":"lab-es","type":"elasticsearch","extra_config":{"urls":["https://es.lab:9200"],"auth_type":"none","verify_certs":false}}`)

	if decodedVerifyCerts(t, stored) {
		t.Error("verify_certs resolved to true although the request explicitly set false")
	}
}

// TestUpdateDatasourceVerifyCertsFallsBackToOn covers the quieter half.
//
// extra_config is replaced whole on update, so an update that does not mention
// verify_certs leaves it unset and the driver's default applies. That default
// is on, which is the direction this has to fail in: the old behavior turned
// verification off on a datasource that had it on, with nothing in the request
// expressing that intent.
func TestUpdateDatasourceVerifyCertsFallsBackToOn(t *testing.T) {
	e, svc, h := setupDatasourceTest(t)

	stored := createDatasourceViaHandler(t, e, h, svc,
		`{"name":"logs-es","type":"elasticsearch","extra_config":{"urls":["https://es.example.com:9200"],"auth_type":"none","verify_certs":true}}`)

	body := `{"name":"logs-es","type":"elasticsearch","extra_config":{"urls":["https://es.example.com:9200"],"auth_type":"none"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/datasources/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(stored.ID, 10))

	if err := h.UpdateDatasource(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	after, err := svc.GetDataSource(t.Context(), stored.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !decodedVerifyCerts(t, after) {
		t.Error("an update that never mentioned verify_certs turned verification off")
	}
}
