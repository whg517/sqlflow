package performance

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

// TestBenchmarkSort verifies the sort utility works correctly.
func TestSortDurations(t *testing.T) {
	durations := []time.Duration{
		100 * time.Millisecond,
		10 * time.Millisecond,
		50 * time.Millisecond,
		200 * time.Millisecond,
		1 * time.Millisecond,
	}
	sortDurations(durations)

	if durations[0] != 1*time.Millisecond {
		t.Errorf("expected min 1ms, got %v", durations[0])
	}
	if durations[len(durations)-1] != 200*time.Millisecond {
		t.Errorf("expected max 200ms, got %v", durations[len(durations)-1])
	}
	if durations[2] != 50*time.Millisecond {
		t.Errorf("expected middle 50ms, got %v", durations[2])
	}
}

// TestPercentile verifies percentile calculation.
func TestPercentile(t *testing.T) {
	durations := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		durations[i] = time.Duration(i+1) * time.Millisecond
	}

	p50 := percentile(durations, 50)
	if p50 != 50*time.Millisecond {
		t.Errorf("P50 = %v, want 50ms", p50)
	}

	p99 := percentile(durations, 99)
	if p99 != 99*time.Millisecond {
		t.Errorf("P99 = %v, want 99ms", p99)
	}

	p100 := percentile(durations, 100)
	if p100 != 100*time.Millisecond {
		t.Errorf("P100 = %v, want 100ms", p100)
	}

	// Edge cases
	empty := []time.Duration{}
	if p := percentile(empty, 50); p != 0 {
		t.Errorf("percentile of empty slice = %v, want 0", p)
	}

	single := []time.Duration{5 * time.Millisecond}
	if p := percentile(single, 99); p != 5*time.Millisecond {
		t.Errorf("percentile of single element = %v, want 5ms", p)
	}
}

// TestAvgDuration verifies average calculation.
func TestAvgDuration(t *testing.T) {
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
	}
	avg := avgDuration(durations)
	if avg != 20*time.Millisecond {
		t.Errorf("avg = %v, want 20ms", avg)
	}

	// Empty case
	if avg := avgDuration(nil); avg != 0 {
		t.Errorf("avg of nil = %v, want 0", avg)
	}
}

// TestDefaultBaseline verifies baseline values are reasonable.
func TestDefaultBaseline(t *testing.T) {
	b := DefaultBaseline()
	if b.APILoginP95 == 0 {
		t.Error("APILoginP95 should not be zero")
	}
	if b.MinQPS < 1 {
		t.Error("MinQPS should be at least 1")
	}
	if b.DBIndexSelectP99 > b.DBSelectP99 {
		t.Error("indexed select should be faster than full scan")
	}
}

func TestDefaultBenchmarkConfig(t *testing.T) {
	cfg := DefaultBenchmarkConfig("http://localhost:8080")
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want http://localhost:8080", cfg.BaseURL)
	}
	if cfg.Concurrency != 10 {
		t.Errorf("Concurrency = %d, want 10", cfg.Concurrency)
	}
	if cfg.TotalReqs != 100 {
		t.Errorf("TotalReqs = %d, want 100", cfg.TotalReqs)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
	if cfg.InsecureTLS {
		t.Error("InsecureTLS should be false by default")
	}
}

// TestNewReport verifies report creation.
func TestNewReport(t *testing.T) {
	r := NewReport(nil)
	if r.Baseline == nil {
		t.Error("baseline should not be nil")
	}
	if r.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

// TestBaselineResultJSON verifies JSON serialization.
func TestBaselineResultJSON(t *testing.T) {
	r := NewBenchmarkResult("TestEndpoint", &BenchmarkConfig{
		BaseURL:     "http://localhost:8080",
		Concurrency: 5,
		TotalReqs:   50,
	})
	r.SuccessCount = 48
	r.FailureCount = 2
	r.AvgLatency = 10 * time.Millisecond

	json := r.JSON()
	if json == "" {
		t.Error("JSON output should not be empty")
	}
}

// TestGenerateSummary verifies summary generation.
func TestGenerateSummary(t *testing.T) {
	r := NewReport(nil)
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:          "Login (POST /api/auth/login)",
		Concurrency:   10,
		TotalRequests: 100,
		SuccessCount:  95,
		FailureCount:  5,
		AvgLatency:    50 * time.Millisecond,
		P95Latency:    200 * time.Millisecond,
		P99Latency:    500 * time.Millisecond,
		RequestsPerSec: 95.0,
	})

	r.CheckBaselines()
	summary := r.GenerateSummary()

	if summary == "" {
		t.Error("summary should not be empty")
	}
}

// TestDBBenchmarkOperation verifies database benchmark execution.
func TestDBBenchmarkOperation(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	// Seed user
	database.ExecContext(ctx, `INSERT INTO users (username, password_hash, role) VALUES ('test', 'hash', 'developer')`)

	result := benchmarkDBOperation("Test Count Query", 50, func(ctx context.Context, db *sql.DB) error {
		var count int
		return db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	})

	if result.Iterations != 50 {
		t.Errorf("iterations = %d, want 50", result.Iterations)
	}
	if result.AvgDuration == 0 {
		t.Error("avg duration should not be zero")
	}
	if result.MinDuration > result.MaxDuration {
		t.Error("min should be <= max")
	}
	if result.MinDuration > result.P50Duration {
		t.Error("min should be <= P50")
	}
}

// TestDBBenchmarkOperationEmpty verifies handling of empty results.
func TestDBBenchmarkOperationEmpty(t *testing.T) {
	result := DBQueryResult{}
	json := result.JSON()
	if json == "" {
		t.Error("JSON should not be empty for zero-value struct")
	}
}

// TestConcurrentWriteBenchmark verifies concurrent write benchmark.
func TestConcurrentWriteBenchmark(t *testing.T) {
	result := benchmarkConcurrentWrites(10, 3)
	if result.Iterations != 30 {
		t.Errorf("iterations = %d, want 30", result.Iterations)
	}
	if result.AvgDuration == 0 {
		t.Error("avg duration should not be zero")
	}
	if result.RowsPerSecond <= 0 {
		t.Error("rows per second should be positive")
	}
}

// TestBaselineCheck verifies baseline checking logic.
func TestBaselineCheck(t *testing.T) {
	r := NewReport(nil)
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:        "Login (POST /api/auth/login)",
		P95Latency:  100 * time.Millisecond, // under baseline
	})
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:        "Dashboard Stats (GET /api/dashboard/stats)",
		P95Latency:  500 * time.Millisecond, // over baseline (200ms)
	})
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:        "Ticket Create (POST /api/tickets)",
		P95Latency:  1000 * time.Millisecond, // over baseline (500ms)
	})

	r.CheckBaselines()

	if len(r.BaselinePass) != 3 {
		t.Errorf("expected 3 baseline checks, got %d", len(r.BaselinePass))
	}

	// Login should pass
	if !r.BaselinePass[0].Passed {
		t.Error("login baseline should pass")
	}
	// Dashboard should fail
	if r.BaselinePass[1].Passed {
		t.Error("dashboard baseline should fail")
	}
}

func TestCheckBaselines_SkipUnknown(t *testing.T) {
	r := NewReport(nil)
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:       "Unknown Endpoint",
		P95Latency: 50 * time.Millisecond,
	})

	r.CheckBaselines()
	if len(r.BaselinePass) != 0 {
		t.Errorf("expected 0 baseline checks for unknown endpoint, got %d", len(r.BaselinePass))
	}
}

func TestGenerateSummary_WithDBResults(t *testing.T) {
	r := NewReport(nil)
	r.DBResults = append(r.DBResults, DBQueryResult{
		Name:        "DB Insert (single row)",
		Iterations:  100,
		AvgDuration: 1 * time.Millisecond,
		P50Duration: 1 * time.Millisecond,
		P90Duration: 2 * time.Millisecond,
		P99Duration: 5 * time.Millisecond,
		MinDuration: 0,
		MaxDuration: 10 * time.Millisecond,
	})

	r.GenerateSummary()
	if r.Summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestGenerateSummary_WithErrors(t *testing.T) {
	r := NewReport(nil)
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:          "Failing Endpoint",
		Concurrency:   10,
		TotalRequests: 100,
		SuccessCount:  50,
		FailureCount:  50,
		AvgLatency:    10 * time.Millisecond,
		P95Latency:    20 * time.Millisecond,
		P99Latency:    30 * time.Millisecond,
		ErrorMessages: []string{"connection refused"},
	})

	r.GenerateSummary()
	if r.Summary == "" {
		t.Error("summary should not be empty")
	}
	if !strings.Contains(r.Summary, "connection refused") {
		t.Error("summary should contain error message")
	}
}

func TestSortDurations_EmptyAndSingle(t *testing.T) {
	// Empty slice
	durations := []time.Duration{}
	sortDurations(durations) // should not panic

	// Single element
	single := []time.Duration{5 * time.Millisecond}
	sortDurations(single)
	if single[0] != 5*time.Millisecond {
		t.Errorf("single element sort: got %v, want 5ms", single[0])
	}
}

func TestPercentile_EdgeCases(t *testing.T) {
	// P0 should return first element
	durations := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	p0 := percentile(durations, 0)
	if p0 != 10*time.Millisecond {
		t.Errorf("P0 = %v, want 10ms", p0)
	}

	// Very large percentile
	p999 := percentile(durations, 999)
	if p999 != 30*time.Millisecond {
		t.Errorf("P999 = %v, want 30ms (clamped to last element)", p999)
	}
}

func TestNewBenchmarkResult(t *testing.T) {
	cfg := &BenchmarkConfig{
		BaseURL:     "http://localhost:8080",
		Concurrency: 20,
		TotalReqs:   200,
	}

	r := NewBenchmarkResult("test", cfg)
	if r.Name != "test" {
		t.Errorf("Name = %q, want %q", r.Name, "test")
	}
	if r.Concurrency != 20 {
		t.Errorf("Concurrency = %d, want 20", r.Concurrency)
	}
	if r.TotalRequests != 200 {
		t.Errorf("TotalRequests = %d, want 200", r.TotalRequests)
	}
}

func TestReportJSON(t *testing.T) {
	r := NewReport(nil)
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:         "Test",
		Concurrency:  5,
		TotalRequests: 10,
		SuccessCount: 10,
		AvgLatency:   10 * time.Millisecond,
	})

	json := r.JSON()
	if json == "" {
		t.Error("Report JSON should not be empty")
	}
}

func TestGenerateSuggestions_FailureRate(t *testing.T) {
	r := NewReport(nil)
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:          "Flaky Endpoint",
		TotalRequests: 100,
		FailureCount:  10, // 10% failure rate
		P99Latency:    100 * time.Millisecond,
		RequestsPerSec: 50,
	})

	suggestions := generateSuggestions(r)
	found := false
	for _, s := range suggestions {
		if len(s) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected failure rate suggestion")
	}
}

func TestGenerateSuggestions_HighP99(t *testing.T) {
	r := NewReport(nil)
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:          "Slow Endpoint",
		TotalRequests: 100,
		FailureCount:  0,
		P99Latency:    5 * time.Second, // over 2s
		RequestsPerSec: 50,
	})

	suggestions := generateSuggestions(r)
	found := false
	for _, s := range suggestions {
		if len(s) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected high P99 suggestion")
	}
}

func TestGenerateSuggestions_LowQPS(t *testing.T) {
	r := NewReport(nil)
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:          "Bottleneck Endpoint",
		TotalRequests: 100,
		FailureCount:  0,
		P99Latency:    100 * time.Millisecond,
		RequestsPerSec: 5, // under 10
	})

	suggestions := generateSuggestions(r)
	found := false
	for _, s := range suggestions {
		if len(s) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected low QPS suggestion")
	}
}

func TestGenerateSuggestions_AllPass(t *testing.T) {
	r := NewReport(nil)
	r.HTTPResults = append(r.HTTPResults, &Result{
		Name:          "Good Endpoint",
		TotalRequests: 100,
		FailureCount:  0,
		P99Latency:    100 * time.Millisecond,
		RequestsPerSec: 100,
	})

	suggestions := generateSuggestions(r)
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions, got %d", len(suggestions))
	}
}

func TestGenerateSuggestions_DBSlowInsert(t *testing.T) {
	r := NewReport(nil)
	r.DBResults = append(r.DBResults, DBQueryResult{
		Name:        "DB Insert (single row)",
		P99Duration: 500 * time.Millisecond, // over 100ms
	})

	suggestions := generateSuggestions(r)
	found := false
	for _, s := range suggestions {
		if len(s) > 0 && len(s) > 20 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected slow insert suggestion")
	}
}

func TestGenerateSuggestions_DBSlowSelect(t *testing.T) {
	r := NewReport(nil)
	r.DBResults = append(r.DBResults, DBQueryResult{
		Name:        "DB Select (COUNT)",
		P99Duration: 200 * time.Millisecond, // over 50ms
	})

	suggestions := generateSuggestions(r)
	found := false
	for _, s := range suggestions {
		if len(s) > 0 && len(s) > 20 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected slow select suggestion")
	}
}

// TestResultJSONSerialization verifies Result struct JSON output.
func TestResultJSONSerialization(t *testing.T) {
	r := &Result{
		Name:          "Test",
		Concurrency:   5,
		TotalRequests: 10,
		SuccessCount:  10,
		FailureCount:  0,
		TotalDuration: 100 * time.Millisecond,
		AvgLatency:    10 * time.Millisecond,
		P50Latency:    10 * time.Millisecond,
		P90Latency:    15 * time.Millisecond,
		P95Latency:    20 * time.Millisecond,
		P99Latency:    25 * time.Millisecond,
		MinLatency:    5 * time.Millisecond,
		MaxLatency:    30 * time.Millisecond,
		RequestsPerSec: 100.0,
	}
	json := r.JSON()
	if len(json) == 0 {
		t.Error("JSON output should not be empty")
	}
}

// TestSeedDB verifies database seeding.
func TestSeedDB(t *testing.T) {
	testDB := setupTestDB()
	defer testDB.Close()

	ctx := context.Background()
	seedDB(ctx, testDB)

	var count int
	err := testDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 100 {
		t.Errorf("expected 100 seed rows, got %d", count)
	}

	var userCount int
	err = testDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if userCount != 1 {
		t.Errorf("expected 1 seed user, got %d", userCount)
	}
}

// BenchmarkSortDurations is a Go benchmark for the sort utility.
func BenchmarkSortDurations(b *testing.B) {
	durations := make([]time.Duration, 1000)
	for i := 0; i < 1000; i++ {
		durations[i] = time.Duration(i % 500)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sortDurations(durations)
	}
}

// BenchmarkDBInsert benchmarks single-row inserts.
func BenchmarkDBInsert(b *testing.B) {
	testDB := setupTestDB()
	defer testDB.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testDB.ExecContext(ctx,
			`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, result_rows, affected_rows, execution_time_ms, error_message, desensitized_fields, ip_address) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			1, "SELECT", 1, "bench", "SELECT 1", "bench", 1, 0, 1, "", "", "127.0.0.1")
	}
}

// BenchmarkDBSelectCount benchmarks COUNT queries.
func BenchmarkDBSelectCount(b *testing.B) {
	testDB := setupTestDB()
	defer testDB.Close()
	ctx := context.Background()
	seedDB(ctx, testDB)

	var count int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&count)
	}
}

// BenchmarkDBIndexedSelect benchmarks indexed SELECT queries.
func BenchmarkDBIndexedSelect(b *testing.B) {
	testDB := setupTestDB()
	defer testDB.Close()
	ctx := context.Background()
	seedDB(ctx, testDB)

	var id int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testDB.QueryRowContext(ctx, `SELECT id FROM audit_logs WHERE user_id = 1 LIMIT 1`).Scan(&id)
	}
}

// BenchmarkDBPaginatedSelect benchmarks paginated SELECT queries.
func BenchmarkDBPaginatedSelect(b *testing.B) {
	testDB := setupTestDB()
	defer testDB.Close()
	ctx := context.Background()
	seedDB(ctx, testDB)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := testDB.QueryContext(ctx,
			`SELECT id, user_id, action, sql_content, created_at FROM audit_logs ORDER BY id DESC LIMIT 20 OFFSET 0`)
		if err != nil {
			b.Fatal(err)
		}
		for rows.Next() {
			var id, userID int
			var action, sqlContent string
			var createdAt time.Time
			rows.Scan(&id, &userID, &action, &sqlContent, &createdAt)
		}
		rows.Close()
	}
}
