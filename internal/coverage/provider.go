package coverage

import (
	"context"
	"io"
)

type ParseResult struct {
	ReportType       ReportType     `json:"report_type"`
	Language         Language       `json:"language"`
	LineCoverage     float64        `json:"line_coverage"`
	BranchCoverage   float64        `json:"branch_coverage"`
	FuncCoverage     float64        `json:"func_coverage"`
	TotalLines       int            `json:"total_lines"`
	CoveredLines     int            `json:"covered_lines"`
	TotalBranches    int            `json:"total_branches"`
	CoveredBranches  int            `json:"covered_branches"`
	TotalFunctions   int            `json:"total_functions"`
	CoveredFunctions int            `json:"covered_functions"`
	Modules          []ParsedModule `json:"modules"`
	Warnings         []ParseWarning `json:"warnings,omitempty"`
}

type ParsedModule struct {
	ModulePath       string       `json:"module_path"`
	ModuleName       string       `json:"module_name"`
	LineCoverage     float64      `json:"line_coverage"`
	BranchCoverage   float64      `json:"branch_coverage"`
	FuncCoverage     float64      `json:"func_coverage"`
	TotalLines       int          `json:"total_lines"`
	CoveredLines     int          `json:"covered_lines"`
	TotalBranches    int          `json:"total_branches"`
	CoveredBranches  int          `json:"covered_branches"`
	TotalFunctions   int          `json:"total_functions"`
	CoveredFunctions int          `json:"covered_functions"`
	FileCount        int          `json:"file_count"`
	Files            []ParsedFile `json:"files"`
}

type ParsedFile struct {
	FilePath         string  `json:"file_path"`
	FileName         string  `json:"file_name"`
	LineCoverage     float64 `json:"line_coverage"`
	BranchCoverage   float64 `json:"branch_coverage"`
	FuncCoverage     float64 `json:"func_coverage"`
	TotalLines       int     `json:"total_lines"`
	CoveredLines     int     `json:"covered_lines"`
	TotalBranches    int     `json:"total_branches"`
	CoveredBranches  int     `json:"covered_branches"`
	TotalFunctions   int     `json:"total_functions"`
	CoveredFunctions int     `json:"covered_functions"`
	UncoveredLines   []int   `json:"uncovered_lines"`
}

type ParseWarning struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type CoverageProvider interface {
	Name() ReportType
	SupportedLanguages() []Language
	Parse(ctx context.Context, reader io.Reader) (*ParseResult, error)
	ParseFile(ctx context.Context, path string) (*ParseResult, error)
}

type FormatDetector interface {
	Detect(data []byte) bool
}

type GateEvaluator interface {
	Evaluate(ctx context.Context, project string, result *ParseResult) (*GateEvalResult, error)
	GetConfig(ctx context.Context, project, modulePath string) (*GateConfig, error)
}

type GateEvalResult struct {
	Passed  bool               `json:"passed"`
	Mode    GateMode           `json:"mode"`
	Results []ModuleGateResult `json:"results"`
}

type ModuleGateResult struct {
	ModulePath string              `json:"module_path"`
	Level      CoverageLevel       `json:"level"`
	Value      float64             `json:"value"`
	GatePassed bool                `json:"gate_passed"`
	Thresholds CoverageLevelResult `json:"thresholds"`
}
