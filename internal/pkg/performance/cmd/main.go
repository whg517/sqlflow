// Command performance runs the full performance test suite against a running sqlflow server.
//
// Usage:
//
//	go run ./internal/pkg/performance/cmd/... -url http://localhost:8080 -reqs 200 -concurrency 20
//
// Flags:
//
//	-url         Server base URL (default: http://localhost:8080)
//	-reqs        Total requests per endpoint (default: 200)
//	-concurrency Concurrent goroutines (default: 20)
//	-output      Output file path for JSON report (default: stdout)
//	-version     Print version and exit
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/whg517/sqlflow/internal/pkg/performance"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
)

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "Server base URL")
	totalReqs := flag.Int("reqs", 200, "Total requests per endpoint")
	concurrency := flag.Int("concurrency", 20, "Concurrent goroutines")
	output := flag.String("output", "", "Output JSON report to file (default: stdout)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("sqlflow-performance %s (built %s)\n", version, buildTime)
		return
	}

	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.SetOutput(os.Stderr)

	report := performance.RunFullBenchmark(*baseURL, *totalReqs, *concurrency)

	// Output JSON report
	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("failed to marshal report: %v", err)
	}

	if *output != "" {
		if err := os.WriteFile(*output, jsonBytes, 0644); err != nil {
			log.Fatalf("failed to write report: %v", err)
		}
		log.Printf("📊 报告已保存到: %s\n", *output)
	} else {
		fmt.Println("\n" + string(jsonBytes))
	}
}
