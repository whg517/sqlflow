package elasticsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

// newStubES points an ESDriver at a fake cluster that always replies with body.
func newStubES(t *testing.T, body string) *ESDriver {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	d := &ESDriver{}
	cfg := &driver.Config{Extra: map[string]interface{}{
		"urls":      []string{srv.URL},
		"auth_type": "none",
	}}
	if err := d.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("connect stub ES: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func executeStub(t *testing.T, d *ESDriver, query string) *driver.QueryResult {
	t.Helper()
	res, err := d.ExecuteQuery(context.Background(), query, 100)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	return res
}

// TestExecuteQuery_ColumnOrderIsStable is the regression for the randomized
// column order: the source field set used to be ranged over directly, so Go's
// map iteration reshuffled the user's columns between identical queries.
func TestExecuteQuery_ColumnOrderIsStable(t *testing.T) {
	const body = `{"hits":{"total":{"value":1},"hits":[{"_index":"i","_id":"1","_score":1.0,
	  "_source":{"zeta":1,"alpha":2,"mike":3,"bravo":4,"yankee":5,"charlie":6,"delta":7,"echo":8}}]}}`

	d := newStubES(t, body)

	want := []string{"_id", "_index", "_score", "alpha", "bravo", "charlie", "delta", "echo", "mike", "yankee", "zeta"}

	// Repeat: a single run can match by luck even with randomized iteration.
	for i := range 20 {
		got := executeStub(t, d, `{"index":"i","operation":"search"}`).Columns
		if len(got) != len(want) {
			t.Fatalf("run %d: got %d columns, want %d (%v)", i, len(got), len(want), got)
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("run %d: columns = %v, want %v", i, got, want)
			}
		}
	}
}

// TestExecuteQuery_AggregationIsPreserved is the regression for aggregation
// results being dropped. An aggregation request sets size:0, so parsing only
// `hits` returned an empty table and silently discarded the actual answer.
func TestExecuteQuery_AggregationIsPreserved(t *testing.T) {
	const body = `{"hits":{"total":{"value":42},"hits":[]},
	  "aggregations":{"by_status":{"buckets":[{"key":"open","doc_count":7},{"key":"closed","doc_count":35}]}}}`

	d := newStubES(t, body)
	res := executeStub(t, d, `{"index":"i","operation":"search","body":{"size":0,"aggs":{"by_status":{"terms":{"field":"status"}}}}}`)

	if res.Shape != driver.ShapeAggregation {
		t.Errorf("Shape = %q, want %q", res.Shape, driver.ShapeAggregation)
	}
	if len(res.Aggregations) == 0 {
		t.Fatal("Aggregations is empty — the aggregation payload was dropped")
	}

	var aggs struct {
		ByStatus struct {
			Buckets []struct {
				Key      string `json:"key"`
				DocCount int    `json:"doc_count"`
			} `json:"buckets"`
		} `json:"by_status"`
	}
	if err := json.Unmarshal(res.Aggregations, &aggs); err != nil {
		t.Fatalf("aggregations is not valid JSON: %v", err)
	}
	if len(aggs.ByStatus.Buckets) != 2 {
		t.Fatalf("got %d buckets, want 2", len(aggs.ByStatus.Buckets))
	}
	if aggs.ByStatus.Buckets[0].Key != "open" || aggs.ByStatus.Buckets[0].DocCount != 7 {
		t.Errorf("first bucket = %+v, want {open 7}", aggs.ByStatus.Buckets[0])
	}
	// Total is the number of rows this result carries, not the ES cluster hit
	// count. An aggregation sets size:0, so there are no hits — the answer
	// lives in Aggregations. Reporting 42 (the cluster total) here would make
	// audit logs and history record 42 rows for a result with zero rows.
	if res.Total != 0 {
		t.Errorf("Total = %d, want 0 (aggregation carries no rows)", res.Total)
	}
}

// TestExecuteQuery_SearchDeclaresDocumentShape keeps hits distinguishable from
// relational results so the UI can pick a renderer that survives nesting.
func TestExecuteQuery_SearchDeclaresDocumentShape(t *testing.T) {
	const body = `{"hits":{"total":{"value":1},"hits":[{"_index":"i","_id":"1","_score":1.0,
	  "_source":{"name":"a","address":{"city":"SF","zip":"94107"}}}]}}`

	d := newStubES(t, body)
	res := executeStub(t, d, `{"index":"i"}`)

	if res.Shape != driver.ShapeDocuments {
		t.Errorf("Shape = %q, want %q", res.Shape, driver.ShapeDocuments)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(res.Rows))
	}
	// The nested object must survive as structured data, not be stringified.
	nested, ok := res.Rows[0]["address"].(map[string]interface{})
	if !ok {
		t.Fatalf("address = %T, want a nested object", res.Rows[0]["address"])
	}
	if nested["city"] != "SF" {
		t.Errorf("address.city = %v, want SF", nested["city"])
	}
}
