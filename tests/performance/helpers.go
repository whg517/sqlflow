package performance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const (
	baseURL        = "http://localhost:8080"
	defaultTimeout = 30 * time.Second
)

// TargetURL returns the API base URL from the TEST_TARGET env or default.
func TargetURL() string {
	if u := os.Getenv("TEST_TARGET"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return baseURL
}



// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// PerfResult captures timing & size metrics for a single request.
type PerfResult struct {
	StatusCode   int
	Duration     time.Duration
	ResponseSize int64
	Error        error
}

// BatchResult aggregates results for a group of concurrent requests.
type BatchResult struct {
	Label       string
	Concurrency int
	Total       int
	Success     int
	Failed      int
	TotalTime   time.Duration
	AvgTime     time.Duration
	MinTime     time.Duration
	MaxTime     time.Duration
	P50         time.Duration
	P90         time.Duration
	P99         time.Duration
	TotalBytes  int64
	Errors      []string
}

// ---------------------------------------------------------------------------
// HTTP Helpers
// ---------------------------------------------------------------------------

var httpClient = &http.Client{Timeout: defaultTimeout}

// doRequest executes an HTTP request and returns detailed metrics.
func doRequest(method, url string, body []byte, headers map[string]string) PerfResult {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return PerfResult{Error: fmt.Errorf("create request: %w", err)}
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := httpClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return PerfResult{Duration: elapsed, Error: fmt.Errorf("http do: %w", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return PerfResult{
		StatusCode:   resp.StatusCode,
		Duration:     elapsed,
		ResponseSize: int64(len(respBody)),
	}
}

// concurrentRun launches n workers that each execute fn total/iterations times.
// Results are collected and aggregated into a BatchResult.
func concurrentRun(label string, concurrency int, total int, fn func() PerfResult) BatchResult {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		results  = make([]PerfResult, 0, total)
		start    = time.Now()
		perWorker = total / concurrency
		remainder = total % concurrency
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		iterations := perWorker
		if i < remainder {
			iterations++
		}
		go func(iter int) {
			defer wg.Done()
			for j := 0; j < iter; j++ {
				r := fn()
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}
		}(iterations)
	}
	wg.Wait()
	totalTime := time.Since(start)

	// Compute aggregated stats
	br := BatchResult{
		Label:       label,
		Concurrency: concurrency,
		Total:       total,
		TotalTime:   totalTime,
	}

	durations := make([]time.Duration, 0, len(results))
	for _, r := range results {
		br.TotalBytes += r.ResponseSize
		if r.Error != nil || r.StatusCode >= 400 {
			br.Failed++
			if r.Error != nil {
				br.Errors = append(br.Errors, r.Error.Error())
			} else {
				br.Errors = append(br.Errors, fmt.Sprintf("HTTP %d", r.StatusCode))
			}
		} else {
			br.Success++
			durations = append(durations, r.Duration)
		}
	}

	if len(durations) > 0 {
		br.MinTime = durations[0]
		br.MaxTime = durations[0]
		sum := time.Duration(0)
		for _, d := range durations {
			if d < br.MinTime {
				br.MinTime = d
			}
			if d > br.MaxTime {
				br.MaxTime = d
			}
			sum += d
		}
		br.AvgTime = sum / time.Duration(len(durations))

		// Percentiles require sorting
		sorted := make([]time.Duration, len(durations))
		copy(sorted, durations)
		sliceSort(sorted)
		br.P50 = sorted[percentileIndex(len(sorted), 50)]
		br.P90 = sorted[percentileIndex(len(sorted), 90)]
		br.P99 = sorted[percentileIndex(len(sorted), 99)]
	}

	return br
}

func percentileIndex(n, p int) int {
	idx := (p * n / 100) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return idx
}

func sliceSort(d []time.Duration) {
	// Simple insertion sort — fine for benchmark-sized slices (<10k).
	for i := 1; i < len(d); i++ {
		key := d[i]
		j := i - 1
		for j >= 0 && d[j] > key {
			d[j+1] = d[j]
			j--
		}
		d[j+1] = key
	}
}

// ---------------------------------------------------------------------------
// Login / Auth Helpers
// ---------------------------------------------------------------------------

// LoginResponse represents the API login response.
type LoginResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

// login performs a login and returns the JWT token string (empty on failure).
func login(base string, username, password string) string {
	payload, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	r := doRequest("POST", base+"/api/auth/login", payload, nil)
	if r.StatusCode != http.StatusOK {
		return ""
	}
	var lr LoginResponse
	if err := json.Unmarshal([]byte(strconv.FormatInt(r.ResponseSize, 10)), &lr); err != nil {
		// Response may not be parseable in test mode — that's ok
		return ""
	}
	return lr.Token
}

// ---------------------------------------------------------------------------
// Embedded Test Server (for offline / CI use)
// ---------------------------------------------------------------------------

// newTestServer creates a minimal Echo-like HTTP test server with stub endpoints.
// This allows benchmarks to run without a real SQLFlow backend.
func newTestServer() *httptest.Server {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Login — simulates bcrypt + JWT generation delay (~2ms)
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Millisecond) // simulate bcrypt cost
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test","refresh_token":"ref-test"}`))
	})

	// Dashboard stats
	mux.HandleFunc("/api/dashboard/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total_users":42,"total_queries":1580,"today_queries":234,"active_datasources":5}`))
	})

	// Query execute — variable delay to simulate DB round-trip
	mux.HandleFunc("/api/query/execute", func(w http.ResponseWriter, r *http.Request) {
		// Simulate DB query processing
		delay := 5 * time.Millisecond
		if r.Body != nil {
			var body map[string]interface{}
			if json.NewDecoder(r.Body).Decode(&body) == nil {
				// Simulate larger result sets taking longer
				if limit, ok := body["limit"].(float64); ok && limit > 1000 {
					delay = 20 * time.Millisecond
				}
			}
		}
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		// Simulate row results
		rows := generateRows(100)
		w.Write([]byte(rows))
	})

	// Query history list
	mux.HandleFunc("/api/query/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"total":0,"page":1,"page_size":20}`))
	})

	// Ticket list
	mux.HandleFunc("/api/tickets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"total":0,"page":1,"page_size":20}`))
	})

	// Audit logs
	mux.HandleFunc("/api/audit-logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"total":0,"page":1,"page_size":20}`))
	})

	// Auth me
	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1,"username":"admin","role":"admin"}`))
	})

	return httptest.NewServer(mux)
}

// generateRows creates a JSON array of row objects of the given count.
func generateRows(n int) string {
	var sb strings.Builder
	sb.WriteString(`{"columns":["id","name","email","created_at"],"rows":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`[%d,"user_%d","user_%d@example.com","2026-01-01T00:00:00Z"]`, i+1, i+1, i+1))
	}
	sb.WriteString(`],"total":` + strconv.Itoa(n) + `}`)
	return sb.String()
}

// ---------------------------------------------------------------------------
// Report Generation
// ---------------------------------------------------------------------------

// generateReport produces a Markdown performance report from batch results.
func generateReport(results []BatchResult, serverInfo string) string {
	var sb strings.Builder

	sb.WriteString("# SQLFlow Performance Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("**Server:** %s\n\n", serverInfo))

	// Summary table
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Test | Concurrency | Total Reqs | Success | Failed | Avg Latency | P50 | P90 | P99 | Throughput |\n")
	sb.WriteString("|------|-------------|------------|---------|--------|-------------|-----|-----|-----|------------|\n")

	for _, r := range results {
		throughput := float64(r.Total) / r.TotalTime.Seconds()
		_ = throughput
		sb.WriteString(fmt.Sprintf(
			"| %s | %d | %d | %d | %d | %s | %s | %s | %s | %.1f req/s |\n",
			r.Label,
			r.Concurrency,
			r.Total,
			r.Success,
			r.Failed,
			r.AvgTime.Round(time.Microsecond),
			r.P50.Round(time.Microsecond),
			r.P90.Round(time.Microsecond),
			r.P99.Round(time.Microsecond),
			throughput,
		))
	}

	// Per-test details
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("\n## %s\n\n", r.Label))
		sb.WriteString(fmt.Sprintf("- **Concurrency:** %d\n", r.Concurrency))
		sb.WriteString(fmt.Sprintf("- **Total Requests:** %d\n", r.Total))
		sb.WriteString(fmt.Sprintf("- **Success Rate:** %.2f%%\n", float64(r.Success)/float64(r.Total)*100))
		sb.WriteString(fmt.Sprintf("- **Total Duration:** %s\n", r.TotalTime.Round(time.Millisecond)))
		sb.WriteString(fmt.Sprintf("- **Avg Response Time:** %s\n", r.AvgTime.Round(time.Microsecond)))
		sb.WriteString(fmt.Sprintf("- **Min Response Time:** %s\n", r.MinTime.Round(time.Microsecond)))
		sb.WriteString(fmt.Sprintf("- **Max Response Time:** %s\n", r.MaxTime.Round(time.Microsecond)))
		sb.WriteString(fmt.Sprintf("- **P50:** %s\n", r.P50.Round(time.Microsecond)))
		sb.WriteString(fmt.Sprintf("- **P90:** %s\n", r.P90.Round(time.Microsecond)))
		sb.WriteString(fmt.Sprintf("- **P99:** %s\n", r.P99.Round(time.Microsecond)))
		sb.WriteString(fmt.Sprintf("- **Throughput:** %.1f req/s\n", float64(r.Total)/r.TotalTime.Seconds()))
		sb.WriteString(fmt.Sprintf("- **Total Response Data:** %s\n", formatBytes(r.TotalBytes)))

		if len(r.Errors) > 0 {
			sb.WriteString(fmt.Sprintf("\n**Errors (%d):**\n", len(r.Errors)))
			// Show up to 5 unique errors
			seen := make(map[string]bool)
			count := 0
			for _, e := range r.Errors {
				if !seen[e] {
					sb.WriteString(fmt.Sprintf("- %s\n", e))
					seen[e] = true
					count++
					if count >= 5 {
						break
					}
				}
			}
			if len(seen) < len(r.Errors) {
				sb.WriteString(fmt.Sprintf("- ... and %d more unique errors\n", len(r.Errors)-len(seen)))
			}
		}
		sb.WriteString("\n")
	}

	// Analysis
	sb.WriteString("## Analysis & Recommendations\n\n")
	sb.WriteString("### Key Observations\n\n")
	for _, r := range results {
		successRate := float64(r.Success) / float64(r.Total) * 100
		if successRate < 99 {
			sb.WriteString(fmt.Sprintf("- ⚠️ **%s**: Success rate %.1f%% is below 99%% threshold\n", r.Label, successRate))
		}
		if r.P99 > 500*time.Millisecond {
			sb.WriteString(fmt.Sprintf("- ⚠️ **%s**: P99 latency (%s) exceeds 500ms threshold\n", r.Label, r.P99.Round(time.Millisecond)))
		}
		if r.P90 > 200*time.Millisecond {
			sb.WriteString(fmt.Sprintf("- ⚠️ **%s**: P90 latency (%s) exceeds 200ms threshold\n", r.Label, r.P90.Round(time.Millisecond)))
		}
	}
	sb.WriteString("\n")

	return sb.String()
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
