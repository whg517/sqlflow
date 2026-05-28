package coverage

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }
func (s *Store) DB() *sql.DB     { return s.db }

func (s *Store) InsertReport(ctx context.Context, report *CoverageReport, modules []ModuleCoverage, files []FileCoverage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("coverage store: begin tx: %w", err) }
	defer tx.Rollback()
	reportID, err := insertReportRow(ctx, tx, report)
	if err != nil { return err }
	report.ID = reportID
	moduleIDMap := make(map[string]int64)
	for i := range modules {
		modules[i].ReportID = reportID
		modID, err := insertModuleRow(ctx, tx, &modules[i])
		if err != nil { return err }
		modules[i].ID = modID
		moduleIDMap[modules[i].ModulePath] = modID
	}
	for i := range files {
		files[i].ReportID = reportID
		if files[i].ModuleID == 0 {
			if modID, ok := moduleIDMap[files[i].FilePath]; ok { files[i].ModuleID = modID }
		}
		if err := insertFileRow(ctx, tx, &files[i]); err != nil { return err }
	}
	return tx.Commit()
}

func (s *Store) InsertReportFromParseResult(ctx context.Context, report *CoverageReport, parsed *ParseResult) error {
	var modules []ModuleCoverage
	var files []FileCoverage
	for _, pm := range parsed.Modules {
		mod := ModuleCoverage{ModulePath: pm.ModulePath, ModuleName: pm.ModuleName,
			LineCoverage: pm.LineCoverage, BranchCoverage: pm.BranchCoverage, FuncCoverage: pm.FuncCoverage,
			TotalLines: pm.TotalLines, CoveredLines: pm.CoveredLines,
			TotalBranches: pm.TotalBranches, CoveredBranches: pm.CoveredBranches,
			TotalFunctions: pm.TotalFunctions, CoveredFunctions: pm.CoveredFunctions, FileCount: pm.FileCount}
		for _, pf := range pm.Files {
			files = append(files, FileCoverage{FilePath: pf.FilePath, FileName: pf.FileName,
				LineCoverage: pf.LineCoverage, BranchCoverage: pf.BranchCoverage, FuncCoverage: pf.FuncCoverage,
				TotalLines: pf.TotalLines, CoveredLines: pf.CoveredLines,
				TotalBranches: pf.TotalBranches, CoveredBranches: pf.CoveredBranches,
				TotalFunctions: pf.TotalFunctions, CoveredFunctions: pf.CoveredFunctions, UncoveredLines: pf.UncoveredLines})
		}
		modules = append(modules, mod)
	}
	return s.InsertReport(ctx, report, modules, files)
}

func (s *Store) GetReport(ctx context.Context, id int64) (*CoverageReport, error) {
	var r CoverageReport
	err := s.db.QueryRowContext(ctx, `SELECT id, project_name, branch, commit_hash, build_id, ci_job,
		report_type, language, line_coverage, branch_coverage, func_coverage,
		total_lines, covered_lines, total_branches, covered_branches,
		total_functions, covered_functions, raw_report_path, raw_report_size,
		uploaded_by, is_merged, created_at FROM coverage_reports WHERE id = $1`, id).Scan(
		&r.ID, &r.ProjectName, &r.Branch, &r.CommitHash, &r.BuildID, &r.CIJob,
		&r.ReportType, &r.Language, &r.LineCoverage, &r.BranchCoverage, &r.FuncCoverage,
		&r.TotalLines, &r.CoveredLines, &r.TotalBranches, &r.CoveredBranches,
		&r.TotalFunctions, &r.CoveredFunctions, &r.RawReportPath, &r.RawReportSize,
		&r.UploadedBy, &r.IsMerged, &r.CreatedAt)
	if err == sql.ErrNoRows { return nil, fmt.Errorf("coverage store: report %d not found", id) }
	if err != nil { return nil, fmt.Errorf("coverage store: get report: %w", err) }
	return &r, nil
}

func (s *Store) ListReports(ctx context.Context, project, branch string, limit, offset int) ([]CoverageReport, error) {
	if limit <= 0 { limit = 20 }; if limit > 100 { limit = 100 }
	q := `SELECT id, project_name, branch, commit_hash, build_id, ci_job, report_type, language,
		line_coverage, branch_coverage, func_coverage, total_lines, covered_lines, total_branches, covered_branches,
		total_functions, covered_functions, raw_report_path, raw_report_size, uploaded_by, is_merged, created_at
		FROM coverage_reports WHERE project_name = $1`
	args := []interface{}{project}; n := 2
	if branch != "" { q += fmt.Sprintf(" AND branch = $%d", n); args = append(args, branch); n++ }
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var reports []CoverageReport
	for rows.Next() {
		var r CoverageReport
		if err := rows.Scan(&r.ID, &r.ProjectName, &r.Branch, &r.CommitHash, &r.BuildID, &r.CIJob,
			&r.ReportType, &r.Language, &r.LineCoverage, &r.BranchCoverage, &r.FuncCoverage,
			&r.TotalLines, &r.CoveredLines, &r.TotalBranches, &r.CoveredBranches,
			&r.TotalFunctions, &r.CoveredFunctions, &r.RawReportPath, &r.RawReportSize,
			&r.UploadedBy, &r.IsMerged, &r.CreatedAt); err != nil { return nil, err }
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func (s *Store) ListModules(ctx context.Context, reportID int64) ([]ModuleCoverage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, report_id, module_path, module_name,
		line_coverage, branch_coverage, func_coverage, total_lines, covered_lines,
		total_branches, covered_branches, total_functions, covered_functions, file_count, created_at
		FROM coverage_modules WHERE report_id = $1 ORDER BY module_path`, reportID)
	if err != nil { return nil, err }
	defer rows.Close()
	var modules []ModuleCoverage
	for rows.Next() {
		var m ModuleCoverage
		if err := rows.Scan(&m.ID, &m.ReportID, &m.ModulePath, &m.ModuleName,
			&m.LineCoverage, &m.BranchCoverage, &m.FuncCoverage, &m.TotalLines, &m.CoveredLines,
			&m.TotalBranches, &m.CoveredBranches, &m.TotalFunctions, &m.CoveredFunctions,
			&m.FileCount, &m.CreatedAt); err != nil { return nil, err }
		modules = append(modules, m)
	}
	return modules, rows.Err()
}

// MUST-3: ListFiles includes uncovered_lines.
func (s *Store) ListFiles(ctx context.Context, moduleID int64) ([]FileCoverage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, module_id, report_id, file_path, file_name,
		line_coverage, branch_coverage, func_coverage, total_lines, covered_lines,
		total_branches, covered_branches, total_functions, covered_functions,
		array_to_string(uncovered_lines, ',') AS ul, created_at
		FROM coverage_files WHERE module_id = $1 ORDER BY file_path`, moduleID)
	if err != nil { return nil, err }
	defer rows.Close()
	var files []FileCoverage
	for rows.Next() {
		var f FileCoverage; var ul string
		if err := rows.Scan(&f.ID, &f.ModuleID, &f.ReportID, &f.FilePath, &f.FileName,
			&f.LineCoverage, &f.BranchCoverage, &f.FuncCoverage, &f.TotalLines, &f.CoveredLines,
			&f.TotalBranches, &f.CoveredBranches, &f.TotalFunctions, &f.CoveredFunctions, &ul, &f.CreatedAt); err != nil { return nil, err }
		f.UncoveredLines = parseIntArray(ul)
		files = append(files, f)
	}
	return files, rows.Err()
}

func (s *Store) GetGateConfig(ctx context.Context, id int64) (*GateConfig, error) {
	var g GateConfig
	err := s.db.QueryRowContext(ctx, `SELECT id, project_name, module_pattern, threshold_red, threshold_orange,
		threshold_yellow, threshold_green, gate_mode, gate_threshold, coverage_type,
		description, created_by, created_at, updated_at FROM coverage_gate_configs WHERE id = $1`, id).Scan(
		&g.ID, &g.ProjectName, &g.ModulePattern, &g.ThresholdRed, &g.ThresholdOrange,
		&g.ThresholdYellow, &g.ThresholdGreen, &g.GateMode, &g.GateThreshold, &g.CoverageType,
		&g.Description, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
	if err == sql.ErrNoRows { return nil, fmt.Errorf("coverage store: gate config %d not found", id) }
	if err != nil { return nil, err }
	return &g, nil
}

func (s *Store) ListGateConfigs(ctx context.Context, project string) ([]GateConfig, error) {
	q := `SELECT id, project_name, module_pattern, threshold_red, threshold_orange, threshold_yellow,
		threshold_green, gate_mode, gate_threshold, coverage_type, description, created_by, created_at, updated_at
		FROM coverage_gate_configs`
	var args []interface{}
	if project != "" { q += " WHERE project_name = $1 OR project_name = ''"; args = append(args, project) }
	q += " ORDER BY project_name, module_pattern"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var configs []GateConfig
	for rows.Next() {
		var g GateConfig
		if err := rows.Scan(&g.ID, &g.ProjectName, &g.ModulePattern, &g.ThresholdRed, &g.ThresholdOrange,
			&g.ThresholdYellow, &g.ThresholdGreen, &g.GateMode, &g.GateThreshold, &g.CoverageType,
			&g.Description, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt); err != nil { return nil, err }
		configs = append(configs, g)
	}
	return configs, rows.Err()
}

func (s *Store) UpsertGateConfig(ctx context.Context, g *GateConfig) error {
	now := time.Now(); g.UpdatedAt = now
	return s.db.QueryRowContext(ctx, `INSERT INTO coverage_gate_configs (
		project_name, module_pattern, threshold_red, threshold_orange, threshold_yellow, threshold_green,
		gate_mode, gate_threshold, coverage_type, description, created_by, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	ON CONFLICT (project_name, module_pattern, coverage_type) DO UPDATE SET
		threshold_red=EXCLUDED.threshold_red, threshold_orange=EXCLUDED.threshold_orange,
		threshold_yellow=EXCLUDED.threshold_yellow, threshold_green=EXCLUDED.threshold_green,
		gate_mode=EXCLUDED.gate_mode, gate_threshold=EXCLUDED.gate_threshold,
		description=EXCLUDED.description, updated_at=EXCLUDED.updated_at
	RETURNING id, created_at`, g.ProjectName, g.ModulePattern, g.ThresholdRed, g.ThresholdOrange,
		g.ThresholdYellow, g.ThresholdGreen, g.GateMode, g.GateThreshold, g.CoverageType,
		g.Description, g.CreatedBy, now, now).Scan(&g.ID, &g.CreatedAt)
}

func (s *Store) DeleteGateConfig(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM coverage_gate_configs WHERE id = $1`, id)
	if err != nil { return err }
	n, _ := res.RowsAffected()
	if n == 0 { return fmt.Errorf("coverage store: gate config %d not found", id) }
	return nil
}

func (s *Store) InsertAuditLog(ctx context.Context, entry *AuditLogEntry) error {
	return s.db.QueryRowContext(ctx, `INSERT INTO coverage_audit_log (
		action, entity_type, entity_id, project_name, old_value, new_value,
		actor_id, actor_type, actor_name, ip_address, user_agent, success, error_message
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id, created_at`,
		entry.Action, entry.EntityType, entry.EntityID, entry.ProjectName, entry.OldValue, entry.NewValue,
		entry.ActorID, entry.ActorType, entry.ActorName, entry.IPAddress, entry.UserAgent,
		entry.Success, entry.ErrorMessage).Scan(&entry.ID, &entry.CreatedAt)
}

// NICE-7: both rawDays and fileDays are parameters (fileDays available for future per-file cleanup).
func (s *Store) PurgeRawData(ctx context.Context, rawDays, fileDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -rawDays)
	res1, err := s.db.ExecContext(ctx, `UPDATE coverage_files SET uncovered_lines = '{}'
		WHERE report_id IN (SELECT id FROM coverage_reports WHERE created_at < $1) AND uncovered_lines <> '{}'`, cutoff)
	if err != nil { return 0, err }
	n1, _ := res1.RowsAffected()
	res2, err := s.db.ExecContext(ctx, `UPDATE coverage_reports SET raw_report_path = '', raw_report_size = 0
		WHERE created_at < $1 AND raw_report_path <> ''`, cutoff)
	if err != nil { return 0, err }
	n2, _ := res2.RowsAffected()
	return n1 + n2, nil
}

func (s *Store) ProjectCoverage(ctx context.Context, project, branch string) (*CoverageReport, error) {
	q := `SELECT id, project_name, branch, commit_hash, build_id, ci_job, report_type, language,
		line_coverage, branch_coverage, func_coverage, total_lines, covered_lines,
		total_branches, covered_branches, total_functions, covered_functions,
		raw_report_path, raw_report_size, uploaded_by, is_merged, created_at
		FROM coverage_reports WHERE project_name = $1`
	args := []interface{}{project}; n := 2
	if branch != "" { q += fmt.Sprintf(" AND branch = $%d", n); args = append(args, branch) }
	q += " ORDER BY created_at DESC LIMIT 1"
	var r CoverageReport
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&r.ID, &r.ProjectName, &r.Branch, &r.CommitHash, &r.BuildID, &r.CIJob,
		&r.ReportType, &r.Language, &r.LineCoverage, &r.BranchCoverage, &r.FuncCoverage,
		&r.TotalLines, &r.CoveredLines, &r.TotalBranches, &r.CoveredBranches,
		&r.TotalFunctions, &r.CoveredFunctions, &r.RawReportPath, &r.RawReportSize,
		&r.UploadedBy, &r.IsMerged, &r.CreatedAt)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return nil, err }
	return &r, nil
}

func (s *Store) CoverageHistory(ctx context.Context, project, branch string, from, to time.Time) ([]CoverageReport, error) {
	q := `SELECT id, project_name, branch, commit_hash, build_id, ci_job, report_type, language,
		line_coverage, branch_coverage, func_coverage, total_lines, covered_lines,
		total_branches, covered_branches, total_functions, covered_functions,
		raw_report_path, raw_report_size, uploaded_by, is_merged, created_at
		FROM coverage_reports WHERE project_name = $1 AND created_at >= $2 AND created_at <= $3`
	args := []interface{}{project, from, to}
	if branch != "" { q += " AND branch = $4"; args = append(args, branch) }
	q += " ORDER BY created_at ASC"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var reports []CoverageReport
	for rows.Next() {
		var r CoverageReport
		if err := rows.Scan(&r.ID, &r.ProjectName, &r.Branch, &r.CommitHash, &r.BuildID, &r.CIJob,
			&r.ReportType, &r.Language, &r.LineCoverage, &r.BranchCoverage, &r.FuncCoverage,
			&r.TotalLines, &r.CoveredLines, &r.TotalBranches, &r.CoveredBranches,
			&r.TotalFunctions, &r.CoveredFunctions, &r.RawReportPath, &r.RawReportSize,
			&r.UploadedBy, &r.IsMerged, &r.CreatedAt); err != nil { return nil, err }
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func insertReportRow(ctx context.Context, tx *sql.Tx, r *CoverageReport) (int64, error) {
	err := tx.QueryRowContext(ctx, `INSERT INTO coverage_reports (
		project_name, branch, commit_hash, build_id, ci_job, report_type, language,
		line_coverage, branch_coverage, func_coverage, total_lines, covered_lines,
		total_branches, covered_branches, total_functions, covered_functions,
		raw_report_path, raw_report_size, uploaded_by, is_merged
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) RETURNING id`,
		r.ProjectName, r.Branch, r.CommitHash, r.BuildID, r.CIJob, r.ReportType, r.Language,
		r.LineCoverage, r.BranchCoverage, r.FuncCoverage, r.TotalLines, r.CoveredLines,
		r.TotalBranches, r.CoveredBranches, r.TotalFunctions, r.CoveredFunctions,
		r.RawReportPath, r.RawReportSize, r.UploadedBy, r.IsMerged).Scan(&r.ID)
	if err != nil { return 0, err }
	r.CreatedAt = time.Now()
	return r.ID, nil
}

func insertModuleRow(ctx context.Context, tx *sql.Tx, m *ModuleCoverage) (int64, error) {
	err := tx.QueryRowContext(ctx, `INSERT INTO coverage_modules (
		report_id, module_path, module_name, line_coverage, branch_coverage, func_coverage,
		total_lines, covered_lines, total_branches, covered_branches,
		total_functions, covered_functions, file_count
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		m.ReportID, m.ModulePath, m.ModuleName, m.LineCoverage, m.BranchCoverage, m.FuncCoverage,
		m.TotalLines, m.CoveredLines, m.TotalBranches, m.CoveredBranches,
		m.TotalFunctions, m.CoveredFunctions, m.FileCount).Scan(&m.ID)
	if err != nil { return 0, err }
	m.CreatedAt = time.Now()
	return m.ID, nil
}

// MUST-2: fully parameterized with string_to_array.
func insertFileRow(ctx context.Context, tx *sql.Tx, f *FileCoverage) error {
	linesStr := ""
	if len(f.UncoveredLines) > 0 {
		parts := make([]string, len(f.UncoveredLines))
		for i, ln := range f.UncoveredLines { parts[i] = strconv.Itoa(ln) }
		linesStr = strings.Join(parts, ",")
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO coverage_files (
		module_id, report_id, file_path, file_name, line_coverage, branch_coverage, func_coverage,
		total_lines, covered_lines, total_branches, covered_branches,
		total_functions, covered_functions, uncovered_lines
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
		CASE WHEN $14 = '' THEN '{}'::integer[] ELSE string_to_array($14, ',')::integer[] END
	) RETURNING id`, f.ModuleID, f.ReportID, f.FilePath, f.FileName,
		f.LineCoverage, f.BranchCoverage, f.FuncCoverage,
		f.TotalLines, f.CoveredLines, f.TotalBranches, f.CoveredBranches,
		f.TotalFunctions, f.CoveredFunctions, linesStr).Scan(&f.ID)
	if err != nil { return fmt.Errorf("coverage store: insert file: %w", err) }
	f.CreatedAt = time.Now()
	return nil
}

func parseIntArray(s string) []int {
	if s == "" { return nil }
	parts := strings.Split(s, ",")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" { continue }
		if v, err := strconv.Atoi(p); err == nil { result = append(result, v) }
	}
	if len(result) == 0 { return nil }
	return result
}
