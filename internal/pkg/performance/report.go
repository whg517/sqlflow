package performance

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

// Baseline represents performance thresholds that the system should meet.
type Baseline struct {
	APILoginP95        time.Duration `json:"api_login_p95"`
	APIDashboardP95    time.Duration `json:"api_dashboard_p95"`
	APITicketListP95   time.Duration `json:"api_ticket_list_p95"`
	APITicketCreateP95 time.Duration `json:"api_ticket_create_p95"`
	DBInsertP99        time.Duration `json:"db_insert_p99"`
	DBSelectP99        time.Duration `json:"db_select_p99"`
	DBIndexSelectP99   time.Duration `json:"db_index_select_p99"`
	MinQPS             float64       `json:"min_qps_login"`
}

// DefaultBaseline returns recommended performance baselines for sqlflow.
func DefaultBaseline() *Baseline {
	return &Baseline{
		APILoginP95:        500 * time.Millisecond,
		APIDashboardP95:    200 * time.Millisecond,
		APITicketListP95:   300 * time.Millisecond,
		APITicketCreateP95: 500 * time.Millisecond,
		DBInsertP99:        50 * time.Millisecond,
		DBSelectP99:        20 * time.Millisecond,
		DBIndexSelectP99:   5 * time.Millisecond,
		MinQPS:             50,
	}
}

// BaselineResult captures whether each baseline threshold was met.
type BaselineResult struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Actual   time.Duration `json:"actual"`
	Expected time.Duration `json:"expected"`
}

// Report holds the complete performance test report.
type Report struct {
	Timestamp    time.Time        `json:"timestamp"`
	Duration     time.Duration    `json:"duration"`
	HTTPResults  []*Result        `json:"http_results"`
	DBResults    []DBQueryResult  `json:"db_results"`
	Baseline     *Baseline        `json:"baseline"`
	BaselinePass []BaselineResult `json:"baseline_pass"`
	Summary      string           `json:"summary"`
}

// NewReport creates an empty report.
func NewReport(baseline *Baseline) *Report {
	if baseline == nil {
		baseline = DefaultBaseline()
	}
	return &Report{
		Timestamp: time.Now(),
		Baseline:  baseline,
	}
}

// JSON returns the report as formatted JSON.
func (r *Report) JSON() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

// CheckBaselines evaluates HTTP results against baseline thresholds.
func (r *Report) CheckBaselines() {
	for _, hr := range r.HTTPResults {
		var name string
		var expected time.Duration

		switch {
		case strings.Contains(hr.Name, "Login"):
			name = "API Login P95"
			expected = r.Baseline.APILoginP95
		case strings.Contains(hr.Name, "Dashboard"):
			name = "API Dashboard P95"
			expected = r.Baseline.APIDashboardP95
		case strings.Contains(hr.Name, "Ticket List"):
			name = "API Ticket List P95"
			expected = r.Baseline.APITicketListP95
		case strings.Contains(hr.Name, "Ticket Create"):
			name = "API Ticket Create P95"
			expected = r.Baseline.APITicketCreateP95
		default:
			continue
		}

		br := BaselineResult{
			Name:     name,
			Passed:   hr.P95Latency <= expected,
			Actual:   hr.P95Latency,
			Expected: expected,
		}
		r.BaselinePass = append(r.BaselinePass, br)
	}
}

// GenerateSummary creates a human-readable summary of the report.
func (r *Report) GenerateSummary() string {
	var sb strings.Builder

	sb.WriteString("=" + strings.Repeat("=", 79) + "\n")
	sb.WriteString("  SQLFlow 性能测试报告\n")
	sb.WriteString("  " + r.Timestamp.Format("2006-01-02 15:04:05 MST") + "\n")
	sb.WriteString("=" + strings.Repeat("=", 79) + "\n\n")

	// HTTP Results
	sb.WriteString("【HTTP 接口并发测试】\n")
	sb.WriteString("-" + strings.Repeat("-", 79) + "\n")
	for _, hr := range r.HTTPResults {
		sb.WriteString(fmt.Sprintf("  %s\n", hr.Name))
		sb.WriteString(fmt.Sprintf("    并发数: %d  |  总请求数: %d  |  成功: %d  |  失败: %d\n",
			hr.Concurrency, hr.TotalRequests, hr.SuccessCount, hr.FailureCount))
		sb.WriteString(fmt.Sprintf("    吞吐量: %.1f req/s\n", hr.RequestsPerSec))
		sb.WriteString(fmt.Sprintf("    延迟  Avg: %v  P50: %v  P90: %v  P95: %v  P99: %v\n",
			hr.AvgLatency, hr.P50Latency, hr.P90Latency, hr.P95Latency, hr.P99Latency))
		sb.WriteString(fmt.Sprintf("    范围  Min: %v  Max: %v\n", hr.MinLatency, hr.MaxLatency))
		if len(hr.ErrorMessages) > 0 {
			sb.WriteString(fmt.Sprintf("    错误示例: %s\n", hr.ErrorMessages[0]))
		}
		sb.WriteString("\n")
	}

	// DB Results
	sb.WriteString("【数据库查询性能】\n")
	sb.WriteString("-" + strings.Repeat("-", 79) + "\n")
	for _, dr := range r.DBResults {
		sb.WriteString(fmt.Sprintf("  %s\n", dr.Name))
		sb.WriteString(fmt.Sprintf("    迭代次数: %d\n", dr.Iterations))
		sb.WriteString(fmt.Sprintf("    延迟  Avg: %v  P50: %v  P90: %v  P99: %v\n",
			dr.AvgDuration, dr.P50Duration, dr.P90Duration, dr.P99Duration))
		sb.WriteString(fmt.Sprintf("    范围  Min: %v  Max: %v\n", dr.MinDuration, dr.MaxDuration))
		if dr.RowsPerSecond > 0 {
			sb.WriteString(fmt.Sprintf("    吞吐量: %.0f rows/s\n", dr.RowsPerSecond))
		}
		sb.WriteString("\n")
	}

	// Baseline Check
	sb.WriteString("【基准线检查】\n")
	sb.WriteString("-" + strings.Repeat("-", 79) + "\n")
	allPassed := true
	for _, bp := range r.BaselinePass {
		status := "✅ PASS"
		if !bp.Passed {
			status = "❌ FAIL"
			allPassed = false
		}
		sb.WriteString(fmt.Sprintf("  %s  %s: 实际=%v  基准=%v\n", status, bp.Name, bp.Actual, bp.Expected))
	}
	sb.WriteString("\n")

	// Optimization suggestions
	sb.WriteString("【优化建议】\n")
	sb.WriteString("-" + strings.Repeat("-", 79) + "\n")
	suggestions := generateSuggestions(r)
	if len(suggestions) == 0 {
		sb.WriteString("  🎉 所有指标均在基准线内，无需额外优化！\n")
	} else {
		for i, s := range suggestions {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, s))
		}
	}
	sb.WriteString("\n")

	// Overall verdict
	sb.WriteString("=" + strings.Repeat("=", 79) + "\n")
	if allPassed {
		sb.WriteString("  总体评估: ✅ 通过 — 所有性能指标满足基准线要求\n")
	} else {
		sb.WriteString("  总体评估: ⚠️  部分指标未达基准线，请参考优化建议\n")
	}
	sb.WriteString("=" + strings.Repeat("=", 79) + "\n")

	r.Summary = sb.String()
	return sb.String()
}

// generateSuggestions analyzes results and returns optimization suggestions.
func generateSuggestions(r *Report) []string {
	var suggestions []string

	for _, hr := range r.HTTPResults {
		if hr.FailureCount > 0 {
			failRate := float64(hr.FailureCount) / float64(hr.TotalRequests) * 100
			if failRate > 5 {
				suggestions = append(suggestions,
					fmt.Sprintf("[%s] 错误率 %.1f%% 超过 5%% 阈值，建议检查服务稳定性、连接池配置和超时设置",
						hr.Name, failRate))
			}
		}
		if hr.P99Latency > 2*time.Second {
			suggestions = append(suggestions,
				fmt.Sprintf("[%s] P99 延迟 %v 超过 2s，建议排查慢查询、加缓存或优化算法",
					hr.Name, hr.P99Latency))
		}
		if hr.RequestsPerSec < 10 && hr.TotalRequests > 50 {
			suggestions = append(suggestions,
				fmt.Sprintf("[%s] 吞吐量仅 %.1f req/s，建议检查数据库锁竞争和连接池瓶颈",
					hr.Name, hr.RequestsPerSec))
		}
	}

	// SQLite-specific suggestions
	for _, dr := range r.DBResults {
		if strings.Contains(dr.Name, "Insert") && dr.P99Duration > 100*time.Millisecond {
			suggestions = append(suggestions,
				fmt.Sprintf("[DB] 写入 P99=%v 偏高。SQLite 单写者模型下建议：1) 减少 MaxOpenConns=1 的瓶颈 2) 使用批量 INSERT 3) 考虑 WAL 模式（已启用）",
					dr.P99Duration))
		}
		if strings.Contains(dr.Name, "Select") && dr.P99Duration > 50*time.Millisecond {
			suggestions = append(suggestions,
				fmt.Sprintf("[DB] 查询 P99=%v 偏高，建议：1) 检查索引覆盖率 2) 避免 SELECT * 3) 考虑查询结果缓存",
					dr.P99Duration))
		}
	}

	// Baseline-specific suggestions
	for _, bp := range r.BaselinePass {
		if !bp.Passed {
			ratio := float64(bp.Actual) / float64(bp.Expected)
			if ratio > 3 {
				suggestions = append(suggestions,
					fmt.Sprintf("[%s] 严重超标（%.1fx），建议作为高优先级优化项", bp.Name, ratio))
			}
		}
	}

	return suggestions
}

// RunFullBenchmark orchestrates the complete performance test suite.
// It starts a test server, runs HTTP benchmarks, DB benchmarks, checks baselines,
// and returns the final report.
func RunFullBenchmark(testServerURL string, totalReqs, concurrency int) *Report {
	cfg := DefaultBenchmarkConfig(testServerURL)
	cfg.TotalReqs = totalReqs
	cfg.Concurrency = concurrency
	cfg.InsecureTLS = true // test server may use self-signed cert

	report := NewReport(nil)
	start := time.Now()

	log.Println("🚀 开始性能测试...")
	log.Printf("  目标: %s", testServerURL)
	log.Printf("  并发数: %d, 总请求: %d\n", concurrency, totalReqs)

	// --- HTTP Benchmarks ---
	log.Println("--- HTTP 接口并发测试 ---")

	// 1. Health check (warmup)
	log.Println("  [Warmup] Health check...")
	healthResult := RunHTTPBenchmark("Health Check (Warmup)", RequestSpec{
		Method: "GET",
		URL:    cfg.BaseURL + "/api/health",
	}, &BenchmarkConfig{
		BaseURL:     cfg.BaseURL,
		Concurrency: 1,
		TotalReqs:   10,
		Timeout:     cfg.Timeout,
		InsecureTLS: cfg.InsecureTLS,
	})
	report.HTTPResults = append(report.HTTPResults, healthResult)
	log.Printf("  Health: avg=%v, qps=%.1f\n", healthResult.AvgLatency, healthResult.RequestsPerSec)

	// 2. Login (unauthenticated)
	loginBody := `{"username":"admin","password":"admin123"}`
	log.Println("  [测试] 登录接口 (POST /api/auth/login)...")
	loginResult := RunHTTPBenchmark("Login (POST /api/auth/login)", RequestSpec{
		Method:  "POST",
		URL:     cfg.BaseURL + "/api/auth/login",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    io.NopCloser(bytes.NewReader([]byte(loginBody))),
	}, cfg)
	report.HTTPResults = append(report.HTTPResults, loginResult)
	log.Printf("  Login: avg=%v, P95=%v, qps=%.1f\n", loginResult.AvgLatency, loginResult.P95Latency, loginResult.RequestsPerSec)

	// Extract JWT token from a successful login response for authenticated tests
	token := extractTokenFromLogin(testServerURL)

	if token != "" {
		authHeaders := map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + token,
		}

		// 3. Dashboard Stats (authenticated, lightweight)
		log.Println("  [测试] Dashboard 统计 (GET /api/dashboard/stats)...")
		dashResult := RunHTTPBenchmark("Dashboard Stats (GET /api/dashboard/stats)", RequestSpec{
			Method:  "GET",
			URL:     cfg.BaseURL + "/api/dashboard/stats",
			Headers: authHeaders,
		}, cfg)
		report.HTTPResults = append(report.HTTPResults, dashResult)
		log.Printf("  Dashboard: avg=%v, P95=%v, qps=%.1f\n", dashResult.AvgLatency, dashResult.P95Latency, dashResult.RequestsPerSec)

		// 4. Query History (authenticated)
		log.Println("  [测试] 查询历史列表 (GET /api/query/history)...")
		queryHistResult := RunHTTPBenchmark("Query History (GET /api/query/history)", RequestSpec{
			Method:  "GET",
			URL:     cfg.BaseURL + "/api/query/history?page=1&page_size=20",
			Headers: authHeaders,
		}, cfg)
		report.HTTPResults = append(report.HTTPResults, queryHistResult)
		log.Printf("  Query History: avg=%v, P95=%v, qps=%.1f\n", queryHistResult.AvgLatency, queryHistResult.P95Latency, queryHistResult.RequestsPerSec)

		// 5. Ticket List (authenticated)
		log.Println("  [测试] 工单列表 (GET /api/tickets)...")
		ticketListResult := RunHTTPBenchmark("Ticket List (GET /api/tickets)", RequestSpec{
			Method:  "GET",
			URL:     cfg.BaseURL + "/api/tickets?page=1&page_size=20",
			Headers: authHeaders,
		}, cfg)
		report.HTTPResults = append(report.HTTPResults, ticketListResult)
		log.Printf("  Ticket List: avg=%v, P95=%v, qps=%.1f\n", ticketListResult.AvgLatency, ticketListResult.P95Latency, ticketListResult.RequestsPerSec)

		// 6. Ticket Create (authenticated, write)
		ticketCreateBody := `{"datasource_id":1,"database":"testdb","sql":"SELECT 1","db_type":"mysql","change_reason":"性能测试","risk_level":"low"}`
		log.Println("  [测试] 创建工单 (POST /api/tickets)...")
		ticketCreateResult := RunHTTPBenchmark("Ticket Create (POST /api/tickets)", RequestSpec{
			Method:  "POST",
			URL:     cfg.BaseURL + "/api/tickets",
			Headers: authHeaders,
			Body:    io.NopCloser(bytes.NewReader([]byte(ticketCreateBody))),
		}, cfg)
		report.HTTPResults = append(report.HTTPResults, ticketCreateResult)
		log.Printf("  Ticket Create: avg=%v, P95=%v, qps=%.1f\n", ticketCreateResult.AvgLatency, ticketCreateResult.P95Latency, ticketCreateResult.RequestsPerSec)

		// 7. Auth Me (authenticated, simple lookup)
		log.Println("  [测试] 当前用户信息 (GET /api/auth/me)...")
		meResult := RunHTTPBenchmark("Auth Me (GET /api/auth/me)", RequestSpec{
			Method:  "GET",
			URL:     cfg.BaseURL + "/api/auth/me",
			Headers: authHeaders,
		}, cfg)
		report.HTTPResults = append(report.HTTPResults, meResult)
		log.Printf("  Auth Me: avg=%v, P95=%v, qps=%.1f\n", meResult.AvgLatency, meResult.P95Latency, meResult.RequestsPerSec)
	} else {
		log.Println("  ⚠️  无法获取 JWT Token，跳过认证接口测试")
	}

	// --- DB Benchmarks ---
	log.Println("\n--- 数据库查询性能 ---")
	runDBBenchmarks(report)

	// --- Baseline Check ---
	report.CheckBaselines()

	report.Duration = time.Since(start)
	report.GenerateSummary()

	log.Println()
	log.Println(report.Summary)

	return report
}

// RunDBBenchmarks performs database-level benchmarks using a temporary SQLite database.
func runDBBenchmarks(report *Report) {
	tmpDB := setupTestDB()
	defer tmpDB.Close()

	ctx := context.Background()
	seedDB(ctx, tmpDB)

	// 1. Single INSERT benchmark
	insertResult := benchmarkDBOperation("DB Insert (single row)", 200, func(ctx context.Context, db *sql.DB) error {
		_, err := db.ExecContext(ctx,
			`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, result_rows, affected_rows, execution_time_ms, error_message, desensitized_fields, ip_address) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			1, "SELECT", 1, "testdb", "SELECT * FROM users", "查询用户", 10, 0, 5, "", "", "127.0.0.1",
		)
		return err
	})
	report.DBResults = append(report.DBResults, insertResult)
	log.Printf("  Insert: avg=%v, P99=%v\n", insertResult.AvgDuration, insertResult.P99Duration)

	// 2. Simple SELECT benchmark
	selectResult := benchmarkDBOperation("DB Select (COUNT)", 500, func(ctx context.Context, db *sql.DB) error {
		var count int
		return db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&count)
	})
	report.DBResults = append(report.DBResults, selectResult)
	log.Printf("  Select COUNT: avg=%v, P99=%v\n", selectResult.AvgDuration, selectResult.P99Duration)

	// 3. Index SELECT benchmark
	indexSelectResult := benchmarkDBOperation("DB Select (indexed)", 500, func(ctx context.Context, db *sql.DB) error {
		var id int
		return db.QueryRowContext(ctx, `SELECT id FROM audit_logs WHERE user_id = 1 LIMIT 1`).Scan(&id)
	})
	report.DBResults = append(report.DBResults, indexSelectResult)
	log.Printf("  Index Select: avg=%v, P99=%v\n", indexSelectResult.AvgDuration, indexSelectResult.P99Duration)

	// 4. Paginated SELECT benchmark
	paginatedResult := benchmarkDBOperation("DB Select (paginated)", 300, func(ctx context.Context, db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `SELECT id, user_id, action, sql_content, created_at FROM audit_logs ORDER BY id DESC LIMIT 20 OFFSET 0`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, userID int
			var action, sqlContent string
			var createdAt time.Time
			_ = rows.Scan(&id, &userID, &action, &sqlContent, &createdAt)
		}
		return rows.Err()
	})
	report.DBResults = append(report.DBResults, paginatedResult)
	log.Printf("  Paginated Select: avg=%v, P99=%v\n", paginatedResult.AvgDuration, paginatedResult.P99Duration)

	// 5. Concurrent write benchmark (simulates SQLite single-writer contention)
	log.Println("  [测试] SQLite 并发写入竞争...")
	concWriteResult := benchmarkConcurrentWrites(50, 5)
	report.DBResults = append(report.DBResults, concWriteResult)
	log.Printf("  Concurrent Write: avg=%v, P99=%v (%d goroutines)\n", concWriteResult.AvgDuration, concWriteResult.P99Duration, 5)
}

// setupTestDB creates a temporary SQLite database with full schema for benchmarking.
func setupTestDB() *sql.DB {
	database, err := db.Open(":memory:")
	if err != nil {
		log.Fatalf("failed to create benchmark database: %v", err)
	}
	if err := database.Migrate(); err != nil {
		log.Fatalf("failed to migrate benchmark database: %v", err)
	}
	return database.DB
}

// seedDB populates the database with test data.
func seedDB(ctx context.Context, d *sql.DB) {
	// Insert seed user
	d.ExecContext(ctx, `INSERT INTO users (username, password_hash, role) VALUES ('benchuser', 'hash', 'developer')`)

	// Insert seed audit log entries for query tests
	stmt, err := d.PrepareContext(ctx,
		`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, result_rows, affected_rows, execution_time_ms, error_message, desensitized_fields, ip_address) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("prepare seed: %v", err)
	}
	defer stmt.Close()

	for i := 0; i < 100; i++ {
		_, _ = stmt.ExecContext(ctx,
			1, "SELECT", 1, "testdb",
			fmt.Sprintf("SELECT * FROM table_%d WHERE id = %d", i%10, i),
			fmt.Sprintf("查询表%d", i%10),
			int64(i%100+1), 0, int64(i%10+1), "", "", "192.168.1."+fmt.Sprintf("%d", i%255))
	}
}

// benchmarkDBOperation runs a database operation multiple times and returns timing stats.
func benchmarkDBOperation(name string, iterations int, op func(context.Context, *sql.DB) error) DBQueryResult {
	durations := make([]time.Duration, iterations)
	ctx := context.Background()
	testDB := setupTestDB()
	defer testDB.Close()
	seedDB(ctx, testDB)

	// Warmup
	for i := 0; i < 10; i++ {
		_ = op(ctx, testDB)
	}

	for i := 0; i < iterations; i++ {
		start := time.Now()
		if err := op(ctx, testDB); err != nil {
			log.Printf("  %s iteration %d failed: %v", name, i, err)
		}
		durations[i] = time.Since(start)
	}

	sortDurations(durations)

	return DBQueryResult{
		Name:        name,
		Iterations:  iterations,
		AvgDuration: avgDuration(durations),
		P50Duration: percentile(durations, 50),
		P90Duration: percentile(durations, 90),
		P99Duration: percentile(durations, 99),
		MinDuration: durations[0],
		MaxDuration: durations[len(durations)-1],
	}
}

// benchmarkConcurrentWrites measures write latency under concurrent goroutine contention.
func benchmarkConcurrentWrites(reqsPerGoroutine, goroutines int) DBQueryResult {
	testDB := setupTestDB()
	defer testDB.Close()
	seedDB(context.Background(), testDB)

	var (
		durations []time.Duration
		mu        sync.Mutex
		wg        sync.WaitGroup
	)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for i := 0; i < reqsPerGoroutine; i++ {
				start := time.Now()
				_, _ = testDB.ExecContext(ctx,
					`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, result_rows, affected_rows, execution_time_ms, error_message, desensitized_fields, ip_address) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					1, "INSERT", 1, "bench", "INSERT INTO t VALUES (1)", "bench", 0, 1, 1, "", "", "127.0.0.1")
				dur := time.Since(start)
				mu.Lock()
				durations = append(durations, dur)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	sortDurations(durations)
	totalOps := goroutines * reqsPerGoroutine

	return DBQueryResult{
		Name:          fmt.Sprintf("DB Concurrent Write (%d goroutines × %d)", goroutines, reqsPerGoroutine),
		Iterations:    totalOps,
		AvgDuration:   avgDuration(durations),
		P50Duration:   percentile(durations, 50),
		P90Duration:   percentile(durations, 90),
		P99Duration:   percentile(durations, 99),
		MinDuration:   durations[0],
		MaxDuration:   durations[len(durations)-1],
		RowsPerSecond: float64(totalOps) / avgDuration(durations).Seconds(),
	}
}

// extractTokenFromLogin performs a login and extracts the JWT token from the response.
func extractTokenFromLogin(baseURL string) string {
	// SECURITY: InsecureSkipVerify is only acceptable here because this function
	// is exclusively called during automated performance benchmarks against a
	// local/self-signed test server. Never use InsecureSkipVerify in production code.
	// The main benchmark client respects BenchmarkConfig.InsecureTLS (default: false).
	body := `{"username":"admin","password":"admin123"}`
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Post(baseURL+"/api/auth/login", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		log.Printf("  login failed: %v", err)
		return ""
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return ""
	}

	// Navigate response envelope: {"code":0,"message":"ok","data":{"token":"...","user":{}}}
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return ""
	}

	// Try "access_token" first, then fall back to "token"
	if token, ok := data["access_token"].(string); ok {
		return token
	}
	if token, ok := data["token"].(string); ok {
		return token
	}

	return ""
}
