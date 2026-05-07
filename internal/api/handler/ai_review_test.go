package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/service"
)

// ---------------------------------------------------------------------------
// Test setup helpers
// ---------------------------------------------------------------------------

// setupAIReviewTest creates a fresh DB, services, and AIReviewHandler for testing.
func setupAIReviewTest(t *testing.T) (*echo.Echo, *service.AIReviewService, *service.DatasourceService, *AIReviewHandler, *db.DB) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef"
	connMgr := connpool.NewManager()
	dsSvc := service.NewDatasourceService(database.DB, encKey, connMgr)
	aiReviewSvc := service.NewAIReviewService(database.DB, "openai", "test-model", "", "https://api.example.com/v1", 5*time.Second)
	handler := NewAIReviewHandler(aiReviewSvc, dsSvc)

	e := echo.New()
	return e, aiReviewSvc, dsSvc, handler, database
}

// setupAIReviewTestWithMockLLM sets up handler with AI enabled and a mock LLM server.
func setupAIReviewTestWithMockLLM(t *testing.T, handler http.HandlerFunc) (*echo.Echo, *service.AIReviewService, *service.DatasourceService, *AIReviewHandler, *db.DB) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef"
	connMgr := connpool.NewManager()
	dsSvc := service.NewDatasourceService(database.DB, encKey, connMgr)

	// Create AI service with API key and mock server
	aiReviewSvc := service.NewAIReviewService(database.DB, "openai", "test-model", "test-api-key", "https://api.example.com/v1", 5*time.Second)
	if handler != nil {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		aiReviewSvc.UpdateConfig("openai", "test-model", "test-api-key", server.URL, 5*time.Second)
	}

	h := NewAIReviewHandler(aiReviewSvc, dsSvc)

	e := echo.New()
	return e, aiReviewSvc, dsSvc, h, database
}

// seedAIReviewUser inserts a test user and returns the ID.
func seedAIReviewUser(t *testing.T, database *db.DB, username, role string) int64 {
	t.Helper()
	ctx := aiReviewContextWithTimeout(t)
	res, err := database.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES (?, 'testhash', ?)`,
		username, role,
	)
	if err != nil {
		t.Fatalf("seed user %q: %v", username, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedAIReviewDatasource creates a test datasource and returns it.
func seedAIReviewDatasource(t *testing.T, dsSvc *service.DatasourceService, name, dsType string) *model.DataSource {
	t.Helper()
	ctx := aiReviewContextWithTimeout(t)
	ds := &model.DataSource{
		Name:     name,
		Type:     dsType,
		Host:     "10.0.0.1",
		Port:     3306,
		Username: "root",
		Database: "testdb",
	}
	if err := dsSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("seed datasource %q: %v", name, err)
	}
	return ds
}

// setAIReviewAuth sets user identity on the echo context (simulates JWT middleware).
func setAIReviewAuth(c echo.Context, userID int64, username, role string) {
	c.Set(middleware.ContextKeyUserID, userID)
	c.Set(middleware.ContextKeyUsername, username)
	c.Set(middleware.ContextKeyRole, role)
}

// aiReviewContextWithTimeout returns a context with 5 second timeout.
func aiReviewContextWithTimeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// parseSSEEvents parses the SSE response body into structured events.
func parseSSEEvents(t *testing.T, body string) []struct {
	Event string
	Data  string
} {
	t.Helper()
	var events []struct {
		Event string
		Data  string
	}

	scanner := bufio.NewScanner(strings.NewReader(body))
	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			events = append(events, struct {
				Event string
				Data  string
			}{Event: currentEvent, Data: data})
			currentEvent = ""
		}
	}
	return events
}

// closeableRecorder wraps httptest.ResponseRecorder to implement io.Closer,
// which is required by AIReviewHandler.ReviewStream for SSE connection closing.
type closeableRecorder struct {
	*httptest.ResponseRecorder
}

func (c *closeableRecorder) Close() error { return nil }

// newSSERequest creates a POST request with auth context set up for AI review SSE testing.
func newSSERequest(e *echo.Echo, userID int64, body string) (echo.Context, *closeableRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/api/query/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := &closeableRecorder{httptest.NewRecorder()}
	c := e.NewContext(req, rec)
	setAIReviewAuth(c, userID, "testuser", "developer")
	return c, rec
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_Validation(t *testing.T) {
	e, _, dsSvc, h, database := setupAIReviewTest(t)
	userID := seedAIReviewUser(t, database, "testuser", "developer")

	// Seed a datasource for valid tests
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds", "mysql")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "missing_datasource_id",
			body:       `{"sql":"SELECT 1","database":"testdb"}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "数据源ID不能为空",
		},
		{
			name:       "zero_datasource_id",
			body:       `{"datasource_id":0,"sql":"SELECT 1","database":"testdb"}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "数据源ID不能为空",
		},
		{
			name:       "empty_sql",
			body:       fmt.Sprintf(`{"datasource_id":%d,"sql":"","database":"testdb"}`, ds.ID),
			wantStatus: http.StatusBadRequest,
			wantMsg:    "SQL不能为空",
		},
		{
			name:       "missing_sql",
			body:       fmt.Sprintf(`{"datasource_id":%d,"database":"testdb"}`, ds.ID),
			wantStatus: http.StatusBadRequest,
			wantMsg:    "SQL不能为空",
		},
		{
			name:       "invalid_json",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "请求格式错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/query/review", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setAIReviewAuth(c, userID, "testuser", "developer")

			err := h.ReviewStream(c)
			if err == nil {
				t.Fatalf("expected error, got nil; status=%d body=%s", rec.Code, rec.Body.String())
			}
			he, ok := err.(*echo.HTTPError)
			if !ok {
				t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
			}
			if he.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; message=%v", he.Code, tt.wantStatus, he.Message)
			}
			if !strings.Contains(fmt.Sprintf("%v", he.Message), tt.wantMsg) {
				t.Errorf("message = %v, want to contain %q", he.Message, tt.wantMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Static-only review (no AI key) — should return SSE with static result
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_StaticOnly(t *testing.T) {
	e, _, dsSvc, h, database := setupAIReviewTest(t)

	userID := seedAIReviewUser(t, database, "testuser", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds", "mysql")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"SELECT * FROM users LIMIT 10"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)

	err := h.ReviewStream(c)
	if err != nil {
		t.Fatalf("ReviewStream returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify SSE headers
	ct := rec.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	cc := rec.Header().Get("Cache-Control")
	if cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	xab := rec.Header().Get("X-Accel-Buffering")
	if xab != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", xab)
	}

	// Parse SSE events
	events := parseSSEEvents(t, rec.Body.String())

	// Should have a "result" event and a "done" event
	var resultData string
	var hasDone bool
	for _, ev := range events {
		if ev.Event == "result" {
			resultData = ev.Data
		}
		if ev.Event == "done" {
			hasDone = true
		}
	}

	if resultData == "" {
		t.Fatal("expected result event in SSE stream")
	}
	if !hasDone {
		t.Error("expected done event in SSE stream")
	}

	// Parse the result JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultData), &result); err != nil {
		t.Fatalf("parse result JSON: %v", err)
	}

	// Static review for SELECT should be low risk with "execute" decision
	reviewSource, _ := result["review_source"].(string)
	if reviewSource != "static" {
		t.Errorf("review_source = %q, want static", reviewSource)
	}
}

// ---------------------------------------------------------------------------
// Datasource not found
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_DatasourceNotFound(t *testing.T) {
	e, _, _, h, database := setupAIReviewTest(t)

	userID := seedAIReviewUser(t, database, "testuser", "developer")

	body := `{"datasource_id":99999,"database":"testdb","sql":"SELECT 1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/query/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAIReviewAuth(c, userID, "testuser", "developer")

	err := h.ReviewStream(c)
	if err == nil {
		t.Fatalf("expected error for missing datasource, got nil; body=%s", rec.Body.String())
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
	}
	if he.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", he.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// Blocked SQL
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_BlockedSQL(t *testing.T) {
	e, _, dsSvc, h, database := setupAIReviewTest(t)

	userID := seedAIReviewUser(t, database, "testuser", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds", "mysql")

	// DROP TABLE should be blocked by static rules
	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"DROP TABLE users"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)

	err := h.ReviewStream(c)
	if err != nil {
		t.Fatalf("ReviewStream returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	events := parseSSEEvents(t, rec.Body.String())
	var resultData string
	for _, ev := range events {
		if ev.Event == "result" {
			resultData = ev.Data
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultData), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	decision, _ := result["decision"].(string)
	if decision != "blocked" {
		t.Errorf("decision = %q, want blocked", decision)
	}
	riskLevel, _ := result["risk_level"].(string)
	if riskLevel != "high" {
		t.Errorf("risk_level = %q, want high", riskLevel)
	}
}

// ---------------------------------------------------------------------------
// Invalid SQL syntax
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_InvalidSQL(t *testing.T) {
	e, _, dsSvc, h, database := setupAIReviewTest(t)

	userID := seedAIReviewUser(t, database, "testuser", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds", "mysql")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"INVALID SQL SYNTAX @@@@@"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)

	err := h.ReviewStream(c)
	// Invalid SQL may be parsed as unknown type, still returns 200 with static result
	// The parser is lenient; verify we get a valid SSE response
	if err != nil {
		// If parser returns error, handler returns 400
		he, ok := err.(*echo.HTTPError)
		if ok && he.Code == http.StatusBadRequest {
			return // expected for truly unparseable SQL
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	events := parseSSEEvents(t, rec.Body.String())
	if len(events) == 0 {
		t.Fatal("expected SSE events in response")
	}
}

// ---------------------------------------------------------------------------
// MongoDB datasource type
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_MongoDB(t *testing.T) {
	e, _, dsSvc, h, database := setupAIReviewTest(t)

	userID := seedAIReviewUser(t, database, "testuser", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-mongo", "mongodb")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"{\"operation\":\"find\",\"collection\":\"users\",\"filter\":{}}"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)

	err := h.ReviewStream(c)
	if err != nil {
		t.Fatalf("ReviewStream returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	events := parseSSEEvents(t, rec.Body.String())
	var resultData string
	for _, ev := range events {
		if ev.Event == "result" {
			resultData = ev.Data
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultData), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	// Should have a valid result from static rules
	if result["risk_level"] == nil {
		t.Error("expected risk_level in result")
	}
}

// ---------------------------------------------------------------------------
// With mock LLM streaming
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_WithMockLLM(t *testing.T) {
	mockHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}

		chunks := []string{
			`{"choices":[{"delta":{"content":"{\"risk_"}}]}`,
			`{"choices":[{"delta":{"content":"level\":\"low\",\"risk"}}]}`,
			`{"choices":[{"delta":{"content":"_score\":10,\"summary\":\"Simple SELECT query\",\"suggestions\":[],\"impact_analysis\":\"none\",\"rollback_sql\":\"\"}"}}]}`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}

	e, _, dsSvc, h, database := setupAIReviewTestWithMockLLM(t, mockHandler)

	userID := seedAIReviewUser(t, database, "testuser", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds", "mysql")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"SELECT * FROM users LIMIT 10"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)

	err := h.ReviewStream(c)
	if err != nil {
		t.Fatalf("ReviewStream returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	events := parseSSEEvents(t, rec.Body.String())

	var contentEvents int
	var resultData string
	var hasDone bool
	for _, ev := range events {
		switch ev.Event {
		case "content":
			contentEvents++
		case "result":
			resultData = ev.Data
		case "done":
			hasDone = true
		}
	}

	if contentEvents == 0 {
		t.Error("expected content events from LLM streaming")
	}
	if resultData == "" {
		t.Fatal("expected result event")
	}
	if !hasDone {
		t.Error("expected done event")
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultData), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	reviewSource, _ := result["review_source"].(string)
	if reviewSource != "ai" {
		t.Errorf("review_source = %q, want ai", reviewSource)
	}
}

// ---------------------------------------------------------------------------
// Mock LLM error → fallback to static
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_LLMError_Fallback(t *testing.T) {
	mockHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error": "internal server error"}`)
	}

	e, _, dsSvc, h, database := setupAIReviewTestWithMockLLM(t, mockHandler)

	userID := seedAIReviewUser(t, database, "testuser", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds", "mysql")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"SELECT * FROM users LIMIT 10"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)

	err := h.ReviewStream(c)
	if err != nil {
		t.Fatalf("ReviewStream returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	events := parseSSEEvents(t, rec.Body.String())

	var resultData string
	for _, ev := range events {
		if ev.Event == "result" {
			resultData = ev.Data
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultData), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	reviewSource, _ := result["review_source"].(string)
	if reviewSource != "static" {
		t.Errorf("review_source = %q, want static (fallback after LLM error)", reviewSource)
	}
}

// ---------------------------------------------------------------------------
// Mock LLM network unreachable → fallback with error event
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_LLMNetworkError(t *testing.T) {
	// Create a server that's already closed to simulate network error
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	database, err := db.Open(t.TempDir() + "/test_net_err.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef"
	connMgr := connpool.NewManager()
	dsSvc := service.NewDatasourceService(database.DB, encKey, connMgr)
	aiSvc := service.NewAIReviewService(database.DB, "openai", "test-model", "test-key", server.URL, 2*time.Second)
	h := NewAIReviewHandler(aiSvc, dsSvc)

	e := echo.New()
	userID := seedAIReviewUser(t, database, "testuser2", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds2", "mysql")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"SELECT * FROM users LIMIT 10"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)
	// Override username for this test
	c.Set(middleware.ContextKeyUsername, "testuser2")

	err = h.ReviewStream(c)
	if err != nil {
		t.Fatalf("ReviewStream returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	events := parseSSEEvents(t, rec.Body.String())
	var hasError bool
	var resultData string
	for _, ev := range events {
		if ev.Event == "error" {
			hasError = true
		}
		if ev.Event == "result" {
			resultData = ev.Data
		}
	}

	if !hasError {
		t.Error("expected error event in SSE stream for network failure")
	}

	if resultData == "" {
		t.Fatal("expected result event (fallback)")
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultData), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	reviewSource, _ := result["review_source"].(string)
	if reviewSource == "" {
		t.Error("expected review_source in fallback result")
	}
}

// ---------------------------------------------------------------------------
// SSE format verification
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_SSEFormat(t *testing.T) {
	e, _, dsSvc, h, database := setupAIReviewTest(t)

	userID := seedAIReviewUser(t, database, "testuser", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds", "mysql")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"SELECT 1"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)

	err := h.ReviewStream(c)
	if err != nil {
		t.Fatalf("ReviewStream returned error: %v", err)
	}

	respBody := rec.Body.String()

	// Verify SSE format: each event block should have "event:" and "data:" lines
	if !strings.Contains(respBody, "event: ") {
		t.Error("SSE response missing 'event: ' prefix")
	}
	if !strings.Contains(respBody, "data: ") {
		t.Error("SSE response missing 'data: ' prefix")
	}

	// Verify the [DONE] marker
	if !strings.Contains(respBody, "event: done\ndata: {}\n\n") {
		t.Errorf("SSE response missing done event; body:\n%s", respBody)
	}

	// Verify each event block ends with double newline
	lines := strings.Split(respBody, "\n\n")
	for i, block := range lines {
		if block == "" {
			continue
		}
		if !strings.Contains(block, "event:") || !strings.Contains(block, "data:") {
			t.Errorf("SSE block %d is malformed: %q", i, block)
		}
	}
}

// ---------------------------------------------------------------------------
// High-risk SQL (UPDATE without WHERE)
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_HighRiskSQL(t *testing.T) {
	e, _, dsSvc, h, database := setupAIReviewTest(t)

	userID := seedAIReviewUser(t, database, "testuser", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds", "mysql")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"UPDATE users SET name = 'hacked'"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)

	err := h.ReviewStream(c)
	if err != nil {
		t.Fatalf("ReviewStream returned error: %v", err)
	}

	events := parseSSEEvents(t, rec.Body.String())
	var resultData string
	for _, ev := range events {
		if ev.Event == "result" {
			resultData = ev.Data
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultData), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	// UPDATE without WHERE should be high risk (may be "blocked" or "ticket")
	riskLevel, _ := result["risk_level"].(string)
	if riskLevel != "high" {
		t.Errorf("risk_level = %q, want high for UPDATE without WHERE", riskLevel)
	}
	decision, _ := result["decision"].(string)
	if decision != "ticket" && decision != "blocked" {
		t.Errorf("decision = %q, want ticket or blocked for high risk DML without WHERE", decision)
	}
}

// ---------------------------------------------------------------------------
// DDL operation
// ---------------------------------------------------------------------------

func TestAIReviewHandler_ReviewStream_DDLOperation(t *testing.T) {
	e, _, dsSvc, h, database := setupAIReviewTest(t)

	userID := seedAIReviewUser(t, database, "testuser", "developer")
	ds := seedAIReviewDatasource(t, dsSvc, "test-ds", "mysql")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"testdb","sql":"ALTER TABLE users ADD COLUMN phone VARCHAR(20)"}`, ds.ID)
	c, rec := newSSERequest(e, userID, body)

	err := h.ReviewStream(c)
	if err != nil {
		t.Fatalf("ReviewStream returned error: %v", err)
	}

	events := parseSSEEvents(t, rec.Body.String())
	var resultData string
	for _, ev := range events {
		if ev.Event == "result" {
			resultData = ev.Data
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultData), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	riskLevel, _ := result["risk_level"].(string)
	if riskLevel != "high" {
		t.Errorf("risk_level = %q, want high for DDL", riskLevel)
	}
	decision, _ := result["decision"].(string)
	if decision != "ticket" {
		t.Errorf("decision = %q, want ticket for DDL", decision)
	}

	// Should have DDL-related suggestions
	suggestions, _ := result["suggestions"].([]interface{})
	foundDDLHint := false
	for _, s := range suggestions {
		if strings.Contains(fmt.Sprintf("%v", s), "DDL") {
			foundDDLHint = true
		}
	}
	if !foundDDLHint {
		t.Error("expected DDL-related suggestion")
	}
}
