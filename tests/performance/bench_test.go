package performance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Shared State
// ---------------------------------------------------------------------------

var (
	reportMu      sync.Mutex
	reportResults []BatchResult
	serverAddr    string
)

// useRealServer controls whether to connect to a real SQLFlow instance.
// Set TEST_REAL_SERVER=1 to enable.
var useRealServer = os.Getenv("TEST_REAL_SERVER") == "1"

// TestMain sets up the shared embedded test server for all tests.
func TestMain(m *testing.M) {
	if useRealServer {
		realURL := TargetURL()
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(realURL + "/api/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			fmt.Fprintf(os.Stderr, "real server not reachable at %s: %v\n", realURL, err)
			os.Exit(1)
		}
		resp.Body.Close()
		serverAddr = realURL
		fmt.Printf("[perf] Using real server at %s\n", realURL)
	} else {
		ts := newTestServer()
		serverAddr = ts.URL
		fmt.Printf("[perf] Starting embedded test server at %s\n", ts.URL)
		defer ts.Close()
	}
	os.Exit(m.Run())
}

// getServerBase returns the shared server base URL.
func getServerBase(t testing.TB) string {
	if serverAddr == "" {
		t.Fatal("server not initialized — TestMain did not run")
	}
	return serverAddr
}

// getAuthToken logs in to the server and returns a JWT token.
// For the embedded test server, returns a dummy token since auth is not enforced.
func getAuthToken(t testing.TB) string {
	base := getServerBase(t)
	payload := []byte(`{"username":"admin","password":"admin123"}`)
	body := bytes.NewBuffer(payload)
	req, _ := http.NewRequest("POST", base+"/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()

	var lr LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		// Embedded server may not return valid JSON — use dummy token
		return "test-token"
	}
	if lr.Token != "" {
		return lr.Token
	}
	return "test-token"
}

// collect adds a BatchResult to the report.
func collect(br BatchResult) {
	reportMu.Lock()
	reportResults = append(reportResults, br)
	reportMu.Unlock()
}

// =============================================================================
// Benchmarks: Sequential API Calls
// =============================================================================

func BenchmarkHealthCheck(b *testing.B) {
	base := getServerBase(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := doRequest("GET", base+"/api/health", nil, nil)
		if r.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status: %d", r.StatusCode)
		}
	}
}

func BenchmarkLogin(b *testing.B) {
	base := getServerBase(b)
	payload := []byte(`{"username":"admin","password":"admin123"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := doRequest("POST", base+"/api/auth/login", payload, nil)
		if r.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status: %d", r.StatusCode)
		}
	}
}

func BenchmarkDashboardStats(b *testing.B) {
	base := getServerBase(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := doRequest("GET", base+"/api/dashboard/stats", nil, nil)
		if r.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status: %d", r.StatusCode)
		}
	}
}

func BenchmarkQueryExecute(b *testing.B) {
	base := getServerBase(b)
	payload := []byte(`{"datasource_id":1,"database":"testdb","sql":"SELECT id, name FROM users LIMIT 100","db_type":"mysql"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := doRequest("POST", base+"/api/query/execute", payload, nil)
		if r.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status: %d", r.StatusCode)
		}
	}
}

func BenchmarkQueryHistory(b *testing.B) {
	base := getServerBase(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := doRequest("GET", base+"/api/query/history?page=1&page_size=20", nil, nil)
		if r.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status: %d", r.StatusCode)
		}
	}
}

func BenchmarkTicketList(b *testing.B) {
	base := getServerBase(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := doRequest("GET", base+"/api/tickets?page=1&page_size=20", nil, nil)
		if r.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status: %d", r.StatusCode)
		}
	}
}

// =============================================================================
// Test: Concurrent Login (50/100/200)
// =============================================================================

func TestConcurrentLogin(t *testing.T) {
	base := getServerBase(t)
	payload := []byte(`{"username":"admin","password":"admin123"}`)

	for _, c := range []int{50, 100, 200} {
		t.Run(fmt.Sprintf("concurrency_%d", c), func(t *testing.T) {
			br := concurrentRun(
				fmt.Sprintf("Login (concurrency=%d)", c),
				c, 200,
				func() PerfResult {
					return doRequest("POST", base+"/api/auth/login", payload, nil)
				},
			)
			collect(br)
			t.Logf("Login concurrency=%d: %d/%d success, avg=%s, P99=%s",
				c, br.Success, br.Total, br.AvgTime, br.P99)
			if br.Success == 0 {
				t.Fatalf("all requests failed")
			}
		})
	}
}

// =============================================================================
// Test: Concurrent Query Execute (50/100/200)
// =============================================================================

func TestConcurrentQueryExecute(t *testing.T) {
	base := getServerBase(t)
	token := getAuthToken(t)
	headers := map[string]string{"Authorization": "Bearer " + token}
	payload := []byte(`{"datasource_id":1,"database":"testdb","sql":"SELECT id, name FROM users LIMIT 100","db_type":"mysql"}`)

	for _, c := range []int{50, 100, 200} {
		t.Run(fmt.Sprintf("concurrency_%d", c), func(t *testing.T) {
			br := concurrentRun(
				fmt.Sprintf("Query Execute (concurrency=%d)", c),
				c, 200,
				func() PerfResult {
					return doRequest("POST", base+"/api/query/execute", payload, headers)
				},
			)
			collect(br)
			t.Logf("Query concurrency=%d: %d/%d success, avg=%s, P99=%s",
				c, br.Success, br.Total, br.AvgTime, br.P99)
			if br.Success == 0 {
				t.Fatalf("all requests failed")
			}
		})
	}
}

// =============================================================================
// Test: Concurrent Dashboard Stats (50/100/200)
// =============================================================================

func TestConcurrentDashboardStats(t *testing.T) {
	base := getServerBase(t)
	token := getAuthToken(t)
	headers := map[string]string{"Authorization": "Bearer " + token}

	for _, c := range []int{50, 100, 200} {
		t.Run(fmt.Sprintf("concurrency_%d", c), func(t *testing.T) {
			br := concurrentRun(
				fmt.Sprintf("Dashboard Stats (concurrency=%d)", c),
				c, 200,
				func() PerfResult {
					return doRequest("GET", base+"/api/dashboard/stats", nil, headers)
				},
			)
			collect(br)
			t.Logf("Dashboard concurrency=%d: %d/%d success, avg=%s, P99=%s",
				c, br.Success, br.Total, br.AvgTime, br.P99)
			if br.Success == 0 {
				t.Fatalf("all requests failed")
			}
		})
	}
}

// =============================================================================
// Test: Large Dataset Queries (1000 / 10000 rows)
// =============================================================================

func TestLargeResultQuery(t *testing.T) {
	base := getServerBase(t)
	token := getAuthToken(t)
	headers := map[string]string{"Authorization": "Bearer " + token}

	for _, n := range []int{1000, 10000} {
		t.Run(fmt.Sprintf("rows_%d", n), func(t *testing.T) {
			payload := []byte(fmt.Sprintf(
				`{"datasource_id":1,"database":"testdb","sql":"SELECT id, name, email FROM users LIMIT %d","db_type":"mysql","limit":%d}`,
				n, n,
			))
			br := concurrentRun(
				fmt.Sprintf("Large Result (%d rows)", n),
				10, 50,
				func() PerfResult {
					return doRequest("POST", base+"/api/query/execute", payload, headers)
				},
			)
			collect(br)
			t.Logf("Large result %d rows: %d/%d success, avg=%s, P99=%s, total_data=%s",
				n, br.Success, br.Total, br.AvgTime, br.P99, formatBytes(br.TotalBytes))
			if br.Success == 0 {
				t.Fatalf("all requests failed for %d rows", n)
			}
		})
	}
}

// =============================================================================
// Test: Login Stress Test (50/100/200 concurrency, higher volume)
// =============================================================================

func TestLoginStress(t *testing.T) {
	base := getServerBase(t)
	payload := []byte(`{"username":"admin","password":"admin123"}`)

	levels := []struct {
		conc  int
		total int
	}{
		{50, 500},
		{100, 1000},
		{200, 2000},
	}

	for _, lv := range levels {
		t.Run(fmt.Sprintf("stress_%d_conc_%d_total", lv.conc, lv.total), func(t *testing.T) {
			br := concurrentRun(
				fmt.Sprintf("Login Stress (concurrency=%d, total=%d)", lv.conc, lv.total),
				lv.conc, lv.total,
				func() PerfResult {
					return doRequest("POST", base+"/api/auth/login", payload, nil)
				},
			)
			collect(br)

			rate := float64(br.Success) / float64(br.Total) * 100
			t.Logf("Login stress c=%d n=%d: %.1f%% success, avg=%s, P50=%s, P99=%s, throughput=%.1f req/s",
				lv.conc, lv.total, rate, br.AvgTime, br.P50, br.P99,
				float64(br.Total)/br.TotalTime.Seconds())

			if rate < 95 {
				t.Errorf("success rate %.1f%% is below 95%% threshold", rate)
			}
		})
	}
}

// =============================================================================
// Test: Mixed Workload (6 endpoints × 50 goroutines)
// =============================================================================

func TestMixedWorkload(t *testing.T) {
	base := getServerBase(t)
	token := getAuthToken(t)
	authH := map[string]string{"Authorization": "Bearer " + token}
	loginPayload := []byte(`{"username":"admin","password":"admin123"}`)
	queryPayload := []byte(`{"datasource_id":1,"database":"testdb","sql":"SELECT id FROM users LIMIT 100","db_type":"mysql"}`)

	endpoints := []struct {
		name    string
		method  string
		path    string
		body    []byte
		headers map[string]string
	}{
		{"Login", "POST", "/api/auth/login", loginPayload, nil},
		{"Dashboard", "GET", "/api/dashboard/stats", nil, authH},
		{"QueryHistory", "GET", "/api/query/history?page=1&page_size=20", nil, authH},
		{"QueryExecute", "POST", "/api/query/execute", queryPayload, authH},
		{"TicketList", "GET", "/api/tickets?page=1&page_size=20", nil, authH},
		{"AuditLogs", "GET", "/api/audit-logs?page=1&page_size=20", nil, authH},
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		total, ok, fail int
		start = time.Now()
	)

	for i := 0; i < 50; i++ {
		for _, ep := range endpoints {
			wg.Add(1)
			go func(method, path string, body []byte, headers map[string]string) {
				defer wg.Done()
				r := doRequest(method, base+path, body, headers)
				mu.Lock()
				total++
				if r.Error != nil || r.StatusCode >= 400 {
					fail++
				} else {
					ok++
				}
				mu.Unlock()
			}(ep.method, ep.path, ep.body, ep.headers)
		}
	}
	wg.Wait()
	elapsed := time.Since(start)

	br := BatchResult{
		Label:       "Mixed Workload (6 endpoints × 50 goroutines)",
		Concurrency: 50,
		Total:       total,
		Success:     ok,
		Failed:      fail,
		TotalTime:   elapsed,
		AvgTime:     elapsed / time.Duration(total),
	}
	collect(br)

	rate := float64(ok) / float64(total) * 100
	t.Logf("Mixed workload: %d/%d (%.1f%%) success in %s, avg=%s, throughput=%.1f req/s",
		ok, total, rate, elapsed, br.AvgTime, float64(total)/elapsed.Seconds())
}

// =============================================================================
// Report Generation
// =============================================================================

func TestGenerateReport(t *testing.T) {
	reportMu.Lock()
	if len(reportResults) == 0 {
		reportMu.Unlock()
		t.Skip("No results collected — run concurrency/stress tests first")
	}
	results := make([]BatchResult, len(reportResults))
	copy(results, reportResults)
	reportMu.Unlock()

	info := serverAddr
	if info == "" {
		info = "embedded-test-server"
	}

	report := generateReport(results, info)

	reportPath := "report.md"
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		t.Fatalf("failed to write report: %v", err)
	}
	t.Logf("Report written to %s (%d bytes)", reportPath, len(report))

	// Print report to stdout for CI visibility
	fmt.Println("\n" + strings.Repeat("=", 80))
	scanner := bufio.NewScanner(strings.NewReader(report))
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	fmt.Println(strings.Repeat("=", 80))
}

// Ensure httptest is used (suppress unused import in non-real-server mode).
var _ = (*httptest.Server)(nil)
