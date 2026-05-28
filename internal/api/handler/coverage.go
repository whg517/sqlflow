package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/coverage"
)

type CoverageUploadHandler struct {
	store    *coverage.Store
	registry *coverage.ParserRegistry
}

func NewCoverageUploadHandler(store *coverage.Store, registry *coverage.ParserRegistry) *CoverageUploadHandler {
	return &CoverageUploadHandler{store: store, registry: registry}
}

func (h *CoverageUploadHandler) UploadReport(c echo.Context) error {
	if err := c.Request().ParseMultipartForm(50 << 20); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("无法解析上传文件: %v", err)})
	}
	file, header, err := c.Request().FormFile("file")
	if err != nil { return c.JSON(http.StatusBadRequest, map[string]string{"error": "缺少 file 字段"}) }
	defer file.Close()
	projectName := c.FormValue("project_name")
	if projectName == "" { return c.JSON(http.StatusBadRequest, map[string]string{"error": "缺少 project_name 字段"}) }
	branch := c.FormValue("branch")
	if branch == "" { branch = "main" }
	userID, _ := c.Get("user_id").(int64)
	username, _ := c.Get("username").(string)

	var result *coverage.ParseResult
	reportType := coverage.ReportType(c.FormValue("report_type"))
	if reportType != "" {
		result, err = h.registry.Parse(c.Request().Context(), reportType, file)
	} else {
		result, reportType, err = h.registry.DetectAndParse(c.Request().Context(), file)
	}
	if err != nil { return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": fmt.Sprintf("解析失败: %v", err)}) }

	report := &coverage.CoverageReport{
		ProjectName: projectName, Branch: branch, CommitHash: c.FormValue("commit_hash"),
		BuildID: c.FormValue("build_id"), CIJob: c.FormValue("ci_job"),
		ReportType: reportType, Language: coverage.Language(c.FormValue("language")),
		LineCoverage: result.LineCoverage, BranchCoverage: result.BranchCoverage, FuncCoverage: result.FuncCoverage,
		TotalLines: result.TotalLines, CoveredLines: result.CoveredLines,
		TotalBranches: result.TotalBranches, CoveredBranches: result.CoveredBranches,
		TotalFunctions: result.TotalFunctions, CoveredFunctions: result.CoveredFunctions,
		RawReportSize: int(header.Size), UploadedBy: userID,
	}
	if err := h.store.InsertReportFromParseResult(c.Request().Context(), report, result); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("保存失败: %v", err)})
	}
	_ = h.store.InsertAuditLog(c.Request().Context(), &coverage.AuditLogEntry{
		Action: coverage.AuditActionUpload, EntityType: coverage.EntityTypeReport, EntityID: report.ID,
		ProjectName: projectName, ActorID: userID, ActorType: getActorType(c), ActorName: username,
		IPAddress: c.RealIP(), UserAgent: c.Request().UserAgent(), Success: true,
	})
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id": report.ID, "line_coverage": report.LineCoverage, "branch_coverage": report.BranchCoverage,
		"func_coverage": report.FuncCoverage, "total_lines": report.TotalLines,
		"modules_count": len(result.Modules), "warnings_count": len(result.Warnings),
		"warnings": result.Warnings, "created_at": report.CreatedAt,
	})
}

func (h *CoverageUploadHandler) MergeReports(c echo.Context) error {
	var req struct {
		ReportIDs   []int64 `json:"report_ids"`
		ProjectName string  `json:"project_name"`
		Branch      string  `json:"branch"`
	}
	if err := c.Bind(&req); err != nil { return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的请求体"}) }
	if len(req.ReportIDs) < 2 { return c.JSON(http.StatusBadRequest, map[string]string{"error": "至少需要 2 个报告"}) }
	userID, _ := c.Get("user_id").(int64)
	username, _ := c.Get("username").(string)
	var results []*coverage.ParseResult
	var baseReport *coverage.CoverageReport
	for _, id := range req.ReportIDs {
		rpt, err := h.store.GetReport(c.Request().Context(), id)
		if err != nil { return c.JSON(http.StatusNotFound, map[string]string{"error": fmt.Sprintf("报告 %d 不存在", id)}) }
		if baseReport == nil { baseReport = rpt }
		modules, _ := h.store.ListModules(c.Request().Context(), rpt.ID)
		parsed := &coverage.ParseResult{ReportType: rpt.ReportType, Language: rpt.Language,
			TotalLines: rpt.TotalLines, CoveredLines: rpt.CoveredLines,
			TotalBranches: rpt.TotalBranches, CoveredBranches: rpt.CoveredBranches,
			TotalFunctions: rpt.TotalFunctions, CoveredFunctions: rpt.CoveredFunctions}
		for _, m := range modules {
			parsed.Modules = append(parsed.Modules, coverage.ParsedModule{
				ModulePath: m.ModulePath, ModuleName: m.ModuleName,
				TotalLines: m.TotalLines, CoveredLines: m.CoveredLines,
				TotalBranches: m.TotalBranches, CoveredBranches: m.CoveredBranches,
				TotalFunctions: m.TotalFunctions, CoveredFunctions: m.CoveredFunctions, FileCount: m.FileCount})
		}
		results = append(results, parsed)
	}
	merged := coverage.MergeResults(results)
	mergedReport := &coverage.CoverageReport{
		ProjectName: req.ProjectName, Branch: req.Branch, CommitHash: baseReport.CommitHash,
		BuildID: baseReport.BuildID, ReportType: baseReport.ReportType, Language: baseReport.Language,
		LineCoverage: merged.LineCoverage, BranchCoverage: merged.BranchCoverage, FuncCoverage: merged.FuncCoverage,
		TotalLines: merged.TotalLines, CoveredLines: merged.CoveredLines,
		TotalBranches: merged.TotalBranches, CoveredBranches: merged.CoveredBranches,
		TotalFunctions: merged.TotalFunctions, CoveredFunctions: merged.CoveredFunctions,
		UploadedBy: userID, IsMerged: true}
	if err := h.store.InsertReportFromParseResult(c.Request().Context(), mergedReport, merged); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("保存合并报告失败: %v", err)})
	}
	_ = h.store.InsertAuditLog(c.Request().Context(), &coverage.AuditLogEntry{
		Action: coverage.AuditActionMerge, EntityID: mergedReport.ID, ProjectName: req.ProjectName,
		ActorID: userID, ActorType: getActorType(c), ActorName: username,
		IPAddress: c.RealIP(), UserAgent: c.Request().UserAgent(), Success: true})
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id": mergedReport.ID, "merged_from": req.ReportIDs,
		"line_coverage": mergedReport.LineCoverage, "total_lines": mergedReport.TotalLines})
}

type CoverageQueryHandler struct{ store *coverage.Store }

func NewCoverageQueryHandler(store *coverage.Store) *CoverageQueryHandler {
	return &CoverageQueryHandler{store: store}
}

func (h *CoverageQueryHandler) GetProjectCoverage(c echo.Context) error {
	report, err := h.store.ProjectCoverage(c.Request().Context(), c.Param("project"), c.QueryParam("branch"))
	if err != nil { return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()}) }
	if report == nil { return c.JSON(http.StatusNotFound, map[string]string{"error": "未找到覆盖率数据"}) }
	modules, _ := h.store.ListModules(c.Request().Context(), report.ID)
	return c.JSON(http.StatusOK, map[string]interface{}{"report": report, "modules": modules})
}

func (h *CoverageQueryHandler) GetModuleFiles(c echo.Context) error {
	moduleID, err := strconv.ParseInt(c.Param("moduleID"), 10, 64)
	if err != nil { return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的模块 ID"}) }
	files, err := h.store.ListFiles(c.Request().Context(), moduleID)
	if err != nil { return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()}) }
	role, _ := c.Get("role").(string)
	if role != "admin" && role != "tech_lead" {
		for i := range files { files[i].UncoveredLines = nil }
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"files": files})
}

func (h *CoverageQueryHandler) GetCoverageHistory(c echo.Context) error {
	from := time.Now().AddDate(0, -1, 0); to := time.Now()
	if v := c.QueryParam("from"); v != "" { if t, err := time.Parse(time.RFC3339, v); err == nil { from = t } }
	if v := c.QueryParam("to"); v != "" { if t, err := time.Parse(time.RFC3339, v); err == nil { to = t } }
	reports, err := h.store.CoverageHistory(c.Request().Context(), c.Param("project"), c.QueryParam("branch"), from, to)
	if err != nil { return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()}) }
	return c.JSON(http.StatusOK, map[string]interface{}{"reports": reports, "from": from, "to": to})
}

func (h *CoverageQueryHandler) ListReports(c echo.Context) error {
	project := c.QueryParam("project")
	if project == "" { return c.JSON(http.StatusBadRequest, map[string]string{"error": "缺少 project 参数"}) }
	limit, offset := 20, 0
	if v, err := strconv.Atoi(c.QueryParam("limit")); err == nil && v > 0 { limit = v }
	if v, err := strconv.Atoi(c.QueryParam("offset")); err == nil && v >= 0 { offset = v }
	reports, err := h.store.ListReports(c.Request().Context(), project, c.QueryParam("branch"), limit, offset)
	if err != nil { return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()}) }
	return c.JSON(http.StatusOK, map[string]interface{}{"reports": reports, "limit": limit, "offset": offset})
}

type CoverageGateHandler struct{ store *coverage.Store }

func NewCoverageGateHandler(store *coverage.Store) *CoverageGateHandler {
	return &CoverageGateHandler{store: store}
}

func (h *CoverageGateHandler) ListGateConfigs(c echo.Context) error {
	configs, err := h.store.ListGateConfigs(c.Request().Context(), c.QueryParam("project"))
	if err != nil { return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()}) }
	return c.JSON(http.StatusOK, map[string]interface{}{"configs": configs})
}

func (h *CoverageGateHandler) CreateGateConfig(c echo.Context) error {
	var cfg coverage.GateConfig
	if err := c.Bind(&cfg); err != nil { return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的请求体"}) }
	userID, _ := c.Get("user_id").(int64); cfg.CreatedBy = userID
	if err := h.store.UpsertGateConfig(c.Request().Context(), &cfg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, cfg)
}

func (h *CoverageGateHandler) UpdateGateConfig(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	existing, err := h.store.GetGateConfig(c.Request().Context(), id)
	if err != nil { return c.JSON(http.StatusNotFound, map[string]string{"error": "配置不存在"}) }
	var cfg coverage.GateConfig
	if err := c.Bind(&cfg); err != nil { return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的请求体"}) }
	cfg.ID = id; cfg.ProjectName = existing.ProjectName; cfg.ModulePattern = existing.ModulePattern
	cfg.CoverageType = existing.CoverageType; cfg.CreatedBy = existing.CreatedBy; cfg.CreatedAt = existing.CreatedAt
	if err := h.store.UpsertGateConfig(c.Request().Context(), &cfg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, cfg)
}

func (h *CoverageGateHandler) DeleteGateConfig(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.store.DeleteGateConfig(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "已删除"})
}

func getActorType(c echo.Context) coverage.ActorType {
	role, _ := c.Get("role").(string)
	if role == "api_token" { return coverage.ActorTypeCI }
	return coverage.ActorTypeUser
}

func SanitizePathForAPI(role, path string) string {
	if role == "admin" || role == "tech_lead" { return path }
	if strings.Contains(path, "auth/") || strings.Contains(path, "jwt/") ||
		strings.Contains(path, "crypto/") || strings.Contains(path, "secret") {
		parts := strings.Split(path, "/")
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return path
}

// MUST-1: nil pgDB guard — returns without registering routes if pgDB is nil.
func RegisterCoverageRoutes(e *echo.Echo, authMW, adminMW echo.MiddlewareFunc, pgDB *sql.DB) {
	if pgDB == nil { return }
	store := coverage.NewStore(pgDB)
	registry := coverage.GlobalRegistry()
	uh := NewCoverageUploadHandler(store, registry)
	qh := NewCoverageQueryHandler(store)
	gh := NewCoverageGateHandler(store)

	g := e.Group("", authMW)
	g.POST("/api/v1/coverage/reports", uh.UploadReport)
	g.POST("/api/v1/coverage/reports/merge", uh.MergeReports)
	g.GET("/api/v1/coverage/reports", qh.ListReports)
	g.GET("/api/v1/coverage/projects/:project", qh.GetProjectCoverage)
	g.GET("/api/v1/coverage/projects/:project/history", qh.GetCoverageHistory)
	g.GET("/api/v1/coverage/reports/:id/modules/:moduleID/files", qh.GetModuleFiles)

	ag := e.Group("", authMW, adminMW)
	ag.GET("/api/v1/coverage/gate-configs", gh.ListGateConfigs)
	ag.POST("/api/v1/coverage/gate-configs", gh.CreateGateConfig)
	ag.PUT("/api/v1/coverage/gate-configs/:id", gh.UpdateGateConfig)
	ag.DELETE("/api/v1/coverage/gate-configs/:id", gh.DeleteGateConfig)
}
