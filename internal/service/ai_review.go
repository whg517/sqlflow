package service

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

// AI review errors
var (
	ErrAIReviewTimeout     = errors.New("AI 评审超时")
	ErrAIReviewUnavailable = errors.New("AI 评审服务不可用，请稍后重试")
	ErrAIReviewConfig      = errors.New("AI 评审服务未配置")
)

// ReviewDecision indicates what action to take based on the review.
type ReviewDecision string

const (
	DecisionExecute  ReviewDecision = "execute"  // Low risk: execute immediately
	DecisionConfirm  ReviewDecision = "confirm"  // Medium risk: need user confirmation
	DecisionTicket   ReviewDecision = "ticket"   // High risk: must submit ticket
	DecisionBlocked  ReviewDecision = "blocked"  // Blocked by static rules
	DecisionFallback ReviewDecision = "fallback" // Degraded to static rules
)

// AIReviewResult holds the complete AI review result.
type AIReviewResult struct {
	RiskLevel      string         `json:"risk_level"`
	RiskScore      int            `json:"risk_score"`
	Decision       ReviewDecision `json:"decision"`
	Summary        string         `json:"summary"`
	Suggestions    []string       `json:"suggestions"`
	ImpactAnalysis string         `json:"impact_analysis"`
	RollbackSQL    string         `json:"rollback_sql,omitempty"`
	Warnings       []string       `json:"warnings"`
	ReviewSource   string         `json:"review_source"` // "ai", "static", or "degraded"
	ReviewedAt     time.Time      `json:"reviewed_at"`
	ExpiresAt      time.Time      `json:"expires_at"`
	ModelUsed      string         `json:"model_used,omitempty"`
}

// AIReviewRequest is the input for an AI review.
type AIReviewRequest struct {
	SQL          string
	DBType       string // "mysql" or "mongodb"
	DatasourceID int64
	Database     string
	UserID       int64
	Username     string
	Role         string
	Operation    sqlparser.OperationType
	Tables       []string
	ParseResult  *sqlparser.SQLParseResult
}

// SSEEvent represents a Server-Sent Event for streaming reviews.
type SSEEvent struct {
	Type string      // "thinking", "content", "result", "error"
	Data interface{}
}

// aiConfig holds AI service configuration.
type aiConfig struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	Timeout  time.Duration
	Enabled  bool
}

// AIReviewService handles AI review logic.
type AIReviewService struct {
	db     *sql.DB
	config aiConfig
	client *http.Client
	mu     sync.RWMutex
}

// NewAIReviewService creates a new AIReviewService.
func NewAIReviewService(db *sql.DB, provider, model, apiKey, baseURL string, timeout time.Duration) *AIReviewService {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	if provider == "" {
		provider = "openai"
	}
	if model == "" {
		model = "gpt-4"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	enabled := apiKey != ""

	return &AIReviewService{
		db: db,
		config: aiConfig{
			Provider: provider,
			Model:    model,
			APIKey:   apiKey,
			BaseURL:  baseURL,
			Timeout:  timeout,
			Enabled:  enabled,
		},
		client: &http.Client{Timeout: timeout + 2*time.Second},
	}
}

// Review performs AI review on the given SQL.
// It returns the review result, which may come from AI or static rules (degraded).
func (s *AIReviewService) Review(ctx context.Context, req *AIReviewRequest) (*AIReviewResult, error) {
	// 1. Apply static rules first
	staticResult := s.applyStaticRules(ctx, req)
	if staticResult.Decision == DecisionBlocked {
		return staticResult, nil
	}

	// 2. Try AI review if enabled
	if s.config.Enabled {
		aiResult, err := s.callLLM(ctx, req, staticResult)
		if err == nil {
			return aiResult, nil
		}

		// Degradation: check error type
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrAIReviewTimeout) {
			log.Printf("AI review timeout, degrading to static rules for SQL: %s", truncateSQL(req.SQL))
			return s.degradeResult(staticResult, req), nil
		}

		if isNetworkError(err) {
			log.Printf("AI review network error: %v, using static rules", err)
			return s.fallbackResult(staticResult, req), nil
		}

		// Other errors → also fallback
		log.Printf("AI review error: %v, using static rules", err)
		return s.fallbackResult(staticResult, req), nil
	}

	// 3. AI not configured → static rules only
	return s.fallbackResult(staticResult, req), nil
}

// ReviewStream performs AI review with SSE streaming.
func (s *AIReviewService) ReviewStream(ctx context.Context, req *AIReviewRequest) (<-chan SSEEvent, error) {
	// Apply static rules first
	staticResult := s.applyStaticRules(ctx, req)
	if staticResult.Decision == DecisionBlocked {
		ch := make(chan SSEEvent, 1)
		ch <- SSEEvent{Type: "result", Data: staticResult}
		close(ch)
		return ch, nil
	}

	if !s.config.Enabled {
		ch := make(chan SSEEvent, 1)
		result := s.fallbackResult(staticResult, req)
		ch <- SSEEvent{Type: "result", Data: result}
		close(ch)
		return ch, nil
	}

	// Stream from LLM
	return s.streamLLM(ctx, req, staticResult)
}

// ---------------------------------------------------------------------------
// Static rules engine (no AI required)
// ---------------------------------------------------------------------------

// applyStaticRules applies static risk rules without AI.
func (s *AIReviewService) applyStaticRules(ctx context.Context, req *AIReviewRequest) *AIReviewResult {
	result := &AIReviewResult{
		Suggestions:  make([]string, 0),
		Warnings:     make([]string, 0),
		ReviewedAt:   time.Now(),
		ReviewSource: "static",
	}

	pr := req.ParseResult
	if pr == nil {
		result.RiskLevel = AIRiskMedium
		result.Decision = DecisionConfirm
		result.Summary = "无法解析SQL，默认中风险处理"
		return result
	}

	// Blocked by static rules
	if pr.IsBlocked {
		result.RiskLevel = AIRiskHigh
		result.Decision = DecisionBlocked
		result.Summary = pr.BlockReason
		return result
	}

	// Copy warnings from parser
	result.Warnings = append(result.Warnings, pr.Warnings...)

	// Check sensitive tables
	sensitiveTables, err := sqlparser.CheckSensitiveTables(ctx, s.db, req.Tables, int(req.DatasourceID))
	if err != nil {
		log.Printf("check sensitive tables: %v", err)
	}
	isSensitive := len(sensitiveTables) > 0

	// Risk determination based on operation type + sensitivity + conditions
	result.RiskLevel = determineStaticRisk(pr, isSensitive, req.Role)
	result.Suggestions = generateStaticSuggestions(pr, isSensitive)

	// Set decision
	result.Decision = RiskToDecision(result.RiskLevel, pr.Operation)

	// Generate summary
	result.Summary = generateStaticSummary(pr, isSensitive, result.RiskLevel)

	return result
}

// determineStaticRisk determines risk level using static rules.
func determineStaticRisk(pr *sqlparser.SQLParseResult, isSensitive bool, role string) string {
	switch pr.Operation {
	case sqlparser.OpSelect:
		if isSensitive {
			return AIRiskMedium
		}
		return AIRiskLow
	case sqlparser.OpDDL:
		return AIRiskHigh
	case sqlparser.OpUpdate, sqlparser.OpDelete, sqlparser.OpDML:
		if pr.RiskLevel == sqlparser.RiskHigh {
			return AIRiskHigh
		}
		if isSensitive {
			return AIRiskHigh
		}
		return AIRiskMedium
	default:
		return AIRiskMedium
	}
}

// generateStaticSuggestions generates optimization suggestions based on static analysis.
func generateStaticSuggestions(pr *sqlparser.SQLParseResult, isSensitive bool) []string {
	suggestions := make([]string, 0)

	if pr.Operation == sqlparser.OpSelect {
		hasLimitWarning := false
		for _, w := range pr.Warnings {
			if strings.Contains(w, "LIMIT") {
				hasLimitWarning = true
			}
		}
		if hasLimitWarning {
			suggestions = append(suggestions, "建议添加 LIMIT 子句限制返回行数，避免大结果集")
		}
		if len(pr.Tables) > 1 {
			suggestions = append(suggestions, "查询涉及多表 JOIN，建议确认关联条件正确并考虑添加索引")
		}
	}

	if isSensitive {
		suggestions = append(suggestions, "涉及敏感表，查询结果将自动脱敏")
	}

	if pr.Operation == sqlparser.OpUpdate || pr.Operation == sqlparser.OpDelete {
		suggestions = append(suggestions, "变更操作需通过工单审批流程")
	}

	if pr.Operation == sqlparser.OpDDL {
		suggestions = append(suggestions, "DDL 操作可能影响表结构和线上服务，请谨慎操作")
	}

	return suggestions
}

// generateStaticSummary creates a summary for static-only review.
func generateStaticSummary(pr *sqlparser.SQLParseResult, isSensitive bool, riskLevel string) string {
	opName := string(pr.Operation)
	if opName == "" {
		opName = "unknown"
	}

	parts := make([]string, 0, 4)
	parts = append(parts, fmt.Sprintf("操作类型: %s", strings.ToUpper(opName)))

	if len(pr.Tables) > 0 {
		parts = append(parts, fmt.Sprintf("涉及表: %s", strings.Join(pr.Tables, ", ")))
	}

	if isSensitive {
		parts = append(parts, "包含敏感表")
	}

	parts = append(parts, fmt.Sprintf("风险等级: %s", riskLevel))

	return strings.Join(parts, " | ")
}

// RiskToDecision maps risk level and operation type to a decision.
func RiskToDecision(risk string, op sqlparser.OperationType) ReviewDecision {
	switch risk {
	case AIRiskHigh:
		if op == sqlparser.OpSelect {
			return DecisionConfirm
		}
		return DecisionTicket
	case AIRiskMedium:
		if op == sqlparser.OpSelect {
			return DecisionConfirm
		}
		return DecisionTicket
	case AIRiskLow:
		return DecisionExecute
	default:
		return DecisionConfirm
	}
}

// ---------------------------------------------------------------------------
// LLM API client
// ---------------------------------------------------------------------------

// callLLM calls the external LLM API for AI review (non-streaming).
func (s *AIReviewService) callLLM(ctx context.Context, req *AIReviewRequest, staticResult *AIReviewResult) (*AIReviewResult, error) {
	systemPrompt := buildSystemPrompt()
	userPrompt := buildUserPrompt(req, staticResult)

	chatReq := &chatCompletionsRequest{
		Model: s.config.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		Stream:      false,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	llmCtx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	url := strings.TrimRight(s.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(llmCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call LLM API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatCompletionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode LLM response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned empty choices")
	}

	content := chatResp.Choices[0].Message.Content
	aiResult, err := parseAIResponse(content)
	if err != nil {
		return nil, fmt.Errorf("parse AI response: %w", err)
	}

	// Fill metadata
	aiResult.ReviewedAt = time.Now()
	aiResult.ExpiresAt = time.Now().Add(30 * time.Second)
	aiResult.ReviewSource = "ai"
	aiResult.ModelUsed = s.config.Model
	aiResult.Warnings = append(aiResult.Warnings, staticResult.Warnings...)

	if aiResult.Decision == "" {
		aiResult.Decision = RiskToDecision(aiResult.RiskLevel, req.ParseResult.Operation)
	}

	return aiResult, nil
}

// streamLLM calls the LLM API with streaming and returns a channel of events.
func (s *AIReviewService) streamLLM(ctx context.Context, req *AIReviewRequest, staticResult *AIReviewResult) (<-chan SSEEvent, error) {
	systemPrompt := buildSystemPrompt()
	userPrompt := buildUserPrompt(req, staticResult)

	chatReq := &chatCompletionsRequest{
		Model: s.config.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		Stream:      true,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	llmCtx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	url := strings.TrimRight(s.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(llmCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		cancel()
		ch := make(chan SSEEvent, 2)
		if isNetworkError(err) {
			ch <- SSEEvent{Type: "error", Data: "AI服务网络不可达，使用静态规则评审"}
			ch <- SSEEvent{Type: "result", Data: s.fallbackResult(staticResult, req)}
		} else {
			ch <- SSEEvent{Type: "error", Data: "AI评审超时，降级为中风险处理"}
			ch <- SSEEvent{Type: "result", Data: s.degradeResult(staticResult, req)}
		}
		close(ch)
		return ch, nil
	}

	if resp.StatusCode != http.StatusOK {
		cancel()
		resp.Body.Close()
		ch := make(chan SSEEvent, 2)
		ch <- SSEEvent{Type: "error", Data: fmt.Sprintf("AI服务返回错误(%d)，使用静态规则", resp.StatusCode)}
		ch <- SSEEvent{Type: "result", Data: s.fallbackResult(staticResult, req)}
		close(ch)
		return ch, nil
	}

	ch := make(chan SSEEvent, 32)
	go func() {
		defer cancel()
		defer resp.Body.Close()
		defer close(ch)
		s.processSSEStream(resp.Body, ch, req, staticResult)
	}()

	return ch, nil
}

// processSSEStream reads SSE events from the response body and sends parsed events.
func (s *AIReviewService) processSSEStream(body io.Reader, ch chan<- SSEEvent, req *AIReviewRequest, staticResult *AIReviewResult) {
	var fullContent strings.Builder
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			fullContent.WriteString(delta.Content)
			ch <- SSEEvent{Type: "content", Data: delta.Content}
		}
	}

	content := fullContent.String()
	if content == "" {
		ch <- SSEEvent{Type: "error", Data: "AI评审返回空内容，使用静态规则"}
		ch <- SSEEvent{Type: "result", Data: s.fallbackResult(staticResult, req)}
		return
	}

	aiResult, err := parseAIResponse(content)
	if err != nil {
		log.Printf("parse streaming AI response: %v", err)
		ch <- SSEEvent{Type: "error", Data: "AI评审结果解析失败，使用静态规则"}
		ch <- SSEEvent{Type: "result", Data: s.fallbackResult(staticResult, req)}
		return
	}

	aiResult.ReviewedAt = time.Now()
	aiResult.ExpiresAt = time.Now().Add(30 * time.Second)
	aiResult.ReviewSource = "ai"
	aiResult.ModelUsed = s.config.Model
	aiResult.Warnings = append(aiResult.Warnings, staticResult.Warnings...)
	if aiResult.Decision == "" {
		aiResult.Decision = RiskToDecision(aiResult.RiskLevel, req.ParseResult.Operation)
	}

	ch <- SSEEvent{Type: "result", Data: aiResult}
}

// ---------------------------------------------------------------------------
// Prompt engineering
// ---------------------------------------------------------------------------

// buildSystemPrompt creates the system prompt for the AI review.
func buildSystemPrompt() string {
	return `You are an expert SQL review assistant for a database management platform. Your role is to analyze SQL statements and provide risk assessment, optimization suggestions, and impact analysis.

You MUST respond with a valid JSON object (no markdown fences, no extra text) with the following structure:
{
    "risk_level": "low|medium|high",
    "risk_score": <0-100>,
    "summary": "<brief summary of the SQL operation and its risk>",
    "suggestions": ["<suggestion 1>", "<suggestion 2>"],
    "impact_analysis": "<description of potential impact>",
    "rollback_sql": "<rollback SQL if applicable, empty string otherwise>"
}

Risk level rules:
- "low": Simple SELECT queries on non-sensitive tables with LIMIT
- "medium": SELECT on sensitive tables, simple DDL (ADD COLUMN, ADD INDEX), DML with proper WHERE clause
- "high": DDL that changes data types or drops objects, DML without WHERE, operations on core business tables, complex multi-table operations

Guidelines:
1. Always analyze the operation type (SELECT/DDL/DML)
2. Check for missing WHERE clauses in UPDATE/DELETE
3. Evaluate JOIN complexity and subquery nesting
4. Consider potential full table scans
5. For DDL, assess the impact on existing data and applications
6. For DML, estimate affected rows and lock duration
7. Provide specific, actionable optimization suggestions
8. For DDL/DML changes, suggest rollback SQL when possible`
}

// buildUserPrompt creates the user prompt with SQL context.
func buildUserPrompt(req *AIReviewRequest, staticResult *AIReviewResult) string {
	var b strings.Builder

	b.WriteString("Please review the following SQL statement:\n\n")
	b.WriteString(fmt.Sprintf("Database type: %s\n", req.DBType))
	b.WriteString(fmt.Sprintf("Operation type: %s\n", req.Operation))

	if len(req.Tables) > 0 {
		b.WriteString(fmt.Sprintf("Tables involved: %s\n", strings.Join(req.Tables, ", ")))
	}
	if req.Database != "" {
		b.WriteString(fmt.Sprintf("Database: %s\n", req.Database))
	}
	if len(staticResult.Warnings) > 0 {
		b.WriteString(fmt.Sprintf("Static analysis warnings: %s\n", strings.Join(staticResult.Warnings, "; ")))
	}
	if staticResult.RiskLevel != "" {
		b.WriteString(fmt.Sprintf("Static risk assessment: %s\n", staticResult.RiskLevel))
	}

	b.WriteString(fmt.Sprintf("\nSQL:\n%s\n", req.SQL))

	if req.DBType == "mongodb" {
		b.WriteString("\nNote: This is a MongoDB operation. Analyze accordingly.\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------------

// parseAIResponse parses the LLM response content into an AIReviewResult.
func parseAIResponse(content string) (*AIReviewResult, error) {
	jsonStr := extractJSON(content)

	var raw struct {
		RiskLevel      string   `json:"risk_level"`
		RiskScore      int      `json:"risk_score"`
		Summary        string   `json:"summary"`
		Suggestions    []string `json:"suggestions"`
		ImpactAnalysis string   `json:"impact_analysis"`
		RollbackSQL    string   `json:"rollback_sql"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	riskLevel := normalizeRiskLevel(raw.RiskLevel)
	suggestions := raw.Suggestions
	if suggestions == nil {
		suggestions = make([]string, 0)
	}

	return &AIReviewResult{
		RiskLevel:      riskLevel,
		RiskScore:      raw.RiskScore,
		Summary:        raw.Summary,
		Suggestions:    suggestions,
		ImpactAnalysis: raw.ImpactAnalysis,
		RollbackSQL:    raw.RollbackSQL,
	}, nil
}

// extractJSON extracts JSON from text that might contain markdown fences.
func extractJSON(content string) string {
	s := strings.TrimSpace(content)

	// Try to find JSON between ```json ... ```
	if idx := strings.Index(s, "```json"); idx >= 0 {
		s = s[idx+7:]
		if end := strings.Index(s, "```"); end >= 0 {
			s = s[:end]
		}
	} else if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+3:]
		if end := strings.Index(s, "```"); end >= 0 {
			s = s[:end]
		}
	}

	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}

	return s
}

// normalizeRiskLevel normalizes the risk level string.
func normalizeRiskLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "low", "l":
		return AIRiskLow
	case "medium", "med", "m", "moderate":
		return AIRiskMedium
	case "high", "h", "critical":
		return AIRiskHigh
	default:
		return AIRiskMedium
	}
}

// ---------------------------------------------------------------------------
// Degradation strategies
// ---------------------------------------------------------------------------

// degradeResult creates a degraded result when AI times out.
func (s *AIReviewService) degradeResult(staticResult *AIReviewResult, req *AIReviewRequest) *AIReviewResult {
	result := *staticResult
	result.ReviewSource = "degraded"
	result.Summary = "[AI超时降级] " + staticResult.Summary

	// Upgrade risk to at least medium for non-SELECT operations
	if result.RiskLevel == AIRiskLow && req.ParseResult != nil &&
		req.ParseResult.Operation != sqlparser.OpSelect {
		result.RiskLevel = AIRiskMedium
	}

	result.Decision = RiskToDecision(result.RiskLevel, req.ParseResult.Operation)
	result.ReviewedAt = time.Now()
	result.ExpiresAt = time.Now().Add(30 * time.Second)

	return &result
}

// fallbackResult creates a static-only result when AI is unavailable.
func (s *AIReviewService) fallbackResult(staticResult *AIReviewResult, req *AIReviewRequest) *AIReviewResult {
	result := *staticResult
	result.ReviewSource = "static"
	result.ReviewedAt = time.Now()
	result.ExpiresAt = time.Now().Add(30 * time.Second)
	return &result
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

// isNetworkError checks if an error is a network connectivity issue.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "DNS") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "i/o timeout")
}

// UpdateConfig updates the AI service configuration at runtime.
func (s *AIReviewService) UpdateConfig(provider, model, apiKey, baseURL string, timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config.Provider = provider
	s.config.Model = model
	s.config.APIKey = apiKey
	if baseURL != "" {
		s.config.BaseURL = baseURL
	}
	if timeout > 0 {
		s.config.Timeout = timeout
		s.client.Timeout = timeout + 2*time.Second
	}
	s.config.Enabled = apiKey != ""
}

// IsEnabled returns whether AI review is enabled.
func (s *AIReviewService) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.Enabled
}

// GetConfig returns the current AI configuration (with API key masked).
func (s *AIReviewService) GetConfig() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.config.APIKey
	if len(key) > 8 {
		key = key[:4] + "****" + key[len(key)-4:]
	} else if key != "" {
		key = "****"
	}

	return map[string]interface{}{
		"provider": s.config.Provider,
		"model":    s.config.Model,
		"api_key":  key,
		"base_url": s.config.BaseURL,
		"timeout":  s.config.Timeout.String(),
		"enabled":  s.config.Enabled,
	}
}

// ---------------------------------------------------------------------------
// OpenAI-compatible API types
// ---------------------------------------------------------------------------

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type chatCompletionsResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// Risk level constants used across the package.
const (
	AIRiskLow    = "low"
	AIRiskMedium = "medium"
	AIRiskHigh   = "high"
)
