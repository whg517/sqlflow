package coverage

import (
	"fmt"
	"path/filepath"
	"strings"
)

func EvaluateCoverageLevel(value float64, config *GateConfig) CoverageLevelResult {
	result := CoverageLevelResult{
		Value: value, ThresholdRed: config.ThresholdRed, ThresholdOrange: config.ThresholdOrange,
		ThresholdYellow: config.ThresholdYellow, ThresholdGreen: config.ThresholdGreen, GateMode: config.GateMode,
	}
	switch {
	case value >= config.ThresholdGreen:
		result.Level = LevelGreen
	case value >= config.ThresholdYellow:
		result.Level = LevelYellow
	case value >= config.ThresholdOrange:
		result.Level = LevelOrange
	default:
		result.Level = LevelRed
	}
	switch config.GateMode {
	case GateModeOff:
		result.GatePassed = true
	case GateModeWarn, GateModeBlock:
		result.GatePassed = value >= config.GateThreshold
	default:
		result.GatePassed = true
	}
	return result
}

func GetCoverageValue(module *ParsedModule, covType CoverageType) float64 {
	switch covType {
	case CoverageTypeBranch:
		return module.BranchCoverage
	case CoverageTypeFunc:
		return module.FuncCoverage
	default:
		return module.LineCoverage
	}
}

func MatchModulePattern(modulePath, pattern string) (bool, error) {
	if pattern == "" {
		return true, nil
	}
	matched, err := filepath.Match(pattern, modulePath)
	if err != nil {
		return false, fmt.Errorf("coverage: invalid module pattern %q: %w", pattern, err)
	}
	if matched {
		return true, nil
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(modulePath, prefix+"/") || modulePath == prefix, nil
	}
	if strings.HasPrefix(pattern, "**/") {
		suffix := strings.TrimPrefix(pattern, "**/")
		return strings.HasSuffix(modulePath, "/"+suffix) || modulePath == suffix, nil
	}
	return false, nil
}

func FindBestGateConfig(project, modulePath string, configs []GateConfig) (*GateConfig, error) {
	var bestMatch *GateConfig
	bestSpecificity := -1
	for i := range configs {
		cfg := &configs[i]
		if cfg.ProjectName != "" && cfg.ProjectName != project {
			continue
		}
		matched, err := MatchModulePattern(modulePath, cfg.ModulePattern)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		specificity := 0
		if cfg.ProjectName != "" {
			specificity += 2
		}
		if cfg.ModulePattern != "" {
			specificity += 1
		}
		if specificity > bestSpecificity {
			bestMatch = cfg
			bestSpecificity = specificity
		}
	}
	if bestMatch == nil {
		return nil, fmt.Errorf("coverage: no gate config found for project=%q module=%q", project, modulePath)
	}
	return bestMatch, nil
}

func DefaultGateConfig() *GateConfig {
	return &GateConfig{
		ThresholdRed: 50.0, ThresholdOrange: 70.0, ThresholdYellow: 80.0, ThresholdGreen: 90.0,
		GateMode: GateModeOff, GateThreshold: 80.0, CoverageType: CoverageTypeLine,
	}
}

// MUST-4: MergeResults deduplicates files across jobs, merges uncovered_lines by intersection.
func MergeResults(results []*ParseResult) *ParseResult {
	if len(results) == 0 {
		return nil
	}
	if len(results) == 1 {
		return results[0]
	}
	merged := &ParseResult{ReportType: results[0].ReportType, Language: results[0].Language}
	moduleMap := make(map[string]*ParsedModule)
	for _, r := range results {
		merged.Warnings = append(merged.Warnings, r.Warnings...)
		for _, m := range r.Modules {
			if existing, ok := moduleMap[m.ModulePath]; ok {
				mergeModuleFiles(existing, m.Files)
			} else {
				cp := m
				moduleMap[m.ModulePath] = &cp
			}
		}
	}
	for _, m := range moduleMap {
		recomputeModuleFromFiles(m)
		merged.TotalLines += m.TotalLines
		merged.CoveredLines += m.CoveredLines
		merged.TotalBranches += m.TotalBranches
		merged.CoveredBranches += m.CoveredBranches
		merged.TotalFunctions += m.TotalFunctions
		merged.CoveredFunctions += m.CoveredFunctions
	}
	if merged.TotalLines > 0 {
		merged.LineCoverage = float64(merged.CoveredLines) / float64(merged.TotalLines) * 100
	}
	if merged.TotalBranches > 0 {
		merged.BranchCoverage = float64(merged.CoveredBranches) / float64(merged.TotalBranches) * 100
	}
	if merged.TotalFunctions > 0 {
		merged.FuncCoverage = float64(merged.CoveredFunctions) / float64(merged.TotalFunctions) * 100
	}
	for _, m := range moduleMap {
		merged.Modules = append(merged.Modules, *m)
	}
	return merged
}

func mergeModuleFiles(mod *ParsedModule, incoming []ParsedFile) {
	fileIndex := make(map[string]int, len(mod.Files))
	for i, f := range mod.Files {
		fileIndex[f.FilePath] = i
	}
	for _, f := range incoming {
		if idx, ok := fileIndex[f.FilePath]; ok {
			existing := &mod.Files[idx]
			existing.UncoveredLines = intersectIntSlice(existing.UncoveredLines, f.UncoveredLines)
		} else {
			mod.Files = append(mod.Files, f)
			fileIndex[f.FilePath] = len(mod.Files) - 1
		}
	}
}

func intersectIntSlice(a, b []int) []int {
	setA := make(map[int]bool, len(a))
	for _, v := range a {
		setA[v] = true
	}
	result := make([]int, 0)
	for _, v := range b {
		if setA[v] {
			result = append(result, v)
		}
	}
	return result
}

func recomputeModuleFromFiles(m *ParsedModule) {
	m.TotalLines = 0
	m.CoveredLines = 0
	m.TotalBranches = 0
	m.CoveredBranches = 0
	m.TotalFunctions = 0
	m.CoveredFunctions = 0
	m.FileCount = len(m.Files)
	for i := range m.Files {
		f := &m.Files[i]
		if len(f.UncoveredLines) > 0 && f.TotalLines > 0 {
			f.CoveredLines = f.TotalLines - len(f.UncoveredLines)
		}
		if f.TotalLines > 0 {
			f.LineCoverage = float64(f.CoveredLines) / float64(f.TotalLines) * 100
		}
		m.TotalLines += f.TotalLines
		m.CoveredLines += f.CoveredLines
		m.TotalBranches += f.TotalBranches
		m.CoveredBranches += f.CoveredBranches
		m.TotalFunctions += f.TotalFunctions
		m.CoveredFunctions += f.CoveredFunctions
	}
	if m.TotalLines > 0 {
		m.LineCoverage = float64(m.CoveredLines) / float64(m.TotalLines) * 100
	}
	if m.TotalBranches > 0 {
		m.BranchCoverage = float64(m.CoveredBranches) / float64(m.TotalBranches) * 100
	}
	if m.TotalFunctions > 0 {
		m.FuncCoverage = float64(m.CoveredFunctions) / float64(m.TotalFunctions) * 100
	}
}

// NICE-6: includes CI runner environments.
func SanitizeFilePath(filePath string) string {
	type replacement struct {
		prefix     string
		stripParts int
		replace    string
	}
	replacements := []replacement{
		{"/home/", 1, "~/"},
		{"/Users/", 1, "~/"},
		{"/root/", 0, "~/"},
		{"/etc/", 0, "<redacted>/"},
		{"/var/secrets/", 0, "<redacted>/"},
		{"/home/runner/work/", 0, "~/work/"},
		{"/builds/", 1, "~/builds/"},
		{"/workspace/", 0, "~/workspace/"},
		{"/opt/atlassian/", 0, "<redacted>/"},
	}
	result := filePath
	for _, r := range replacements {
		if strings.HasPrefix(result, r.prefix) {
			remaining := result[len(r.prefix):]
			for i := 0; i < r.stripParts; i++ {
				if idx := strings.Index(remaining, "/"); idx >= 0 {
					remaining = remaining[idx+1:]
				} else {
					remaining = ""
				}
			}
			return r.replace + remaining
		}
	}
	return result
}
