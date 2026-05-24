// Package performance provides benchmarking utilities for sqlflow.
// It includes concurrent HTTP endpoint benchmarks, database query benchmarks,
// and automated report generation with baseline comparison.
package performance

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"time"
)

// RequestSpec defines a single HTTP request for benchmarking.
type RequestSpec struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    io.Reader // optional request body; if string, will be wrapped with io.Reader
}

// LatencyRecord captures timing details for a single request.
type LatencyRecord struct {
	Total        time.Duration
	DNS          time.Duration
	TCPConnect   time.Duration
	TLSHandshake time.Duration
	FirstByte    time.Duration
	StatusCode   int
	Error        error
}

// Result holds aggregated benchmark results for a test scenario.
type Result struct {
	Name           string        `json:"name"`
	Concurrency    int           `json:"concurrency"`
	TotalRequests  int           `json:"total_requests"`
	SuccessCount   int64         `json:"success_count"`
	FailureCount   int64         `json:"failure_count"`
	TotalDuration  time.Duration `json:"total_duration"`
	AvgLatency     time.Duration `json:"avg_latency"`
	P50Latency     time.Duration `json:"p50_latency"`
	P90Latency     time.Duration `json:"p90_latency"`
	P95Latency     time.Duration `json:"p95_latency"`
	P99Latency     time.Duration `json:"p99_latency"`
	MinLatency     time.Duration `json:"min_latency"`
	MaxLatency     time.Duration `json:"max_latency"`
	RequestsPerSec float64       `json:"requests_per_sec"`
	ErrorMessages  []string      `json:"error_messages,omitempty"`
}

// DBQueryResult holds benchmark results for database queries.
type DBQueryResult struct {
	Name          string        `json:"name"`
	Iterations    int           `json:"iterations"`
	AvgDuration   time.Duration `json:"avg_duration"`
	P50Duration   time.Duration `json:"p50_duration"`
	P90Duration   time.Duration `json:"p90_duration"`
	P99Duration   time.Duration `json:"p99_duration"`
	MinDuration   time.Duration `json:"min_duration"`
	MaxDuration   time.Duration `json:"max_duration"`
	RowsPerSecond float64       `json:"rows_per_second,omitempty"`
}

// BenchmarkConfig controls benchmark behavior.
type BenchmarkConfig struct {
	BaseURL     string
	Concurrency int
	TotalReqs   int
	Timeout     time.Duration
	InsecureTLS bool
}

// DefaultBenchmarkConfig returns sensible defaults.
func DefaultBenchmarkConfig(baseURL string) *BenchmarkConfig {
	return &BenchmarkConfig{
		BaseURL:     baseURL,
		Concurrency: 10,
		TotalReqs:   100,
		Timeout:     30 * time.Second,
		InsecureTLS: false,
	}
}

// NewBenchmarkResult initializes a Result with the given name and config.
func NewBenchmarkResult(name string, cfg *BenchmarkConfig) *Result {
	return &Result{
		Name:          name,
		Concurrency:   cfg.Concurrency,
		TotalRequests: cfg.TotalReqs,
	}
}

// RunHTTPBenchmark executes a concurrent HTTP load test against a single endpoint.
func RunHTTPBenchmark(name string, spec RequestSpec, cfg *BenchmarkConfig) *Result {
	result := NewBenchmarkResult(name, cfg)

	client := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: cfg.InsecureTLS},
			MaxIdleConnsPerHost: cfg.Concurrency,
			MaxIdleConns:        cfg.Concurrency * 2,
			DisableKeepAlives:   false,
		},
	}

	var (
		latencies  []time.Duration
		mu         sync.Mutex
		wg         sync.WaitGroup
		reqCounter int64
		successCnt int64
		failureCnt int64
		errorMsgs  []string
		errorMu    sync.Mutex
	)

	// Work dispatcher
	sem := make(chan struct{}, cfg.Concurrency)
	start := time.Now()

	for i := 0; i < cfg.TotalReqs; i++ {
		wg.Add(1)
		sem <- struct{}{} // acquire slot

		go func() {
			defer wg.Done()
			defer func() { <-sem }() // release slot

			record := executeRequest(client, spec)
			cur := atomic.AddInt64(&reqCounter, 1)

			mu.Lock()
			latencies = append(latencies, record.Total)
			mu.Unlock()

			if record.Error != nil {
				atomic.AddInt64(&failureCnt, 1)
				if len(errorMsgs) < 20 { // cap error messages
					errorMu.Lock()
					errorMsgs = append(errorMsgs, fmt.Sprintf("req #%d: %v", cur, record.Error))
					errorMu.Unlock()
				}
			} else {
				atomic.AddInt64(&successCnt, 1)
			}
		}()
	}

	wg.Wait()
	result.TotalDuration = time.Since(start)
	result.SuccessCount = successCnt
	result.FailureCount = failureCnt
	result.ErrorMessages = errorMsgs

	if len(latencies) > 0 {
		sortDurations(latencies)
		result.AvgLatency = avgDuration(latencies)
		result.P50Latency = percentile(latencies, 50)
		result.P90Latency = percentile(latencies, 90)
		result.P95Latency = percentile(latencies, 95)
		result.P99Latency = percentile(latencies, 99)
		result.MinLatency = latencies[0]
		result.MaxLatency = latencies[len(latencies)-1]
		result.RequestsPerSec = float64(successCnt) / result.TotalDuration.Seconds()
	}

	return result
}

// executeRequest performs a single HTTP request and records latency breakdown.
func executeRequest(client *http.Client, spec RequestSpec) LatencyRecord {
	var dnsStart, tcpStart, tlsStart, firstByteStart time.Time
	var dnsDur, tcpDur, tlsDur, firstByteDur time.Duration

	trace := &httptrace.ClientTrace{
		DNSStart:             func(_ httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:              func(_ httptrace.DNSDoneInfo) { dnsDur = time.Since(dnsStart) },
		TLSHandshakeStart:    func() { tlsStart = time.Now() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tlsDur = time.Since(tlsStart) },
		ConnectStart:         func(_, _ string) { tcpStart = time.Now() },
		ConnectDone:          func(_, _ string, err error) { tcpDur = time.Since(tcpStart) },
		GotFirstResponseByte: func() { firstByteDur = time.Since(firstByteStart) },
	}

	req, err := http.NewRequest(spec.Method, spec.URL, spec.Body)
	if err != nil {
		return LatencyRecord{Error: fmt.Errorf("create request: %w", err)}
	}
	for k, v := range spec.Headers {
		req.Header.Set(k, v)
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	start := time.Now()
	firstByteStart = start

	resp, err := client.Do(req)
	if err != nil {
		return LatencyRecord{
			Total:        time.Since(start),
			DNS:          dnsDur,
			TCPConnect:   tcpDur,
			TLSHandshake: tlsDur,
			Error:        err,
		}
	}
	defer resp.Body.Close()

	// Drain body to ensure full read timing
	_, _ = io.Copy(io.Discard, resp.Body)

	return LatencyRecord{
		Total:        time.Since(start),
		DNS:          dnsDur,
		TCPConnect:   tcpDur,
		TLSHandshake: tlsDur,
		FirstByte:    firstByteDur,
		StatusCode:   resp.StatusCode,
	}
}

// percentile returns the duration at the given percentile from a sorted slice.
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// avgDuration computes the average duration.
func avgDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

// sortDurations sorts durations in ascending order (insertion sort for small slices,
// shell sort for larger ones — avoids importing sort package).
func sortDurations(d []time.Duration) {
	n := len(d)
	if n <= 1 {
		return
	}
	// Shell sort — good enough for benchmark data sizes
	for gap := n / 2; gap > 0; gap /= 2 {
		for i := gap; i < n; i++ {
			for j := i; j >= gap && d[j-gap] > d[j]; j -= gap {
				d[j], d[j-gap] = d[j-gap], d[j]
			}
		}
	}
}

// JSON returns the result as a JSON string.
func (r *Result) JSON() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

// JSON returns the DB query result as a JSON string.
func (r *DBQueryResult) JSON() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}
