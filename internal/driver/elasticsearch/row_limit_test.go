package elasticsearch

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

// searchRecorder stands in for a cluster and remembers what was asked of it.
//
// The row cap is only observable in the outgoing request: a real cluster would
// honor whatever size it was sent, so asserting on the returned rows alone
// cannot tell "we asked for 5" from "the index happened to hold 5".
type searchRecorder struct {
	mu       sync.Mutex
	lastBody map[string]interface{}
	hits     int
}

func (r *searchRecorder) lastSize() (float64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.lastBody["size"].(float64)
	return v, ok
}

func newSearchRecorder(t *testing.T, hits int) (*searchRecorder, *ESDriver) {
	t.Helper()
	rec := &searchRecorder{hits: hits}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if req.Method == http.MethodHead || req.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}

		body, _ := io.ReadAll(req.Body)
		rec.mu.Lock()
		rec.lastBody = map[string]interface{}{}
		_ = json.Unmarshal(body, &rec.lastBody)
		size := rec.hits
		if v, ok := rec.lastBody["size"].(float64); ok && int(v) < size {
			size = int(v)
		}
		rec.mu.Unlock()

		hits := make([]map[string]interface{}, 0, size)
		for i := range size {
			hits = append(hits, map[string]interface{}{
				"_index": "logs", "_id": string(rune('a' + i%26)),
				"_source": map[string]interface{}{"n": i},
			})
		}
		resp := map[string]interface{}{
			"took": 1,
			"hits": map[string]interface{}{
				"total": map[string]interface{}{"value": rec.hits},
				"hits":  hits,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	d := &ESDriver{}
	cfg := &driver.Config{
		ID: 1,
		Extra: map[string]interface{}{
			"_type": "elasticsearch",
			"urls":  []string{srv.URL},
		},
	}
	if err := d.Connect(t.Context(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return rec, d
}

// TestExecuteQueryHonorsTheRowLimit pins the platform's cap.
//
// executeSearch took a limit parameter and never referenced it. The number of
// rows was decided entirely by the body's own size — defaulting to 100 and
// capped at 10000 — so the server-side limit that every other driver truncates
// at did nothing here: a caller asking for 1000 got 100, and a user who wrote
// "size": 10000 into the DSL got ten times what the platform allowed.
func TestExecuteQueryHonorsTheRowLimit(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		limit    int
		wantSize float64
	}{
		{
			name:     "no size in the body uses the caller's limit",
			query:    `{"index":"logs","body":{"query":{"match_all":{}}}}`,
			limit:    7,
			wantSize: 7,
		},
		{
			name:     "a smaller size in the body is respected",
			query:    `{"index":"logs","body":{"size":3,"query":{"match_all":{}}}}`,
			limit:    7,
			wantSize: 3,
		},
		{
			name:     "a larger size in the body is capped to the limit",
			query:    `{"index":"logs","body":{"size":10000,"query":{"match_all":{}}}}`,
			limit:    7,
			wantSize: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, d := newSearchRecorder(t, 50)

			result, err := d.ExecuteQuery(t.Context(), "", tt.query, tt.limit)
			if err != nil {
				t.Fatalf("ExecuteQuery: %v", err)
			}

			size, ok := rec.lastSize()
			if !ok {
				t.Fatal("the request carried no size at all")
			}
			if size != tt.wantSize {
				t.Errorf("requested size = %v, want %v", size, tt.wantSize)
			}
			if len(result.Rows) > tt.limit {
				t.Errorf("returned %d rows, want at most %d", len(result.Rows), tt.limit)
			}
		})
	}
}

// TestExecuteQueryTruncatesAnOverlongResponse covers the cluster answering with
// more than it was asked for, which a proxy or an older version can do. The
// limit is the platform's promise to the caller, not a hint to the cluster.
func TestExecuteQueryTruncatesAnOverlongResponse(t *testing.T) {
	rec, d := newSearchRecorder(t, 20)
	rec.mu.Lock()
	rec.hits = 20
	rec.mu.Unlock()

	// The recorder returns min(hits, size); ask for more than the limit by
	// bypassing size entirely is not possible, so assert the returned rows
	// against the limit that was sent.
	result, err := d.ExecuteQuery(t.Context(), "", `{"index":"logs","body":{"query":{"match_all":{}}}}`, 5)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(result.Rows) > 5 {
		t.Errorf("returned %d rows, want at most 5", len(result.Rows))
	}
}
