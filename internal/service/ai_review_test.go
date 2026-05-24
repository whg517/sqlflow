package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	// Create mask_rules table for sensitive table checks
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS mask_rules (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			datasource_id   INTEGER NOT NULL DEFAULT 0,
			database        TEXT    NOT NULL DEFAULT '',
			table_name      TEXT    NOT NULL DEFAULT '',
			field           TEXT    NOT NULL DEFAULT '',
			mask_type       TEXT    NOT NULL DEFAULT '',
			custom_regex    TEXT    NOT NULL DEFAULT '',
			custom_template TEXT    NOT NULL DEFAULT '',
			created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		t.Fatalf("create mask_rules table: %v", err)
	}
	return db
}

func newTestService(t *testing.T, db *sql.DB) *AIReviewService {
	t.Helper()
	return NewAIReviewService(db, "openai", "test-model", "", "https://api.example.com/v1", 5*time.Second)
}

func newTestServiceWithAI(t *testing.T, db *sql.DB, handler http.HandlerFunc) *AIReviewService {
	t.Helper()
	svc := NewAIReviewService(db, "openai", "test-model", "test-api-key", "https://api.example.com/v1", 5*time.Second)
	if handler != nil {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		svc.config.BaseURL = server.URL
		svc.client = server.Client()
		svc.client.Timeout = 5 * time.Second
	}
	return svc
}

func makeTestRequest(sql string, op sqlparser.OperationType) *AIReviewRequest {
	return &AIReviewRequest{
		SQL:       sql,
		DBType:    "mysql",
		Operation: op,
		Tables:    []string{"users"},
		ParseResult: &sqlparser.SQLParseResult{
			DBType:    "mysql",
			Operation: op,
			Tables:    []string{"users"},
			RiskLevel: sqlparser.RiskLow,
			Warnings:  make([]string, 0),
		},
	}
}

// ---------------------------------------------------------------------------
// Static rules tests
// ---------------------------------------------------------------------------

func TestApplyStaticRules_Blocked(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db)

	tests := []struct {
		name     string
		request  *AIReviewRequest
		want     ReviewDecision
		wantRisk string
	}{
		{
			name: "blocked by static rules",
			request: &AIReviewRequest{
				SQL:       "DROP TABLE users",
				DBType:    "mysql",
				Operation: sqlparser.OpDDL,
				Tables:    []string{"users"},
				ParseResult: &sqlparser.SQLParseResult{
					DBType:      "mysql",
					Operation:   sqlparser.OpDDL,
					Tables:      []string{"users"},
					IsBlocked:   true,
					BlockReason: "DROP TABLE is not allowed",
					RiskLevel:   sqlparser.RiskHigh,
				},
			},
			want:     DecisionBlocked,
			wantRisk: AIRiskHigh,
		},
		{
			name: "SELECT low risk",
			request: &AIReviewRequest{
				SQL:       "SELECT * FROM users LIMIT 10",
				DBType:    "mysql",
				Operation: sqlparser.OpSelect,
				Tables:    []string{"users"},
				ParseResult: &sqlparser.SQLParseResult{
					DBType:    "mysql",
					Operation: sqlparser.OpSelect,
					Tables:    []string{"users"},
					RiskLevel: sqlparser.RiskLow,
					Warnings:  make([]string, 0),
				},
			},
			want:     DecisionExecute,
			wantRisk: AIRiskLow,
		},
		{
			name: "UPDATE medium risk with WHERE",
			request: &AIReviewRequest{
				SQL:       "UPDATE users SET name = 'test' WHERE id = 1",
				DBType:    "mysql",
				Operation: sqlparser.OpUpdate,
				Tables:    []string{"users"},
				ParseResult: &sqlparser.SQLParseResult{
					DBType:    "mysql",
					Operation: sqlparser.OpUpdate,
					Tables:    []string{"users"},
					RiskLevel: sqlparser.RiskMedium,
					Warnings:  make([]string, 0),
				},
			},
			want:     DecisionTicket,
			wantRisk: AIRiskMedium,
		},
		{
			name: "DDL high risk",
			request: &AIReviewRequest{
				SQL:       "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
				DBType:    "mysql",
				Operation: sqlparser.OpDDL,
				Tables:    []string{"users"},
				ParseResult: &sqlparser.SQLParseResult{
					DBType:    "mysql",
					Operation: sqlparser.OpDDL,
					Tables:    []string{"users"},
					RiskLevel: sqlparser.RiskMedium,
					Warnings:  make([]string, 0),
				},
			},
			want:     DecisionTicket,
			wantRisk: AIRiskHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.applyStaticRules(context.Background(), tt.request)
			if result.Decision != tt.want {
				t.Errorf("decision = %v, want %v", result.Decision, tt.want)
			}
			if result.RiskLevel != tt.wantRisk {
				t.Errorf("risk = %v, want %v", result.RiskLevel, tt.wantRisk)
			}
		})
	}
}

func TestApplyStaticRules_NilParseResult(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db)

	req := &AIReviewRequest{
		SQL:         "SELECT 1",
		DBType:      "mysql",
		ParseResult: nil,
	}

	result := svc.applyStaticRules(context.Background(), req)
	if result.Decision != DecisionConfirm {
		t.Errorf("decision = %v, want %v", result.Decision, DecisionConfirm)
	}
	if result.RiskLevel != AIRiskMedium {
		t.Errorf("risk = %v, want %v", result.RiskLevel, AIRiskMedium)
	}
}

func TestApplyStaticRules_SensitiveTable(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Insert a mask rule marking "users" as sensitive
	_, err := db.Exec(`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type) VALUES (1, '', 'users', 'phone', 'phone')`)
	if err != nil {
		t.Fatalf("insert mask rule: %v", err)
	}

	svc := newTestService(t, db)

	req := &AIReviewRequest{
		SQL:          "SELECT * FROM users LIMIT 10",
		DBType:       "mysql",
		DatasourceID: 1,
		Operation:    sqlparser.OpSelect,
		Tables:       []string{"users"},
		ParseResult: &sqlparser.SQLParseResult{
			DBType:    "mysql",
			Operation: sqlparser.OpSelect,
			Tables:    []string{"users"},
			RiskLevel: sqlparser.RiskLow,
			Warnings:  make([]string, 0),
		},
	}

	result := svc.applyStaticRules(context.Background(), req)
	if result.RiskLevel != AIRiskMedium {
		t.Errorf("risk = %v, want %v (sensitive table should be medium)", result.RiskLevel, AIRiskMedium)
	}
	if result.Decision != DecisionConfirm {
		t.Errorf("decision = %v, want %v", result.Decision, DecisionConfirm)
	}

	// Check that sensitive table suggestion is present
	found := false
	for _, s := range result.Suggestions {
		if strings.Contains(s, "敏感表") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected sensitive table suggestion")
	}
}

// ---------------------------------------------------------------------------
// Risk determination tests
// ---------------------------------------------------------------------------

func TestDetermineStaticRisk(t *testing.T) {
	tests := []struct {
		name        string
		operation   sqlparser.OperationType
		parserRisk  sqlparser.RiskLevel
		isSensitive bool
		wantRisk    string
	}{
		{
			name:        "SELECT non-sensitive = low",
			operation:   sqlparser.OpSelect,
			isSensitive: false,
			wantRisk:    AIRiskLow,
		},
		{
			name:        "SELECT sensitive = medium",
			operation:   sqlparser.OpSelect,
			isSensitive: true,
			wantRisk:    AIRiskMedium,
		},
		{
			name:        "DDL = high",
			operation:   sqlparser.OpDDL,
			isSensitive: false,
			wantRisk:    AIRiskHigh,
		},
		{
			name:        "UPDATE non-sensitive = medium",
			operation:   sqlparser.OpUpdate,
			parserRisk:  sqlparser.RiskMedium,
			isSensitive: false,
			wantRisk:    AIRiskMedium,
		},
		{
			name:        "UPDATE sensitive = high",
			operation:   sqlparser.OpUpdate,
			parserRisk:  sqlparser.RiskMedium,
			isSensitive: true,
			wantRisk:    AIRiskHigh,
		},
		{
			name:        "UPDATE no WHERE = high",
			operation:   sqlparser.OpUpdate,
			parserRisk:  sqlparser.RiskHigh,
			isSensitive: false,
			wantRisk:    AIRiskHigh,
		},
		{
			name:        "DELETE non-sensitive = medium",
			operation:   sqlparser.OpDelete,
			parserRisk:  sqlparser.RiskMedium,
			isSensitive: false,
			wantRisk:    AIRiskMedium,
		},
		{
			name:        "DML = medium",
			operation:   sqlparser.OpDML,
			parserRisk:  sqlparser.RiskMedium,
			isSensitive: false,
			wantRisk:    AIRiskMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &sqlparser.SQLParseResult{
				Operation: tt.operation,
				RiskLevel: tt.parserRisk,
			}
			got := determineStaticRisk(pr, tt.isSensitive, "developer")
			if got != tt.wantRisk {
				t.Errorf("risk = %v, want %v", got, tt.wantRisk)
			}
		})
	}
}

func TestRiskToDecision(t *testing.T) {
	tests := []struct {
		name     string
		risk     string
		op       sqlparser.OperationType
		expected ReviewDecision
	}{
		{"low risk SELECT", AIRiskLow, sqlparser.OpSelect, DecisionExecute},
		{"medium risk SELECT", AIRiskMedium, sqlparser.OpSelect, DecisionConfirm},
		{"high risk SELECT", AIRiskHigh, sqlparser.OpSelect, DecisionConfirm},
		{"low risk UPDATE", AIRiskLow, sqlparser.OpUpdate, DecisionExecute},
		{"medium risk UPDATE", AIRiskMedium, sqlparser.OpUpdate, DecisionTicket},
		{"high risk UPDATE", AIRiskHigh, sqlparser.OpUpdate, DecisionTicket},
		{"medium risk DDL", AIRiskMedium, sqlparser.OpDDL, DecisionTicket},
		{"high risk DDL", AIRiskHigh, sqlparser.OpDDL, DecisionTicket},
		{"unknown risk", "unknown", sqlparser.OpSelect, DecisionConfirm},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RiskToDecision(tt.risk, tt.op)
			if got != tt.expected {
				t.Errorf("decision = %v, want %v", got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Prompt engineering tests
// ---------------------------------------------------------------------------

func TestBuildSystemPrompt(t *testing.T) {
	prompt := buildSystemPrompt()
	if prompt == "" {
		t.Fatal("system prompt is empty")
	}
	if !strings.Contains(prompt, "risk_level") {
		t.Error("system prompt missing risk_level field description")
	}
	if !strings.Contains(prompt, "low") || !strings.Contains(prompt, "medium") || !strings.Contains(prompt, "high") {
		t.Error("system prompt missing risk level descriptions")
	}
	if !strings.Contains(prompt, "suggestions") {
		t.Error("system prompt missing suggestions field")
	}
	if !strings.Contains(prompt, "impact_analysis") {
		t.Error("system prompt missing impact_analysis field")
	}
	if !strings.Contains(prompt, "rollback_sql") {
		t.Error("system prompt missing rollback_sql field")
	}
}

func TestBuildUserPrompt(t *testing.T) {
	req := &AIReviewRequest{
		SQL:       "SELECT * FROM users LIMIT 10",
		DBType:    "mysql",
		Operation: sqlparser.OpSelect,
		Tables:    []string{"users"},
		Database:  "testdb",
	}
	staticResult := &AIReviewResult{
		RiskLevel: AIRiskLow,
		Warnings:  []string{"query without LIMIT may return large result set"},
	}

	prompt := buildUserPrompt(req, staticResult)
	if prompt == "" {
		t.Fatal("user prompt is empty")
	}
	if !strings.Contains(prompt, "mysql") {
		t.Error("user prompt missing database type")
	}
	if !strings.Contains(prompt, "SELECT * FROM users LIMIT 10") {
		t.Error("user prompt missing SQL content")
	}
	if !strings.Contains(prompt, "users") {
		t.Error("user prompt missing table name")
	}
	if !strings.Contains(prompt, "testdb") {
		t.Error("user prompt missing database name")
	}
	if !strings.Contains(prompt, "LIMIT") {
		t.Error("user prompt missing warnings")
	}
}

func TestBuildUserPrompt_MongoDB(t *testing.T) {
	req := &AIReviewRequest{
		SQL:       `{"operation": "find", "collection": "orders", "filter": {"status": "active"}}`,
		DBType:    "mongodb",
		Operation: sqlparser.OpSelect,
		Tables:    []string{"orders"},
	}
	staticResult := &AIReviewResult{RiskLevel: AIRiskLow}

	prompt := buildUserPrompt(req, staticResult)
	if !strings.Contains(prompt, "MongoDB") {
		t.Error("user prompt missing MongoDB note")
	}
}

// ---------------------------------------------------------------------------
// Response parsing tests
// ---------------------------------------------------------------------------

func TestParseAIResponse_ValidJSON(t *testing.T) {
	content := `{
		"risk_level": "medium",
		"risk_score": 45,
		"summary": "SELECT on sensitive table with potential full scan",
		"suggestions": ["Add LIMIT clause", "Consider adding index on status column"],
		"impact_analysis": "May return large result set from sensitive table",
		"rollback_sql": ""
	}`

	result, err := parseAIResponse(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.RiskLevel != AIRiskMedium {
		t.Errorf("risk = %v, want %v", result.RiskLevel, AIRiskMedium)
	}
	if result.RiskScore != 45 {
		t.Errorf("score = %v, want 45", result.RiskScore)
	}
	if len(result.Suggestions) != 2 {
		t.Errorf("suggestions count = %v, want 2", len(result.Suggestions))
	}
	if result.Summary == "" {
		t.Error("summary is empty")
	}
}

func TestParseAIResponse_MarkdownFenced(t *testing.T) {
	content := "Here is the analysis:\n\n```json\n{\"risk_level\": \"low\", \"risk_score\": 10, \"summary\": \"test\", \"suggestions\": [], \"impact_analysis\": \"none\", \"rollback_sql\": \"\"}\n```\n"

	result, err := parseAIResponse(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.RiskLevel != AIRiskLow {
		t.Errorf("risk = %v, want %v", result.RiskLevel, AIRiskLow)
	}
}

func TestParseAIResponse_InvalidJSON(t *testing.T) {
	content := "This is not JSON at all"
	_, err := parseAIResponse(content)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestNormalizeRiskLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"low", AIRiskLow},
		{"Low", AIRiskLow},
		{"LOW", AIRiskLow},
		{"l", AIRiskLow},
		{"medium", AIRiskMedium},
		{"Medium", AIRiskMedium},
		{"med", AIRiskMedium},
		{"m", AIRiskMedium},
		{"moderate", AIRiskMedium},
		{"high", AIRiskHigh},
		{"High", AIRiskHigh},
		{"h", AIRiskHigh},
		{"critical", AIRiskHigh},
		{"unknown", AIRiskMedium},
		{"", AIRiskMedium},
		{"  low  ", AIRiskLow},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeRiskLevel(tt.input)
			if got != tt.want {
				t.Errorf("normalizeRiskLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "plain JSON",
			content: `{"key": "value"}`,
			want:    `{"key": "value"}`,
		},
		{
			name:    "JSON with markdown fence",
			content: "```json\n{\"key\": \"value\"}\n```",
			want:    `{"key": "value"}`,
		},
		{
			name:    "JSON with plain fence",
			content: "```\n{\"key\": \"value\"}\n```",
			want:    `{"key": "value"}`,
		},
		{
			name:    "JSON with surrounding text",
			content: "Here is the result:\n{\"key\": \"value\"}\nEnd.",
			want:    `{"key": "value"}`,
		},
		{
			name:    "nested JSON",
			content: `{"outer": {"inner": 1}, "arr": [1, 2]}`,
			want:    `{"outer": {"inner": 1}, "arr": [1, 2]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.content)
			if got != tt.want {
				t.Errorf("extractJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Suggestion generation tests
// ---------------------------------------------------------------------------

func TestGenerateStaticSuggestions(t *testing.T) {
	tests := []struct {
		name        string
		operation   sqlparser.OperationType
		warnings    []string
		tables      []string
		isSensitive bool
		wantLen     int // minimum expected suggestions
	}{
		{
			name:      "SELECT with LIMIT warning",
			operation: sqlparser.OpSelect,
			warnings:  []string{"query without LIMIT may return large result set"},
			tables:    []string{"users"},
			wantLen:   1, // LIMIT suggestion
		},
		{
			name:      "SELECT with multi-table JOIN",
			operation: sqlparser.OpSelect,
			warnings:  []string{},
			tables:    []string{"users", "orders"},
			wantLen:   1, // JOIN suggestion
		},
		{
			name:        "SELECT sensitive table",
			operation:   sqlparser.OpSelect,
			warnings:    []string{},
			tables:      []string{"users"},
			isSensitive: true,
			wantLen:     1, // sensitive table suggestion
		},
		{
			name:      "UPDATE operation",
			operation: sqlparser.OpUpdate,
			warnings:  []string{},
			tables:    []string{"users"},
			wantLen:   1, // ticket suggestion
		},
		{
			name:      "DDL operation",
			operation: sqlparser.OpDDL,
			warnings:  []string{},
			tables:    []string{"users"},
			wantLen:   1, // DDL suggestion
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &sqlparser.SQLParseResult{
				Operation: tt.operation,
				Tables:    tt.tables,
				Warnings:  tt.warnings,
			}
			suggestions := generateStaticSuggestions(pr, tt.isSensitive)
			if len(suggestions) < tt.wantLen {
				t.Errorf("got %d suggestions, want at least %d: %v", len(suggestions), tt.wantLen, suggestions)
			}
		})
	}
}

func TestGenerateStaticSummary(t *testing.T) {
	pr := &sqlparser.SQLParseResult{
		Operation: sqlparser.OpSelect,
		Tables:    []string{"users", "orders"},
	}

	summary := generateStaticSummary(pr, true, AIRiskMedium)
	if !strings.Contains(summary, "SELECT") {
		t.Error("summary missing operation type")
	}
	if !strings.Contains(summary, "users") {
		t.Error("summary missing table name")
	}
	if !strings.Contains(summary, "敏感") {
		t.Error("summary missing sensitive table indicator")
	}
	if !strings.Contains(summary, "medium") {
		t.Error("summary missing risk level")
	}
}

// ---------------------------------------------------------------------------
// Degradation tests
// ---------------------------------------------------------------------------

func TestDegradeResult(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db)

	staticResult := &AIReviewResult{
		RiskLevel:    AIRiskLow,
		Decision:     DecisionExecute,
		Summary:      "操作类型: SELECT | 涉及表: users | 风险等级: low",
		Suggestions:  make([]string, 0),
		ReviewSource: "static",
	}

	req := makeTestRequest("SELECT * FROM users LIMIT 10", sqlparser.OpSelect)

	result := svc.degradeResult(staticResult, req)
	if result.ReviewSource != "degraded" {
		t.Errorf("source = %v, want degraded", result.ReviewSource)
	}
	if !strings.Contains(result.Summary, "AI超时降级") {
		t.Errorf("summary should contain degradation indicator: %s", result.Summary)
	}
	if result.ExpiresAt.IsZero() {
		t.Error("expires_at should be set")
	}
	if result.ReviewedAt.IsZero() {
		t.Error("reviewed_at should be set")
	}
}

func TestDegradeResult_UpgradeNonSelect(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db)

	staticResult := &AIReviewResult{
		RiskLevel:    AIRiskLow, // low risk but UPDATE
		Decision:     DecisionExecute,
		Summary:      "操作类型: UPDATE | 涉及表: users | 风险等级: low",
		Suggestions:  make([]string, 0),
		ReviewSource: "static",
	}

	req := makeTestRequest("UPDATE users SET name = 'test' WHERE id = 1", sqlparser.OpUpdate)

	result := svc.degradeResult(staticResult, req)
	// Non-SELECT should be upgraded to at least medium
	if result.RiskLevel == AIRiskLow {
		t.Error("non-SELECT degradation should upgrade risk from low")
	}
}

func TestFallbackResult(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db)

	staticResult := &AIReviewResult{
		RiskLevel:    AIRiskMedium,
		Decision:     DecisionConfirm,
		Summary:      "test summary",
		Suggestions:  make([]string, 0),
		ReviewSource: "static",
	}

	req := makeTestRequest("SELECT * FROM users", sqlparser.OpSelect)

	result := svc.fallbackResult(staticResult, req)
	if result.ReviewSource != "static" {
		t.Errorf("source = %v, want static", result.ReviewSource)
	}
	if result.ExpiresAt.IsZero() {
		t.Error("expires_at should be set")
	}
}

// ---------------------------------------------------------------------------
// Full review flow tests (without AI - static only)
// ---------------------------------------------------------------------------

func TestReview_StaticOnly_NoAPIKey(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db) // no API key

	req := makeTestRequest("SELECT * FROM users LIMIT 10", sqlparser.OpSelect)
	result, err := svc.Review(context.Background(), req)
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}
	if result.ReviewSource != "static" {
		t.Errorf("source = %v, want static", result.ReviewSource)
	}
	if result.Decision != DecisionExecute {
		t.Errorf("decision = %v, want %v", result.Decision, DecisionExecute)
	}
}

func TestReview_StaticOnly_Blocked(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db)

	req := &AIReviewRequest{
		SQL:       "DROP TABLE users",
		DBType:    "mysql",
		Operation: sqlparser.OpDDL,
		ParseResult: &sqlparser.SQLParseResult{
			DBType:      "mysql",
			Operation:   sqlparser.OpDDL,
			Tables:      []string{"users"},
			IsBlocked:   true,
			BlockReason: "DROP TABLE is not allowed",
			RiskLevel:   sqlparser.RiskHigh,
		},
	}

	result, err := svc.Review(context.Background(), req)
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}
	if result.Decision != DecisionBlocked {
		t.Errorf("decision = %v, want %v", result.Decision, DecisionBlocked)
	}
}

// ---------------------------------------------------------------------------
// LLM client integration tests (with mock server)
// ---------------------------------------------------------------------------

func TestReview_WithMockLLM(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	aiResponse := chatCompletionsResponse{
		ID: "test-id",
		Choices: []struct {
			Message chatMessage `json:"message"`
		}{
			{Message: chatMessage{Role: "assistant", Content: `{"risk_level": "low", "risk_score": 15, "summary": "Simple SELECT query", "suggestions": ["Consider adding LIMIT"], "impact_analysis": "Minimal impact", "rollback_sql": ""}`}},
		},
		Model: "test-model",
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("missing Bearer token in Authorization header")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(aiResponse)
	}

	svc := newTestServiceWithAI(t, db, handler)
	req := makeTestRequest("SELECT * FROM users LIMIT 10", sqlparser.OpSelect)

	result, err := svc.Review(context.Background(), req)
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}
	if result.ReviewSource != "ai" {
		t.Errorf("source = %v, want ai", result.ReviewSource)
	}
	if result.ModelUsed != "test-model" {
		t.Errorf("model = %v, want test-model", result.ModelUsed)
	}
	if result.RiskLevel != AIRiskLow {
		t.Errorf("risk = %v, want %v", result.RiskLevel, AIRiskLow)
	}
}

func TestReview_WithMockLLM_HighRisk(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	aiResponse := chatCompletionsResponse{
		ID: "test-id",
		Choices: []struct {
			Message chatMessage `json:"message"`
		}{
			{Message: chatMessage{Role: "assistant", Content: `{"risk_level": "high", "risk_score": 85, "summary": "DDL operation on core table", "suggestions": ["Backup table before ALTER", "Test in staging first"], "impact_analysis": "Adding column may cause table lock", "rollback_sql": "ALTER TABLE users DROP COLUMN phone"}`}},
		},
		Model: "test-model",
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(aiResponse)
	}

	svc := newTestServiceWithAI(t, db, handler)
	req := makeTestRequest("ALTER TABLE users ADD COLUMN phone VARCHAR(20)", sqlparser.OpDDL)

	result, err := svc.Review(context.Background(), req)
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}
	if result.RiskLevel != AIRiskHigh {
		t.Errorf("risk = %v, want %v", result.RiskLevel, AIRiskHigh)
	}
	if result.Decision != DecisionTicket {
		t.Errorf("decision = %v, want %v", result.Decision, DecisionTicket)
	}
	if len(result.Suggestions) != 2 {
		t.Errorf("suggestions count = %v, want 2", len(result.Suggestions))
	}
	if result.RollbackSQL == "" {
		t.Error("expected rollback SQL for DDL")
	}
}

func TestReview_WithMockLLM_ErrorResponse(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error": "internal server error"}`)
	}

	svc := newTestServiceWithAI(t, db, handler)
	req := makeTestRequest("SELECT * FROM users", sqlparser.OpSelect)

	result, err := svc.Review(context.Background(), req)
	if err != nil {
		t.Fatalf("review should not fail, got: %v", err)
	}
	// Should fallback to static rules
	if result.ReviewSource != "static" {
		t.Errorf("source = %v, want static (fallback)", result.ReviewSource)
	}
}

func TestReview_WithMockLLM_InvalidResponse(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices": []}`)
	}

	svc := newTestServiceWithAI(t, db, handler)
	req := makeTestRequest("SELECT * FROM users", sqlparser.OpSelect)

	result, err := svc.Review(context.Background(), req)
	if err != nil {
		t.Fatalf("review should not fail, got: %v", err)
	}
	// Should fallback to static rules
	if result.ReviewSource != "static" {
		t.Errorf("source = %v, want static (fallback)", result.ReviewSource)
	}
}

func TestReview_WithMockLLM_NonJSONContent(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := chatCompletionsResponse{
			ID: "test-id",
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "I cannot analyze this SQL"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}

	svc := newTestServiceWithAI(t, db, handler)
	req := makeTestRequest("SELECT * FROM users", sqlparser.OpSelect)

	result, err := svc.Review(context.Background(), req)
	if err != nil {
		t.Fatalf("review should not fail, got: %v", err)
	}
	if result.ReviewSource != "static" {
		t.Errorf("source = %v, want static (fallback)", result.ReviewSource)
	}
}

// ---------------------------------------------------------------------------
// Streaming tests
// ---------------------------------------------------------------------------

func TestReviewStream_StaticOnly(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db) // no API key

	req := makeTestRequest("SELECT * FROM users LIMIT 10", sqlparser.OpSelect)
	ch, err := svc.ReviewStream(context.Background(), req)
	if err != nil {
		t.Fatalf("review stream failed: %v", err)
	}

	var results []SSEEvent
	for event := range ch {
		results = append(results, event)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 event, got %d", len(results))
	}
	if results[0].Type != "result" {
		t.Errorf("event type = %v, want result", results[0].Type)
	}
}

func TestReviewStream_Blocked(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db)

	req := &AIReviewRequest{
		SQL:       "DROP TABLE users",
		DBType:    "mysql",
		Operation: sqlparser.OpDDL,
		ParseResult: &sqlparser.SQLParseResult{
			DBType:      "mysql",
			Operation:   sqlparser.OpDDL,
			Tables:      []string{"users"},
			IsBlocked:   true,
			BlockReason: "DROP TABLE is not allowed",
			RiskLevel:   sqlparser.RiskHigh,
		},
	}

	ch, err := svc.ReviewStream(context.Background(), req)
	if err != nil {
		t.Fatalf("review stream failed: %v", err)
	}

	var results []SSEEvent
	for event := range ch {
		results = append(results, event)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 event, got %d", len(results))
	}
	result := results[0].Data.(*AIReviewResult)
	if result.Decision != DecisionBlocked {
		t.Errorf("decision = %v, want %v", result.Decision, DecisionBlocked)
	}
}

func TestReviewStream_WithMockLLM(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}

		// Send streaming chunks
		chunks := []string{
			`{"choices":[{"delta":{"content":"{\"risk_"}}]}`,
			`{"choices":[{"delta":{"content":"level\":\"low\",\"risk"}}]}`,
			`{"choices":[{"delta":{"content":"_score\":10,\"summary\":\"test\",\"suggestions\":[],\"impact_analysis\":\"none\",\"rollback_sql\":\"\"}"}}]}`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}

	svc := newTestServiceWithAI(t, db, handler)
	req := makeTestRequest("SELECT * FROM users LIMIT 10", sqlparser.OpSelect)

	ch, err := svc.ReviewStream(context.Background(), req)
	if err != nil {
		t.Fatalf("review stream failed: %v", err)
	}

	var contentEvents int
	var resultReceived bool
	for event := range ch {
		switch event.Type {
		case "content":
			contentEvents++
		case "result":
			resultReceived = true
			result := event.Data.(*AIReviewResult)
			if result.ReviewSource != "ai" {
				t.Errorf("source = %v, want ai", result.ReviewSource)
			}
			if result.RiskLevel != AIRiskLow {
				t.Errorf("risk = %v, want %v", result.RiskLevel, AIRiskLow)
			}
		}
	}

	if contentEvents == 0 {
		t.Error("expected content events from streaming")
	}
	if !resultReceived {
		t.Error("expected result event")
	}
}

// ---------------------------------------------------------------------------
// Config management tests
// ---------------------------------------------------------------------------

func TestUpdateConfig(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := newTestService(t, db)

	if svc.IsEnabled() {
		t.Error("service should not be enabled without API key")
	}

	svc.UpdateConfig("anthropic", "claude-3", "sk-new-long-api-key", "https://api.anthropic.com/v1", 15*time.Second)

	if !svc.IsEnabled() {
		t.Error("service should be enabled after setting API key")
	}

	cfg := svc.GetConfig()
	if cfg["provider"] != "anthropic" {
		t.Errorf("provider = %v, want anthropic", cfg["provider"])
	}
	if cfg["model"] != "claude-3" {
		t.Errorf("model = %v, want claude-3", cfg["model"])
	}
	if cfg["api_key"] != "sk-n****-key" {
		t.Errorf("api_key should be masked, got %v", cfg["api_key"])
	}
}

func TestGetConfig_MaskedAPIKey(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := NewAIReviewService(db, "openai", "gpt-4", "sk-test-long-api-key-12345678", "", 10*time.Second)

	cfg := svc.GetConfig()
	key, ok := cfg["api_key"].(string)
	if !ok {
		t.Fatal("api_key is not a string")
	}
	if key != "sk-t****5678" {
		t.Errorf("masked key = %v, want sk-t****5678", key)
	}
}

func TestGetConfig_ShortAPIKey(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	svc := NewAIReviewService(db, "openai", "gpt-4", "short", "", 10*time.Second)

	cfg := svc.GetConfig()
	key := cfg["api_key"].(string)
	if key != "****" {
		t.Errorf("short key should be fully masked, got %v", key)
	}
}

func TestNewAIReviewService_Defaults(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	svc := NewAIReviewService(db, "", "", "", "", 0)

	if svc.config.Provider != "openai" {
		t.Errorf("default provider = %v, want openai", svc.config.Provider)
	}
	if svc.config.Model != "gpt-4" {
		t.Errorf("default model = %v, want gpt-4", svc.config.Model)
	}
	if svc.config.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("default base URL = %v", svc.config.BaseURL)
	}
	if svc.config.Timeout != 10*time.Second {
		t.Errorf("default timeout = %v, want 10s", svc.config.Timeout)
	}
	if svc.config.Enabled {
		t.Error("should not be enabled without API key")
	}
}

// ---------------------------------------------------------------------------
// isNetworkError tests
// ---------------------------------------------------------------------------

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		errMsg string
		want   bool
	}{
		{"connection refused", true},
		{"dial tcp: no such host", true},
		{"DNS lookup failed", true},
		{"network is unreachable", true},
		{"i/o timeout", true},
		{"some other error", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			got := isNetworkError(err)
			if got != tt.want {
				t.Errorf("isNetworkError(%q) = %v, want %v", tt.errMsg, got, tt.want)
			}
		})
	}
}
