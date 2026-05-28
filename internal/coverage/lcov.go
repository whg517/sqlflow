package coverage

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	MaxFileSize   = 50 * 1024 * 1024
	MaxLineLength = 65536
	MaxRecords    = 500000
	MaxFiles      = 50000
	MaxPathLength = 1024
	MaxLineNum    = 500000
	MaxDARecords  = 200000
)

type lcovProvider struct{ BaseProvider }

func NewLCOVProvider() CoverageProvider {
	return &lcovProvider{BaseProvider{
		TypeName:  ReportTypeLCOV,
		Languages: []Language{LanguageGo, LanguageJavaScript, LanguageTypeScript},
	}}
}

func (p *lcovProvider) Parse(ctx context.Context, reader io.Reader) (*ParseResult, error) {
	limitedReader := io.LimitReader(reader, MaxFileSize+1)
	scanner := bufio.NewScanner(limitedReader)
	scanner.Buffer(make([]byte, 0, MaxLineLength), MaxLineLength)

	var (
		warnings  []ParseWarning
		files     []ParsedFile
		curFile   *ParsedFile
		recordNum int
		lineNum   int
	)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if len(line) > MaxLineLength {
			warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("line exceeds max length (%d bytes), skipping", len(line))})
			continue
		}
		if recordNum >= MaxRecords {
			warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("exceeded max record count (%d), stopping", MaxRecords)})
			break
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "TN:"):
			// skip
		case strings.HasPrefix(line, "SF:"):
			recordNum++
			filePath := strings.TrimPrefix(line, "SF:")
			if len(filePath) > MaxPathLength {
				warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("file path too long (%d chars)", len(filePath))})
				continue
			}
			if len(files) >= MaxFiles {
				warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("exceeded max file count (%d)", MaxFiles)})
				break
			}
			if curFile != nil {
				finalizeFile(curFile)
				files = append(files, *curFile)
			}
			sanitized := SanitizeFilePath(filePath)
			fileName := sanitized
			if idx := strings.LastIndex(sanitized, "/"); idx >= 0 {
				fileName = sanitized[idx+1:]
			}
			curFile = &ParsedFile{FilePath: sanitized, FileName: fileName, UncoveredLines: make([]int, 0)}
		case strings.HasPrefix(line, "DA:"):
			if curFile == nil {
				warnings = append(warnings, ParseWarning{Line: lineNum, Message: "DA record without preceding SF, skipping"})
				continue
			}
			curFile.TotalLines++
			if curFile.TotalLines > MaxDARecords {
				warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("file %s exceeds max DA records", curFile.FilePath)})
				continue
			}
			data := strings.TrimPrefix(line, "DA:")
			parts := strings.SplitN(data, ",", 3)
			if len(parts) < 2 {
				warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("malformed DA record: %q", data)})
				continue
			}
			ln, err := strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("invalid line number %q", parts[0])})
				continue
			}
			if ln < 0 || ln > MaxLineNum {
				warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("line number %d out of range", ln)})
				continue
			}
			hitCount, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				hitCount = 0
				warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("invalid hit count %q, defaulting to 0", parts[1])})
			}
			if hitCount > 0 {
				curFile.CoveredLines++
			} else {
				curFile.UncoveredLines = append(curFile.UncoveredLines, ln)
			}
		case strings.HasPrefix(line, "BRF:"):
			if curFile == nil { continue }
			if val, err := strconv.Atoi(strings.TrimPrefix(line, "BRF:")); err == nil && val >= 0 { curFile.TotalBranches += val }
		case strings.HasPrefix(line, "BRH:"):
			if curFile == nil { continue }
			if val, err := strconv.Atoi(strings.TrimPrefix(line, "BRH:")); err == nil && val >= 0 { curFile.CoveredBranches += val }
		case strings.HasPrefix(line, "BRDA:"):
			// skip — BRF/BRH sufficient
		case strings.HasPrefix(line, "FN:"):
			if curFile == nil { continue }
			curFile.TotalFunctions++
		case strings.HasPrefix(line, "FNDA:"):
			if curFile == nil { continue }
			p := strings.SplitN(strings.TrimPrefix(line, "FNDA:"), ",", 2)
			if len(p) >= 1 { if hc, err := strconv.Atoi(strings.TrimSpace(p[0])); err == nil && hc > 0 { curFile.CoveredFunctions++ } }
		case strings.HasPrefix(line, "end_of_record"):
			if curFile != nil { finalizeFile(curFile); files = append(files, *curFile); curFile = nil }
		default:
			snippet := line
			if len(snippet) > 50 { snippet = snippet[:50] + "..." }
			warnings = append(warnings, ParseWarning{Line: lineNum, Message: fmt.Sprintf("unknown record type: %q", snippet)})
		}
	}
	if err := scanner.Err(); err != nil { return nil, fmt.Errorf("lcov: read error: %w", err) }
	if curFile != nil { finalizeFile(curFile); files = append(files, *curFile) }
	if len(files) == 0 { return nil, fmt.Errorf("lcov: no coverage data found in input") }
	result := buildResultFromFiles(files)
	result.ReportType = ReportTypeLCOV
	result.Warnings = warnings
	return result, nil
}

func (p *lcovProvider) ParseFile(ctx context.Context, path string) (*ParseResult, error) {
	info, err := os.Stat(path)
	if err != nil { return nil, fmt.Errorf("lcov: stat %s: %w", path, err) }
	if info.Size() > MaxFileSize { return nil, fmt.Errorf("lcov: file size exceeds limit") }
	return ParseFileHelper(ctx, path, func(ctx context.Context, r io.Reader) (*ParseResult, error) { return p.Parse(ctx, r) })
}

func (p *lcovProvider) Detect(data []byte) bool {
	if len(data) == 0 { return false }
	end := len(data); if end > 1024 { end = 1024 }
	for _, line := range strings.Split(string(data[:end]), "\n") {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		return strings.HasPrefix(line, "TN:") || strings.HasPrefix(line, "SF:") || strings.HasPrefix(line, "DA:")
	}
	return false
}

func finalizeFile(f *ParsedFile) {
	if f.TotalLines > 0 { f.LineCoverage = float64(f.CoveredLines) / float64(f.TotalLines) * 100 }
	if f.TotalBranches > 0 { f.BranchCoverage = float64(f.CoveredBranches) / float64(f.TotalBranches) * 100 }
	if f.TotalFunctions > 0 { f.FuncCoverage = float64(f.CoveredFunctions) / float64(f.TotalFunctions) * 100 }
}

func buildResultFromFiles(files []ParsedFile) *ParseResult {
	result := &ParseResult{}
	moduleMap := make(map[string]*ParsedModule)
	for _, f := range files {
		mp := ""
		if idx := strings.LastIndex(f.FilePath, "/"); idx >= 0 { mp = f.FilePath[:idx] }
		mod, ok := moduleMap[mp]
		if !ok {
			mn := mp; if idx := strings.LastIndex(mp, "/"); idx >= 0 { mn = mp[idx+1:] }
			mod = &ParsedModule{ModulePath: mp, ModuleName: mn}
			moduleMap[mp] = mod
		}
		mod.Files = append(mod.Files, f)
		mod.TotalLines += f.TotalLines; mod.CoveredLines += f.CoveredLines
		mod.TotalBranches += f.TotalBranches; mod.CoveredBranches += f.CoveredBranches
		mod.TotalFunctions += f.TotalFunctions; mod.CoveredFunctions += f.CoveredFunctions
		mod.FileCount++
		result.TotalLines += f.TotalLines; result.CoveredLines += f.CoveredLines
		result.TotalBranches += f.TotalBranches; result.CoveredBranches += f.CoveredBranches
		result.TotalFunctions += f.TotalFunctions; result.CoveredFunctions += f.CoveredFunctions
	}
	for _, mod := range moduleMap {
		if mod.TotalLines > 0 { mod.LineCoverage = float64(mod.CoveredLines) / float64(mod.TotalLines) * 100 }
		if mod.TotalBranches > 0 { mod.BranchCoverage = float64(mod.CoveredBranches) / float64(mod.TotalBranches) * 100 }
		if mod.TotalFunctions > 0 { mod.FuncCoverage = float64(mod.CoveredFunctions) / float64(mod.TotalFunctions) * 100 }
		result.Modules = append(result.Modules, *mod)
	}
	if result.TotalLines > 0 { result.LineCoverage = float64(result.CoveredLines) / float64(result.TotalLines) * 100 }
	if result.TotalBranches > 0 { result.BranchCoverage = float64(result.CoveredBranches) / float64(result.TotalBranches) * 100 }
	if result.TotalFunctions > 0 { result.FuncCoverage = float64(result.CoveredFunctions) / float64(result.TotalFunctions) * 100 }
	return result
}
