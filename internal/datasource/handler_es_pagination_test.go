package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/labstack/echo/v4"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/platform/httpx"
	"github.com/whg517/sqlflow/internal/testutil"
)

// allowNamed reports objects visible only when their name is in the set.
type allowNamed map[string]bool

func (a allowNamed) CanViewObject(_ context.Context, _ int64, _, _, object string) (bool, error) {
	return a[object], nil
}

// setupESIndexHandler wires a datasource whose cluster reports indexCount
// indices, with an injected client so no real Elasticsearch is needed.
func setupESIndexHandler(t *testing.T, indexCount int, checker ObjectViewChecker) (*echo.Echo, *Handler, int64) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entries := make([]map[string]interface{}, 0, indexCount)
		for i := range indexCount {
			entries = append(entries, map[string]interface{}{
				"index": fmt.Sprintf("logs-%03d", i), "health": "green", "status": "open",
				"store.size": "4.2mb", "docs.count": "10",
			})
		}
		// The v8 client refuses a response without this header.
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	}))
	t.Cleanup(server.Close)

	client, err := es.NewClient(es.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatalf("create ES client: %v", err)
	}

	testDB := setupDatasourceTestDB(t)
	connMgr := connpool.NewManager()
	svc := NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, connMgr, driver.NewPoolManager(), auditlog.Discard)

	ds := &model.DataSource{
		Name: "logs-es", Type: "elasticsearch",
		Host: "elasticsearch", Port: 9200,
		ExtraConfig: `{"urls":["` + server.URL + `"],"auth_type":"none","verify_certs":true}`,
	}
	if err := svc.CreateDataSource(t.Context(), ds); err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	connMgr.InjectESForTest(ds.ID, []string{server.URL}, client)

	h := NewHandler(svc, checker)
	return echo.New(), h, ds.ID
}

// callESIndices runs the handler and decodes its envelope.
func callESIndices(t *testing.T, e *echo.Echo, h *Handler, dsID int64, page, pageSize int) (items []map[string]interface{}, total float64) {
	t.Helper()
	target := fmt.Sprintf("/api/datasources/%d/es-indices?page=%d&page_size=%d", dsID, page, pageSize)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", dsID))
	c.Set(httpx.ContextKeyUserID, int64(1))
	c.Set(httpx.ContextKeyRole, "admin")
	c.SetPath("/api/datasources/:id/es-indices")
	_ = url.Values{}

	if err := h.GetESIndices(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Items []map[string]interface{} `json:"items"`
			Total float64                  `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Data.Items, body.Data.Total
}

// TestESIndicesTotalCountsEveryVisibleIndex pins what a pager needs.
//
// The handler discarded the total the service computed and reported
// len(visible) — the size of the page it had just filtered. With 200 indices
// and page_size 20 the response said total 20, so the UI drew one page and the
// other 180 indices were unreachable by any input the user could give.
func TestESIndicesTotalCountsEveryVisibleIndex(t *testing.T) {
	e, h, dsID := setupESIndexHandler(t, 200, allowAll{})

	items, total := callESIndices(t, e, h, dsID, 1, 20)

	if len(items) != 20 {
		t.Errorf("page holds %d indices, want 20", len(items))
	}
	if total != 200 {
		t.Errorf("total = %v, want 200 — a pager cannot reach page 2 otherwise", total)
	}
}

// TestESIndicesPagesAreFullAfterFiltering is why authorization has to run
// before pagination rather than after.
//
// Filtering a page that was already cut left pages of unpredictable size: a
// user permitted to see three indices out of two hundred received twenty rows
// with three of them filled, then nineteen empty pages.
func TestESIndicesPagesAreFullAfterFiltering(t *testing.T) {
	visible := allowNamed{"logs-000": true, "logs-050": true, "logs-100": true, "logs-150": true, "logs-199": true}
	e, h, dsID := setupESIndexHandler(t, 200, visible)

	items, total := callESIndices(t, e, h, dsID, 1, 20)

	if total != 5 {
		t.Errorf("total = %v, want 5 — the count must reflect what this caller may see", total)
	}
	if len(items) != 5 {
		t.Errorf("page holds %d indices, want all 5 visible ones", len(items))
	}
}

// TestESIndicesSecondPageIsReachable is the user-visible consequence.
func TestESIndicesSecondPageIsReachable(t *testing.T) {
	e, h, dsID := setupESIndexHandler(t, 45, allowAll{})

	first, total := callESIndices(t, e, h, dsID, 1, 20)
	second, _ := callESIndices(t, e, h, dsID, 2, 20)
	third, _ := callESIndices(t, e, h, dsID, 3, 20)

	if total != 45 {
		t.Fatalf("total = %v, want 45", total)
	}
	if len(first) != 20 || len(second) != 20 || len(third) != 5 {
		t.Errorf("page sizes = %d/%d/%d, want 20/20/5", len(first), len(second), len(third))
	}
	if first[0]["name"] == second[0]["name"] {
		t.Error("page 2 repeats page 1")
	}
}

// allowAll permits every object, standing in for a caller with full access.
type allowAll struct{}

func (allowAll) CanViewObject(context.Context, int64, string, string, string) (bool, error) {
	return true, nil
}
