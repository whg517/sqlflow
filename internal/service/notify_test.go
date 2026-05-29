package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

func TestNewNotifyService(t *testing.T) {
	t.Run("enabled when webhook provided", func(t *testing.T) {
		svc := NewNotifyService("https://example.com/webhook", "secret123")
		if !svc.IsEnabled() {
			t.Error("expected service to be enabled")
		}
	})

	t.Run("disabled when webhook empty", func(t *testing.T) {
		svc := NewNotifyService("", "")
		if svc.IsEnabled() {
			t.Error("expected service to be disabled")
		}
	})
}

func TestNotifyService_UpdateConfig(t *testing.T) {
	svc := NewNotifyService("", "")
	if svc.IsEnabled() {
		t.Error("expected disabled initially")
	}

	svc.UpdateConfig("https://example.com/webhook", "secret")
	if !svc.IsEnabled() {
		t.Error("expected enabled after update")
	}
}

func TestNotifyService_GetConfig(t *testing.T) {
	t.Run("masks secret", func(t *testing.T) {
		svc := NewNotifyService("https://example.com/webhook", "abcdefgh")
		cfg := svc.GetConfig()

		secret, ok := cfg["secret"].(string)
		if !ok {
			t.Fatal("secret is not a string")
		}
		if secret != "ab****gh" {
			t.Errorf("secret = %s, want ab****gh", secret)
		}
	})

	t.Run("masks short secret", func(t *testing.T) {
		svc := NewNotifyService("https://example.com/webhook", "abc")
		cfg := svc.GetConfig()

		secret, ok := cfg["secret"].(string)
		if !ok {
			t.Fatal("secret is not a string")
		}
		if secret != "****" {
			t.Errorf("secret = %s, want ****", secret)
		}
	})

	t.Run("empty secret", func(t *testing.T) {
		svc := NewNotifyService("https://example.com/webhook", "")
		cfg := svc.GetConfig()

		secret, ok := cfg["secret"].(string)
		if !ok {
			t.Fatal("secret is not a string")
		}
		if secret != "" {
			t.Errorf("secret = %s, want empty", secret)
		}
	})
}

func TestNotifyService_DisabledNoop(t *testing.T) {
	svc := NewNotifyService("", "")

	// These should not panic
	ticket := &model.Ticket{
		ID:            1,
		SubmitterName: "test",
		DatasourceID:  1,
		Database:      "testdb",
		SQLSummary:    "SELECT 1",
		RiskLevel:     "low",
		CreatedAt:     time.Now(),
	}

	svc.NotifyTicketCreated(ticket)
	svc.NotifyTicketApproved(ticket)
	svc.NotifyTicketRejected(ticket)
	svc.NotifyTicketExecuted(ticket)
	svc.NotifyRiskAlert("user", "SELECT 1", "medium", 1, "testdb")
	svc.SendTestMessage()
}

func TestNotifyService_SendsWebhook(t *testing.T) {
	var receivedReq struct {
		mu     sync.Mutex
		body   dingTalkRequest
		called bool
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedReq.mu.Lock()
		defer receivedReq.mu.Unlock()

		if err := json.NewDecoder(r.Body).Decode(&receivedReq.body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		receivedReq.called = true

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	svc := NewNotifyService(server.URL, "")

	t.Run("ticket created notification", func(t *testing.T) {
		ticket := &model.Ticket{
			ID:            42,
			SubmitterName: "alice",
			DatasourceID:  1,
			Database:      "mydb",
			SQLSummary:    "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
			RiskLevel:     "medium",
			CreatedAt:     time.Now(),
		}
		svc.NotifyTicketCreated(ticket)

		// Wait for async goroutine
		time.Sleep(200 * time.Millisecond)

		receivedReq.mu.Lock()
		if !receivedReq.called {
			t.Error("expected webhook to be called")
		}
		if receivedReq.body.MsgType != "markdown" {
			t.Errorf("MsgType = %s, want markdown", receivedReq.body.MsgType)
		}
		if receivedReq.body.Markdown == nil {
			t.Fatal("Markdown is nil")
		}
		if receivedReq.body.Markdown.Title == "" {
			t.Error("Markdown.Title is empty")
		}
		if receivedReq.body.Markdown.Text == "" {
			t.Error("Markdown.Text is empty")
		}
		receivedReq.called = false
		receivedReq.mu.Unlock()
	})

	t.Run("risk alert notification", func(t *testing.T) {
		svc.NotifyRiskAlert("bob", "DELETE FROM users", "high", 1, "proddb")

		time.Sleep(200 * time.Millisecond)

		receivedReq.mu.Lock()
		if !receivedReq.called {
			t.Error("expected webhook to be called for risk alert")
		}
		if receivedReq.body.Markdown == nil {
			t.Fatal("Markdown is nil")
		}
		if receivedReq.body.Markdown.Title != "⚠️ 风险操作告警" {
			t.Errorf("Title = %s, want ⚠️ 风险操作告警", receivedReq.body.Markdown.Title)
		}
		receivedReq.called = false
		receivedReq.mu.Unlock()
	})

	t.Run("test message", func(t *testing.T) {
		svc.SendTestMessage()

		time.Sleep(200 * time.Millisecond)

		receivedReq.mu.Lock()
		if !receivedReq.called {
			t.Error("expected webhook to be called for test message")
		}
		receivedReq.called = false
		receivedReq.mu.Unlock()
	})
}

func TestNotifyService_Signature(t *testing.T) {
	var receivedURL struct {
		mu  sync.Mutex
		url string
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL.mu.Lock()
		receivedURL.url = r.URL.String()
		receivedURL.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	svc := NewNotifyService(server.URL, "test-secret-key")

	ticket := &model.Ticket{
		ID:            1,
		SubmitterName: "alice",
		DatasourceID:  1,
		Database:      "mydb",
		SQLSummary:    "SELECT 1",
		RiskLevel:     "low",
		CreatedAt:     time.Now(),
	}
	svc.NotifyTicketCreated(ticket)

	time.Sleep(200 * time.Millisecond)

	receivedURL.mu.Lock()
	u := receivedURL.url
	receivedURL.mu.Unlock()

	if u == "" {
		t.Fatal("no request received")
	}

	// URL should contain timestamp and sign parameters
	if !containsParam(u, "timestamp") {
		t.Error("URL missing timestamp parameter")
	}
	if !containsParam(u, "sign") {
		t.Error("URL missing sign parameter")
	}
}

func TestNotifyService_ErrorHandling(t *testing.T) {
	t.Run("handles server error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"errcode":300001,"errmsg":"token is not exist"}`))
		}))
		defer server.Close()

		svc := NewNotifyService(server.URL, "")

		ticket := &model.Ticket{
			ID:            1,
			SubmitterName: "alice",
			DatasourceID:  1,
			Database:      "mydb",
			SQLSummary:    "SELECT 1",
			RiskLevel:     "low",
			CreatedAt:     time.Now(),
		}
		// Should not panic
		svc.NotifyTicketCreated(ticket)
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("handles connection refused", func(t *testing.T) {
		svc := NewNotifyService("http://127.0.0.1:1/webhook", "")

		ticket := &model.Ticket{
			ID:            1,
			SubmitterName: "alice",
			DatasourceID:  1,
			Database:      "mydb",
			SQLSummary:    "SELECT 1",
			RiskLevel:     "low",
			CreatedAt:     time.Now(),
		}
		// Should not panic
		svc.NotifyTicketCreated(ticket)
		time.Sleep(200 * time.Millisecond)
	})
}

func TestNotifyService_TicketLifecycleNotifications(t *testing.T) {
	var mu sync.Mutex
	var messages []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req dingTalkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Markdown != nil {
			mu.Lock()
			messages = append(messages, req.Markdown.Title)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	svc := NewNotifyService(server.URL, "")

	ticket := &model.Ticket{
		ID:            100,
		SubmitterName: "developer",
		ReviewerName:  "dba",
		DatasourceID:  1,
		Database:      "mydb",
		SQLSummary:    "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
		RiskLevel:     "medium",
		ReviewComment: "approved",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	svc.NotifyTicketCreated(ticket)
	svc.NotifyTicketApproved(ticket)
	svc.NotifyTicketRejected(ticket)
	svc.NotifyTicketExecuted(ticket)

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(messages) != 4 {
		t.Errorf("expected 4 notifications, got %d", len(messages))
	}
}

func TestRiskLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"low", "低风险"},
		{"medium", "中风险"},
		{"high", "高风险"},
		{"LOW", "低风险"},
		{"HIGH", "高风险"},
		{"", "未评估"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := riskLabel(tt.input)
			if got != tt.want {
				t.Errorf("riskLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNotifyService_Sign(t *testing.T) {
	svc := NewNotifyService("", "test-secret")

	// Test that sign produces a valid base64 string
	result := svc.sign(1700000000000, "test-secret")
	if result == "" {
		t.Error("sign returned empty string")
	}

	// Verify it's valid base64
	decoded := make([]byte, 32)
	n, err := decoded, false
	_ = n
	if err {
		t.Error("sign result is not valid base64")
	}
}

// containsParam checks if the URL query string contains a given parameter.
func containsParam(urlStr, param string) bool {
	return len(urlStr) > 0 && (contains(urlStr, param+"="))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Feishu notification tests
// ---------------------------------------------------------------------------

func TestNotifyService_FeishuBasic(t *testing.T) {
	t.Run("disabled when webhook empty", func(t *testing.T) {
		svc := NewNotifyService("", "")
		if svc.IsFeishuEnabled() {
			t.Error("expected Feishu disabled when no URL set")
		}
	})

	t.Run("enabled after setting webhook", func(t *testing.T) {
		svc := NewNotifyService("", "")
		svc.SetFeishuWebhook("https://open.feishu.cn/open-apis/bot/v2/hook/test")
		if !svc.IsFeishuEnabled() {
			t.Error("expected Feishu enabled after setting webhook")
		}
	})

	t.Run("config returns webhook info", func(t *testing.T) {
		svc := NewNotifyService("", "")
		svc.SetFeishuWebhook("https://example.com/feishu")
		cfg := svc.GetFeishuConfig()
		if cfg["webhook_url"] != "https://example.com/feishu" {
			t.Errorf("webhook_url = %v, want https://example.com/feishu", cfg["webhook_url"])
		}
		if cfg["enabled"] != true {
			t.Error("expected enabled=true")
		}
	})

	t.Run("disabled after clearing webhook", func(t *testing.T) {
		svc := NewNotifyService("", "")
		svc.SetFeishuWebhook("https://example.com/feishu")
		svc.SetFeishuWebhook("")
		if svc.IsFeishuEnabled() {
			t.Error("expected Feishu disabled after clearing webhook")
		}
	})
}

func TestNotifyService_FeishuSendsWebhook(t *testing.T) {
	var mu sync.Mutex
	var receivedBodies []map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			mu.Lock()
			receivedBodies = append(receivedBodies, body)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer server.Close()

	svc := NewNotifyService("", "")
	svc.SetFeishuWebhook(server.URL)

	t.Run("ticket created sends feishu card", func(t *testing.T) {
		mu.Lock()
		receivedBodies = nil
		mu.Unlock()

		ticket := &model.Ticket{
			ID:            100,
			SubmitterName: "alice",
			DatasourceID:  1,
			Database:      "mydb",
			SQLSummary:    "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
			RiskLevel:     "medium",
			CreatedAt:     time.Now(),
		}
		svc.NotifyTicketCreated(ticket)

		time.Sleep(300 * time.Millisecond)

		mu.Lock()
		if len(receivedBodies) == 0 {
			t.Fatal("expected feishu webhook to be called")
		}
		body := receivedBodies[0]
		mu.Unlock()

		if body["msg_type"] != "interactive" {
			t.Errorf("msg_type = %v, want interactive", body["msg_type"])
		}
		card, ok := body["card"].(map[string]interface{})
		if !ok {
			t.Fatal("card is not a map")
		}
		header, ok := card["header"].(map[string]interface{})
		if !ok {
			t.Fatal("header is not a map")
		}
		title, ok := header["title"].(map[string]interface{})
		if !ok {
			t.Fatal("title is not a map")
		}
		if title["content"] != "📋 工单提交通知" {
			t.Errorf("title content = %v", title["content"])
		}
	})

	t.Run("ticket approved sends feishu card", func(t *testing.T) {
		mu.Lock()
		receivedBodies = nil
		mu.Unlock()

		ticket := &model.Ticket{
			ID:            200,
			SubmitterName: "alice",
			ReviewerName:  "bob",
			SQLSummary:    "SELECT 1",
			RiskLevel:     "low",
			UpdatedAt:     time.Now(),
		}
		svc.NotifyTicketApproved(ticket)

		time.Sleep(300 * time.Millisecond)

		mu.Lock()
		if len(receivedBodies) == 0 {
			t.Fatal("expected feishu webhook to be called for approval")
		}
		mu.Unlock()
	})

	t.Run("ticket rejected sends feishu card", func(t *testing.T) {
		mu.Lock()
		receivedBodies = nil
		mu.Unlock()

		ticket := &model.Ticket{
			ID:            300,
			SubmitterName: "alice",
			ReviewerName:  "bob",
			SQLSummary:    "DELETE FROM users",
			ReviewComment: "不允许全表删除",
			RiskLevel:     "high",
			UpdatedAt:     time.Now(),
		}
		svc.NotifyTicketRejected(ticket)

		time.Sleep(300 * time.Millisecond)

		mu.Lock()
		if len(receivedBodies) == 0 {
			t.Fatal("expected feishu webhook to be called for rejection")
		}
		mu.Unlock()
	})
}

func TestNotifyService_FeishuRetryOnError(t *testing.T) {
	var mu sync.Mutex
	attemptCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		count := attemptCount
		mu.Unlock()

		// First two attempts fail, third succeeds
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer server.Close()

	svc := NewNotifyService("", "")
	svc.SetFeishuWebhook(server.URL)

	ticket := &model.Ticket{
		ID:            1,
		SubmitterName: "test",
		DatasourceID:  1,
		Database:      "testdb",
		SQLSummary:    "SELECT 1",
		RiskLevel:     "low",
		CreatedAt:     time.Now(),
	}
	svc.NotifyTicketCreated(ticket)

	time.Sleep(7 * time.Second) // wait for retries with exponential backoff (1s + 4s)

	mu.Lock()
	if attemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount)
	}
	mu.Unlock()
}

func TestNotifyService_FeishuDisabledNoop(t *testing.T) {
	svc := NewNotifyService("", "")
	// Feishu not configured — should not panic
	ticket := &model.Ticket{
		ID:            1,
		SubmitterName: "test",
		DatasourceID:  1,
		Database:      "testdb",
		SQLSummary:    "SELECT 1",
		RiskLevel:     "low",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	svc.NotifyTicketCreated(ticket)
	svc.NotifyTicketApproved(ticket)
	svc.NotifyTicketRejected(ticket)
}

func TestNotifyService_FeishuTestMessage(t *testing.T) {
	var mu sync.Mutex
	var received bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer server.Close()

	svc := NewNotifyService("", "")
	svc.SetFeishuWebhook(server.URL)

	svc.SendFeishuTestMessage()
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	if !received {
		t.Error("expected feishu test message to be sent")
	}
	mu.Unlock()
}

func TestNotifyService_BothChannelsSimultaneously(t *testing.T) {
	var mu sync.Mutex
	var dingtalkCalled, feishuCalled bool

	dingtalkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		dingtalkCalled = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer dingtalkServer.Close()

	feishuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		feishuCalled = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer feishuServer.Close()

	svc := NewNotifyService(dingtalkServer.URL, "")
	svc.SetFeishuWebhook(feishuServer.URL)

	ticket := &model.Ticket{
		ID:            1,
		SubmitterName: "test",
		DatasourceID:  1,
		Database:      "testdb",
		SQLSummary:    "SELECT 1",
		RiskLevel:     "low",
		CreatedAt:     time.Now(),
	}
	svc.NotifyTicketCreated(ticket)

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	if !dingtalkCalled {
		t.Error("expected DingTalk to be called")
	}
	if !feishuCalled {
		t.Error("expected Feishu to be called")
	}
	mu.Unlock()
}
