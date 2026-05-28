package coverage

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestLCOVProvider_Parse(t *testing.T) {
	input := `TN:go test
SF:internal/service/user.go
DA:10,1
DA:11,1
DA:12,0
DA:13,1
DA:15,0
FN:10,NewUser
FN:20,GetUser
FNDA:1,NewUser
FNDA:0,GetUser
BRF:4
BRH:3
end_of_record
SF:internal/service/order.go
DA:5,1
DA:6,1
DA:7,1
DA:8,1
FN:5,CreateOrder
FNDA:1,CreateOrder
end_of_record
SF:pkg/utils/helper.go
DA:1,1
DA:2,0
DA:3,1
end_of_record
`
	p := NewLCOVProvider()
	result, err := p.Parse(context.Background(), strings.NewReader(input))
	if err != nil { t.Fatalf("Parse failed: %v", err) }
	if result.TotalLines != 12 { t.Errorf("TotalLines = %d, want 12", result.TotalLines) }
	if result.CoveredLines != 9 { t.Errorf("CoveredLines = %d, want 9", result.CoveredLines) }
	if result.TotalBranches != 4 { t.Errorf("TotalBranches = %d, want 4", result.TotalBranches) }
	if result.CoveredBranches != 3 { t.Errorf("CoveredBranches = %d, want 3", result.CoveredBranches) }
	if result.TotalFunctions != 3 { t.Errorf("TotalFunctions = %d, want 3", result.TotalFunctions) }
	if result.CoveredFunctions != 2 { t.Errorf("CoveredFunctions = %d, want 2", result.CoveredFunctions) }
	if len(result.Modules) != 2 { t.Fatalf("Modules count = %d, want 2", len(result.Modules)) }
}

func TestLCOVProvider_EmptyInput(t *testing.T) {
	p := NewLCOVProvider()
	_, err := p.Parse(context.Background(), strings.NewReader(""))
	if err == nil { t.Error("expected error for empty input") }
}

func TestLCOVProvider_DAWithoutSF(t *testing.T) {
	input := "DA:1,1\nSF:real.go\nDA:1,1\nend_of_record\n"
	p := NewLCOVProvider()
	result, err := p.Parse(context.Background(), strings.NewReader(input))
	if err != nil { t.Fatalf("Parse failed: %v", err) }
	if result.TotalLines != 1 { t.Errorf("TotalLines = %d, want 1", result.TotalLines) }
}

func TestLCOVProvider_UnterminatedRecord(t *testing.T) {
	input := "SF:test.go\nDA:1,1\nDA:2,0\n"
	p := NewLCOVProvider()
	result, err := p.Parse(context.Background(), strings.NewReader(input))
	if err != nil { t.Fatalf("Parse failed: %v", err) }
	if result.TotalLines != 2 { t.Errorf("TotalLines = %d, want 2", result.TotalLines) }
	if result.LineCoverage != 50.0 { t.Errorf("LineCoverage = %v, want 50", result.LineCoverage) }
}

func TestLCOVProvider_Detect(t *testing.T) {
	p := NewLCOVProvider()
	detector, ok := p.(FormatDetector)
	if !ok { t.Fatal("LCOV provider does not implement FormatDetector") }
	if !detector.Detect([]byte("TN:test\nSF:foo")) { t.Error("should detect TN: prefix") }
	if !detector.Detect([]byte("SF:main.go")) { t.Error("should detect SF: prefix") }
	if detector.Detect([]byte("<?xml")) { t.Error("should not detect XML") }
}

func TestMergeResults_DifferentFiles(t *testing.T) {
	r1 := &ParseResult{Modules: []ParsedModule{{
		ModulePath: "internal/service", TotalLines: 60, CoveredLines: 50,
		Files: []ParsedFile{{FilePath: "a.go", TotalLines: 60, CoveredLines: 50}},
	}}}
	r2 := &ParseResult{Modules: []ParsedModule{{
		ModulePath: "internal/service", TotalLines: 50, CoveredLines: 40,
		Files: []ParsedFile{{FilePath: "c.go", TotalLines: 50, CoveredLines: 40}},
	}}}
	merged := MergeResults([]*ParseResult{r1, r2})
	if merged.TotalLines != 110 { t.Errorf("TotalLines = %d, want 110", merged.TotalLines) }
	if merged.CoveredLines != 90 { t.Errorf("CoveredLines = %d, want 90", merged.CoveredLines) }
}

// MUST-4: same file in multiple jobs should NOT double-count.
func TestMergeResults_SameFile(t *testing.T) {
	r1 := &ParseResult{Modules: []ParsedModule{{
		ModulePath: "internal/service", TotalLines: 100, CoveredLines: 80,
		Files: []ParsedFile{{
			FilePath: "handler.go", TotalLines: 100, CoveredLines: 80,
			UncoveredLines: []int{5, 10, 15, 20}, // 4 uncovered
		}},
	}}}
	r2 := &ParseResult{Modules: []ParsedModule{{
		ModulePath: "internal/service", TotalLines: 100, CoveredLines: 85,
		Files: []ParsedFile{{
			FilePath: "handler.go", TotalLines: 100, CoveredLines: 85,
			UncoveredLines: []int{5, 10, 30}, // 3 uncovered, but only 5 and 10 overlap with r1
		}},
	}}}
	merged := MergeResults([]*ParseResult{r1, r2})
	// Same file should be deduplicated: 1 file, not 2
	if len(merged.Modules) != 1 { t.Fatalf("Modules = %d, want 1", len(merged.Modules)) }
	if merged.Modules[0].FileCount != 1 { t.Errorf("FileCount = %d, want 1", merged.Modules[0].FileCount) }
	// Uncovered = intersection of {5,10,15,20} ∩ {5,10,30} = {5,10}
	// Covered = 100 - 2 = 98
	if merged.CoveredLines != 98 { t.Errorf("CoveredLines = %d, want 98 (NOT 80+85=165)", merged.CoveredLines) }
	if merged.TotalLines != 100 { t.Errorf("TotalLines = %d, want 100 (NOT 200)", merged.TotalLines) }
}

func TestMergeResults_Single(t *testing.T) {
	r := &ParseResult{TotalLines: 100, CoveredLines: 80}
	merged := MergeResults([]*ParseResult{r})
	if merged.TotalLines != 100 { t.Errorf("single merge should be identity") }
}

func TestMergeResults_Empty(t *testing.T) {
	if MergeResults(nil) != nil { t.Error("nil input should return nil") }
}

func TestEvaluateCoverageLevel(t *testing.T) {
	config := &GateConfig{ThresholdRed: 50, ThresholdOrange: 70, ThresholdYellow: 80, ThresholdGreen: 90, GateMode: GateModeBlock, GateThreshold: 80}
	tests := []struct {
		value      float64
		wantLevel  CoverageLevel
		wantPassed bool
	}{
		{30, LevelRed, false}, {60, LevelRed, false}, {75, LevelOrange, false},
		{85, LevelYellow, true}, {95, LevelGreen, true},
	}
	for _, tt := range tests {
		result := EvaluateCoverageLevel(tt.value, config)
		if result.Level != tt.wantLevel { t.Errorf("value=%v level=%v want=%v", tt.value, result.Level, tt.wantLevel) }
		if result.GatePassed != tt.wantPassed { t.Errorf("value=%v passed=%v want=%v", tt.value, result.GatePassed, tt.wantPassed) }
	}
}

func TestMatchModulePattern(t *testing.T) {
	tests := []struct{ path, pattern string; want bool }{
		{"internal/foo", "", true},
		{"internal/service", "internal/service", true},
		{"internal/handler", "internal/service", false},
		{"internal/service/auth", "internal/**", true},
		{"pkg/db", "pkg/*", true},
		{"pkg/db/sub", "pkg/*", false},
		{"cmd/server/handler", "**/handler", true},
	}
	for _, tt := range tests {
		got, err := MatchModulePattern(tt.path, tt.pattern)
		if err != nil { t.Fatalf("unexpected error: %v", err) }
		if got != tt.want { t.Errorf("MatchModulePattern(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want) }
	}
}

func TestSanitizeFilePath(t *testing.T) {
	tests := []struct{ input, want string }{
		{"/home/user/project/main.go", "~/project/main.go"},
		{"/Users/john/code/app.js", "~/code/app.js"},
		{"/home/runner/work/project/main.go", "~/work/project/main.go"},
		{"internal/service/handler.go", "internal/service/handler.go"},
	}
	for _, tt := range tests {
		if got := SanitizeFilePath(tt.input); got != tt.want {
			t.Errorf("SanitizeFilePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDefaultGateConfig(t *testing.T) {
	cfg := DefaultGateConfig()
	if cfg.ThresholdGreen != 90 { t.Errorf("ThresholdGreen = %v, want 90", cfg.ThresholdGreen) }
	if cfg.GateMode != GateModeOff { t.Errorf("GateMode = %v, want off", cfg.GateMode) }
}

func TestGlobalRegistry_Init(t *testing.T) {
	// NICE-5: init() should auto-register LCOV
	reg := GlobalRegistry()
	_, err := reg.Lookup(ReportTypeLCOV)
	if err != nil { t.Errorf("LCOV should be auto-registered: %v", err) }
}

func TestParserRegistry_RegisterDuplicate(t *testing.T) {
	reg := NewParserRegistry()
	reg.MustRegister(&lcovProvider{BaseProvider{TypeName: "test_dup"}})
	if err := reg.Register(&lcovProvider{BaseProvider{TypeName: "test_dup"}}); err == nil {
		t.Error("expected error on duplicate registration")
	}
}

func TestLCOVProvider_ExceedMaxLineNum(t *testing.T) {
	input := fmt.Sprintf("SF:test.go\nDA:%d,1\nend_of_record", MaxLineNum+1)
	p := NewLCOVProvider()
	result, _ := p.Parse(context.Background(), strings.NewReader(input))
	hasRangeWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w.Message, "out of range") { hasRangeWarning = true; break }
	}
	if !hasRangeWarning { t.Error("expected warning about line number out of range") }
}
