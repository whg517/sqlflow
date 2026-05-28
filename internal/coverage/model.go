package coverage

import (
	"time"
)

type ReportType string

const (
	ReportTypeLCOV      ReportType = "lcov"
	ReportTypeCobertura ReportType = "cobertura"
	ReportTypeJaCoCo    ReportType = "jacoco"
	ReportTypeGoCover   ReportType = "go_cover"
	ReportTypeCustom    ReportType = "custom"
)

type Language string

const (
	LanguageGo         Language = "go"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
	LanguagePython     Language = "python"
	LanguageJava       Language = "java"
)

type GateMode string

const (
	GateModeOff   GateMode = "off"
	GateModeWarn  GateMode = "warn"
	GateModeBlock GateMode = "block"
)

type CoverageType string

const (
	CoverageTypeLine   CoverageType = "line"
	CoverageTypeBranch CoverageType = "branch"
	CoverageTypeFunc   CoverageType = "func"
)

type AuditAction string

const (
	AuditActionUpload           AuditAction = "upload"
	AuditActionDelete           AuditAction = "delete"
	AuditActionMerge            AuditAction = "merge"
	AuditActionGateConfigUpdate AuditAction = "gate_config_update"
	AuditActionGateEval         AuditAction = "gate_eval"
)

type EntityType string

const (
	EntityTypeReport     EntityType = "report"
	EntityTypeModule     EntityType = "module"
	EntityTypeFile       EntityType = "file"
	EntityTypeGateConfig EntityType = "gate_config"
)

type ActorType string

const (
	ActorTypeUser   ActorType = "user"
	ActorTypeSystem ActorType = "system"
	ActorTypeCI     ActorType = "ci"
)

type CoverageLevel string

const (
	LevelRed    CoverageLevel = "red"
	LevelOrange CoverageLevel = "orange"
	LevelYellow CoverageLevel = "yellow"
	LevelGreen  CoverageLevel = "green"
)

type CoverageReport struct {
	ID               int64      `json:"id" db:"id"`
	ProjectName      string     `json:"project_name" db:"project_name"`
	Branch           string     `json:"branch" db:"branch"`
	CommitHash       string     `json:"commit_hash" db:"commit_hash"`
	BuildID          string     `json:"build_id" db:"build_id"`
	CIJob            string     `json:"ci_job" db:"ci_job"`
	ReportType       ReportType `json:"report_type" db:"report_type"`
	Language         Language   `json:"language" db:"language"`
	LineCoverage     float64    `json:"line_coverage" db:"line_coverage"`
	BranchCoverage   float64    `json:"branch_coverage" db:"branch_coverage"`
	FuncCoverage     float64    `json:"func_coverage" db:"func_coverage"`
	TotalLines       int        `json:"total_lines" db:"total_lines"`
	CoveredLines     int        `json:"covered_lines" db:"covered_lines"`
	TotalBranches    int        `json:"total_branches" db:"total_branches"`
	CoveredBranches  int        `json:"covered_branches" db:"covered_branches"`
	TotalFunctions   int        `json:"total_functions" db:"total_functions"`
	CoveredFunctions int        `json:"covered_functions" db:"covered_functions"`
	RawReportPath    string     `json:"raw_report_path,omitempty" db:"raw_report_path"`
	RawReportSize    int        `json:"raw_report_size" db:"raw_report_size"`
	UploadedBy       int64      `json:"uploaded_by" db:"uploaded_by"`
	IsMerged         bool       `json:"is_merged" db:"is_merged"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
}

type ModuleCoverage struct {
	ID               int64     `json:"id" db:"id"`
	ReportID         int64     `json:"report_id" db:"report_id"`
	ModulePath       string    `json:"module_path" db:"module_path"`
	ModuleName       string    `json:"module_name" db:"module_name"`
	LineCoverage     float64   `json:"line_coverage" db:"line_coverage"`
	BranchCoverage   float64   `json:"branch_coverage" db:"branch_coverage"`
	FuncCoverage     float64   `json:"func_coverage" db:"func_coverage"`
	TotalLines       int       `json:"total_lines" db:"total_lines"`
	CoveredLines     int       `json:"covered_lines" db:"covered_lines"`
	TotalBranches    int       `json:"total_branches" db:"total_branches"`
	CoveredBranches  int       `json:"covered_branches" db:"covered_branches"`
	TotalFunctions   int       `json:"total_functions" db:"total_functions"`
	CoveredFunctions int       `json:"covered_functions" db:"covered_functions"`
	FileCount        int       `json:"file_count" db:"file_count"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

type FileCoverage struct {
	ID               int64     `json:"id" db:"id"`
	ModuleID         int64     `json:"module_id" db:"module_id"`
	ReportID         int64     `json:"report_id" db:"report_id"`
	FilePath         string    `json:"file_path" db:"file_path"`
	FileName         string    `json:"file_name" db:"file_name"`
	LineCoverage     float64   `json:"line_coverage" db:"line_coverage"`
	BranchCoverage   float64   `json:"branch_coverage" db:"branch_coverage"`
	FuncCoverage     float64   `json:"func_coverage" db:"func_coverage"`
	TotalLines       int       `json:"total_lines" db:"total_lines"`
	CoveredLines     int       `json:"covered_lines" db:"covered_lines"`
	TotalBranches    int       `json:"total_branches" db:"total_branches"`
	CoveredBranches  int       `json:"covered_branches" db:"covered_branches"`
	TotalFunctions   int       `json:"total_functions" db:"total_functions"`
	CoveredFunctions int       `json:"covered_functions" db:"covered_functions"`
	UncoveredLines   []int     `json:"uncovered_lines" db:"uncovered_lines"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

type GateConfig struct {
	ID              int64        `json:"id" db:"id"`
	ProjectName     string       `json:"project_name" db:"project_name"`
	ModulePattern   string       `json:"module_pattern" db:"module_pattern"`
	ThresholdRed    float64      `json:"threshold_red" db:"threshold_red"`
	ThresholdOrange float64      `json:"threshold_orange" db:"threshold_orange"`
	ThresholdYellow float64      `json:"threshold_yellow" db:"threshold_yellow"`
	ThresholdGreen  float64      `json:"threshold_green" db:"threshold_green"`
	GateMode        GateMode     `json:"gate_mode" db:"gate_mode"`
	GateThreshold   float64      `json:"gate_threshold" db:"gate_threshold"`
	CoverageType    CoverageType `json:"coverage_type" db:"coverage_type"`
	Description     string       `json:"description" db:"description"`
	CreatedBy       int64        `json:"created_by" db:"created_by"`
	CreatedAt       time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at" db:"updated_at"`
}

type AuditLogEntry struct {
	ID           int64       `json:"id" db:"id"`
	Action       AuditAction `json:"action" db:"action"`
	EntityType   EntityType  `json:"entity_type" db:"entity_type"`
	EntityID     int64       `json:"entity_id" db:"entity_id"`
	ProjectName  string      `json:"project_name" db:"project_name"`
	OldValue     string      `json:"old_value,omitempty" db:"old_value"`
	NewValue     string      `json:"new_value,omitempty" db:"new_value"`
	ActorID      int64       `json:"actor_id" db:"actor_id"`
	ActorType    ActorType   `json:"actor_type" db:"actor_type"`
	ActorName    string      `json:"actor_name" db:"actor_name"`
	IPAddress    string      `json:"ip_address" db:"ip_address"`
	UserAgent    string      `json:"user_agent" db:"user_agent"`
	Success      bool        `json:"success" db:"success"`
	ErrorMessage string      `json:"error_message,omitempty" db:"error_message"`
	CreatedAt    time.Time   `json:"created_at" db:"created_at"`
}

type CoverageLevelResult struct {
	Level           CoverageLevel `json:"level"`
	Value           float64       `json:"value"`
	ThresholdRed    float64       `json:"threshold_red"`
	ThresholdOrange float64       `json:"threshold_orange"`
	ThresholdYellow float64       `json:"threshold_yellow"`
	ThresholdGreen  float64       `json:"threshold_green"`
	GateMode        GateMode      `json:"gate_mode"`
	GatePassed      bool          `json:"gate_passed"`
}
