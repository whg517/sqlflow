package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// reqRecorder is a concurrency-safe helper that captures the last request body
// from a test server and exposes a channel-based synchronization primitive so
// tests can wait deterministically for the async goroutine spawned by
// Service (which uses fire-and-forget `go s.sendMarkdown(...)`) instead
// of brittle time.Sleep calls.
type reqRecorder struct {
	mu       sync.Mutex
	body     dingTalkRequest
	feishu   map[string]interface{}
	url      string
	called   bool
	notifyCh chan struct{} // closed-per-call signal (see newReqRecorder / signalOnce)
}

func newReqRecorder() *reqRecorder {
	return &reqRecorder{notifyCh: make(chan struct{}, 32)}
}

// signal marks one request received (non-blocking).
func (r *reqRecorder) signal() {
	select {
	case r.notifyCh <- struct{}{}:
	default:
	}
}

// waitFor blocks until at least `n` requests are received or the timeout
// elapses. Returns the number of requests received in that window.
func (r *reqRecorder) waitFor(t *testing.T, n int, timeout time.Duration) int {
	t.Helper()
	deadline := time.After(timeout)
	got := 0
	for got < n {
		select {
		case <-r.notifyCh:
			got++
		case <-deadline:
			return got
		}
	}
	return got
}

// lastBody returns a copy of the last captured DingTalk request body.
func (r *reqRecorder) lastBody() dingTalkRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body
}

func (r *reqRecorder) lastURL() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.url
}

func (r *reqRecorder) wasCalled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.called
}

// reset clears captured state between subtests.
func (r *reqRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.body = dingTalkRequest{}
	r.feishu = nil
	r.url = ""
	r.called = false
	// drain pending signals
	for {
		select {
		case <-r.notifyCh:
		default:
			return
		}
	}
}

func TestNewNotifyService(t *testing.T) {
	t.Run("enabled when webhook provided", func(t *testing.T) {
		svc := NewService(Deps{WebhookURL: "https://example.com/webhook", Secret: "secret123"})
		if !svc.IsEnabled() {
			t.Error("expected service to be enabled")
		}
	})

	t.Run("disabled when webhook empty", func(t *testing.T) {
		svc := NewService(Deps{})
		if svc.IsEnabled() {
			t.Error("expected service to be disabled")
		}
	})
}

func TestNotifyService_UpdateConfig(t *testing.T) {
	svc := NewService(Deps{})
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
		svc := NewService(Deps{WebhookURL: "https://example.com/webhook", Secret: "abcdefgh"})
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
		svc := NewService(Deps{WebhookURL: "https://example.com/webhook", Secret: "abc"})
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
		svc := NewService(Deps{WebhookURL: "https://example.com/webhook", Secret: ""})
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
	svc := NewService(Deps{})

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

	svc.NotifyTicketCreated(context.Background(), ticket)
	svc.NotifyTicketApproved(context.Background(), ticket)
	svc.NotifyTicketRejected(context.Background(), ticket)
	svc.NotifyTicketExecuted(context.Background(), ticket)
	svc.NotifyTicketFailed(context.Background(), ticket, "some error")
	svc.NotifyRiskAlert("user", "SELECT 1", "medium", 1, "testdb")
	svc.SendTestMessage()
}

func TestNotifyService_SendsWebhook(t *testing.T) {
	rec := newReqRecorder()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		if err := json.NewDecoder(r.Body).Decode(&rec.body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		rec.called = true
		rec.mu.Unlock()
		rec.signal()

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	svc := NewService(Deps{WebhookURL: server.URL, Secret: ""})

	t.Run("ticket created notification", func(t *testing.T) {
		rec.reset()
		ticket := &model.Ticket{
			ID:            42,
			SubmitterName: "alice",
			DatasourceID:  1,
			Database:      "mydb",
			SQLSummary:    "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
			RiskLevel:     "medium",
			CreatedAt:     time.Now(),
		}
		svc.NotifyTicketCreated(context.Background(), ticket)

		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected webhook to be called")
		}
		body := rec.lastBody()
		if body.MsgType != "markdown" {
			t.Errorf("MsgType = %s, want markdown", body.MsgType)
		}
		if body.Markdown == nil {
			t.Fatal("Markdown is nil")
		}
		if body.Markdown.Title == "" {
			t.Error("Markdown.Title is empty")
		}
		if body.Markdown.Text == "" {
			t.Error("Markdown.Text is empty")
		}
	})

	t.Run("risk alert notification", func(t *testing.T) {
		rec.reset()
		svc.NotifyRiskAlert("bob", "DELETE FROM users", "high", 1, "proddb")

		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected webhook to be called for risk alert")
		}
		body := rec.lastBody()
		if body.Markdown == nil {
			t.Fatal("Markdown is nil")
		}
		if body.Markdown.Title != "⚠️ 风险操作告警" {
			t.Errorf("Title = %s, want ⚠️ 风险操作告警", body.Markdown.Title)
		}
	})

	t.Run("test message", func(t *testing.T) {
		rec.reset()
		svc.SendTestMessage()

		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected webhook to be called for test message")
		}
		if !rec.wasCalled() {
			t.Error("expected webhook to be called for test message")
		}
	})
}

func TestNotifyService_Signature(t *testing.T) {
	rec := newReqRecorder()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.url = r.URL.String()
		rec.mu.Unlock()
		rec.signal()

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	svc := NewService(Deps{WebhookURL: server.URL, Secret: "test-secret-key"})

	ticket := &model.Ticket{
		ID:            1,
		SubmitterName: "alice",
		DatasourceID:  1,
		Database:      "mydb",
		SQLSummary:    "SELECT 1",
		RiskLevel:     "low",
		CreatedAt:     time.Now(),
	}
	svc.NotifyTicketCreated(context.Background(), ticket)

	if rec.waitFor(t, 1, 2*time.Second) != 1 {
		t.Fatal("no request received")
	}
	u := rec.lastURL()

	// URL should contain timestamp and sign parameters
	if !containsParam(u, "timestamp") {
		t.Error("URL missing timestamp parameter")
	}
	if !containsParam(u, "sign") {
		t.Error("URL missing sign parameter")
	}
}

func TestNotifyService_ErrorHandling(t *testing.T) {
	t.Run("handles server error response without panic", func(t *testing.T) {
		rec := newReqRecorder()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec.signal()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"errcode":300001,"errmsg":"token is not exist"}`))
		}))
		defer server.Close()

		svc := NewService(Deps{WebhookURL: server.URL, Secret: ""})

		ticket := &model.Ticket{
			ID:            1,
			SubmitterName: "alice",
			DatasourceID:  1,
			Database:      "mydb",
			SQLSummary:    "SELECT 1",
			RiskLevel:     "low",
			CreatedAt:     time.Now(),
		}
		// The contract under test: a non-OK errcode in the response body must
		// not panic the notifier. We additionally assert the request reached
		// the server so the error path was actually exercised.
		svc.NotifyTicketCreated(context.Background(), ticket)
		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected request to reach the error-responding server")
		}
	})

	t.Run("handles connection refused without panic", func(t *testing.T) {
		svc := NewService(Deps{WebhookURL: "http://127.0.0.1:1/webhook", Secret: ""})

		ticket := &model.Ticket{
			ID:            1,
			SubmitterName: "alice",
			DatasourceID:  1,
			Database:      "mydb",
			SQLSummary:    "SELECT 1",
			RiskLevel:     "low",
			CreatedAt:     time.Now(),
		}
		// Connection-refused has no observable success side effect; the only
		// contract is "must not panic". We still bound the wait deterministically
		// via the http.Client 5s timeout rather than an arbitrary sleep.
		done := make(chan struct{})
		go func() {
			defer close(done)
			svc.NotifyTicketCreated(context.Background(), ticket)
		}()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Fatal("NotifyTicketCreated did not return within 6s on connection refused")
		}
	})
}

func TestNotifyService_TicketLifecycleNotifications(t *testing.T) {
	var mu sync.Mutex
	var messages []string
	rec := newReqRecorder()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req dingTalkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Markdown != nil {
			mu.Lock()
			messages = append(messages, req.Markdown.Title)
			mu.Unlock()
		}
		rec.signal()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	svc := NewService(Deps{WebhookURL: server.URL, Secret: ""})

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

	svc.NotifyTicketCreated(context.Background(), ticket)
	svc.NotifyTicketApproved(context.Background(), ticket)
	svc.NotifyTicketRejected(context.Background(), ticket)
	svc.NotifyTicketExecuted(context.Background(), ticket)
	svc.NotifyTicketFailed(context.Background(), ticket, "execution error")

	// Wait deterministically for all 5 lifecycle notifications.
	if got := rec.waitFor(t, 5, 3*time.Second); got != 5 {
		mu.Lock()
		t.Errorf("expected 5 notifications, got %d (titles so far: %v)", got, messages)
		mu.Unlock()
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
	svc := NewService(Deps{WebhookURL: "", Secret: "test-secret"})

	// Test that sign produces a non-empty string
	result := svc.sign(1700000000000, "test-secret")
	if result == "" {
		t.Error("sign returned empty string")
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
		svc := NewService(Deps{})
		if svc.IsFeishuEnabled() {
			t.Error("expected Feishu disabled when no URL set")
		}
	})

	t.Run("enabled after setting webhook", func(t *testing.T) {
		svc := NewService(Deps{FeishuURL: "https://open.feishu.cn/open-apis/bot/v2/hook/test"})
		if !svc.IsFeishuEnabled() {
			t.Error("expected Feishu enabled after setting webhook")
		}
	})

	t.Run("config returns webhook info", func(t *testing.T) {
		svc := NewService(Deps{FeishuURL: "https://example.com/feishu"})
		cfg := svc.GetFeishuConfig()
		if cfg["webhook_url"] != "https://example.com/feishu" {
			t.Errorf("webhook_url = %v, want https://example.com/feishu", cfg["webhook_url"])
		}
		if cfg["enabled"] != true {
			t.Error("expected enabled=true")
		}
	})

	t.Run("disabled after clearing webhook", func(t *testing.T) {
		svc := NewService(Deps{FeishuURL: "https://example.com/feishu"})
		svc.UpdateFeishuConfig("")
		if svc.IsFeishuEnabled() {
			t.Error("expected Feishu disabled after clearing webhook")
		}
	})
}

func TestNotifyService_FeishuSendsWebhook(t *testing.T) {
	var mu sync.Mutex
	var receivedBodies []map[string]interface{}
	rec := newReqRecorder()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			mu.Lock()
			receivedBodies = append(receivedBodies, body)
			mu.Unlock()
		}
		rec.signal()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer server.Close()

	svc := NewService(Deps{FeishuURL: server.URL})

	t.Run("ticket created sends feishu card", func(t *testing.T) {
		mu.Lock()
		receivedBodies = nil
		mu.Unlock()
		rec.reset()

		ticket := &model.Ticket{
			ID:            100,
			SubmitterName: "alice",
			DatasourceID:  1,
			Database:      "mydb",
			SQLSummary:    "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
			RiskLevel:     "medium",
			CreatedAt:     time.Now(),
		}
		svc.NotifyTicketCreated(context.Background(), ticket)

		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected feishu webhook to be called")
		}
		mu.Lock()
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
		rec.reset()

		ticket := &model.Ticket{
			ID:            200,
			SubmitterName: "alice",
			ReviewerName:  "bob",
			SQLSummary:    "SELECT 1",
			RiskLevel:     "low",
			UpdatedAt:     time.Now(),
		}
		svc.NotifyTicketApproved(context.Background(), ticket)

		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected feishu webhook to be called for approval")
		}
	})

	t.Run("ticket rejected sends feishu card", func(t *testing.T) {
		mu.Lock()
		receivedBodies = nil
		mu.Unlock()
		rec.reset()

		ticket := &model.Ticket{
			ID:            300,
			SubmitterName: "alice",
			ReviewerName:  "bob",
			SQLSummary:    "DELETE FROM users",
			ReviewComment: "不允许全表删除",
			RiskLevel:     "high",
			UpdatedAt:     time.Now(),
		}
		svc.NotifyTicketRejected(context.Background(), ticket)

		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected feishu webhook to be called for rejection")
		}
	})

	t.Run("ticket failed sends feishu card", func(t *testing.T) {
		mu.Lock()
		receivedBodies = nil
		mu.Unlock()
		rec.reset()

		ticket := &model.Ticket{
			ID:            400,
			SubmitterName: "alice",
			SQLSummary:    "DROP TABLE users",
			UpdatedAt:     time.Now(),
		}
		svc.NotifyTicketFailed(context.Background(), ticket, "table does not exist")

		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected feishu webhook to be called for failure")
		}
		mu.Lock()
		body := receivedBodies[0]
		mu.Unlock()

		card, _ := body["card"].(map[string]interface{})
		header, _ := card["header"].(map[string]interface{})
		title, _ := header["title"].(map[string]interface{})
		if title["content"] != "🚨 工单执行失败通知" {
			t.Errorf("title content = %v", title["content"])
		}
	})

	t.Run("ticket scheduled sends feishu card", func(t *testing.T) {
		mu.Lock()
		receivedBodies = nil
		mu.Unlock()
		rec.reset()

		schedTime := time.Now().Add(1 * time.Hour)
		ticket := &model.Ticket{
			ID:            500,
			SubmitterName: "alice",
			SQLSummary:    "ALTER TABLE users ADD COLUMN email VARCHAR(50)",
			RiskLevel:     "low",
			ScheduledAt:   &schedTime,
			UpdatedAt:     time.Now(),
		}
		svc.NotifyTicketScheduled(context.Background(), ticket)

		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected feishu webhook to be called for scheduled")
		}
	})
}

func TestNotifyService_FeishuDisabledNoop(t *testing.T) {
	svc := NewService(Deps{})
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
	svc.NotifyTicketCreated(context.Background(), ticket)
	svc.NotifyTicketApproved(context.Background(), ticket)
	svc.NotifyTicketRejected(context.Background(), ticket)
	svc.NotifyTicketExecuted(context.Background(), ticket)
	svc.NotifyTicketFailed(context.Background(), ticket, "error")
	svc.NotifyTicketScheduled(context.Background(), ticket)
}

func TestNotifyService_FeishuTestMessage(t *testing.T) {
	rec := newReqRecorder()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.signal()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer server.Close()

	svc := NewService(Deps{FeishuURL: server.URL})

	svc.SendFeishuTestMessage()
	if rec.waitFor(t, 1, 2*time.Second) != 1 {
		t.Error("expected feishu test message to be sent")
	}
}

func TestNotifyService_BothChannelsSimultaneously(t *testing.T) {
	dingRec := newReqRecorder()
	feishuRec := newReqRecorder()

	dingtalkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dingRec.signal()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer dingtalkServer.Close()

	feishuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		feishuRec.signal()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer feishuServer.Close()

	svc := NewService(Deps{WebhookURL: dingtalkServer.URL, FeishuURL: feishuServer.URL})

	ticket := &model.Ticket{
		ID:            1,
		SubmitterName: "test",
		DatasourceID:  1,
		Database:      "testdb",
		SQLSummary:    "SELECT 1",
		RiskLevel:     "low",
		CreatedAt:     time.Now(),
	}
	svc.NotifyTicketCreated(context.Background(), ticket)

	if dingRec.waitFor(t, 1, 2*time.Second) != 1 {
		t.Error("expected DingTalk to be called")
	}
	if feishuRec.waitFor(t, 1, 2*time.Second) != 1 {
		t.Error("expected Feishu to be called")
	}
}

// ---------------------------------------------------------------------------
// Deduplication tests (SF-FEAT0049)
// ---------------------------------------------------------------------------

func TestNotifyService_Dedup(t *testing.T) {
	t.Run("nil db allows all notifications through", func(t *testing.T) {
		rec := newReqRecorder()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec.signal()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		}))
		defer server.Close()

		svc := NewService(Deps{WebhookURL: server.URL, Secret: ""}) // no DB attached → shouldNotify always returns true

		// Same ticket, same event type sent twice — without a DB the dedup layer
		// cannot suppress the second one, so both must reach the webhook.
		ticket := &model.Ticket{ID: 1, SubmitterName: "test", SQLSummary: "SELECT 1", CreatedAt: time.Now()}
		svc.NotifyTicketCreated(context.Background(), ticket)
		svc.NotifyTicketCreated(context.Background(), ticket)

		if got := rec.waitFor(t, 2, 2*time.Second); got != 2 {
			t.Errorf("expected 2 notifications with nil-db (no dedup), got %d", got)
		}
	})
}

func TestNotifyService_FailedNotificationContent(t *testing.T) {
	rec := newReqRecorder()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		json.NewDecoder(r.Body).Decode(&rec.body)
		rec.mu.Unlock()
		rec.signal()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	svc := NewService(Deps{WebhookURL: server.URL, Secret: ""})

	t.Run("error message truncated at 200 chars with marker", func(t *testing.T) {
		rec.reset()
		longErr := strings.Repeat("x", 300)
		ticket := &model.Ticket{
			ID: 1, SubmitterName: "test", SQLSummary: "SELECT 1", UpdatedAt: time.Now(),
		}
		svc.NotifyTicketFailed(context.Background(), ticket, longErr)

		if rec.waitFor(t, 1, 2*time.Second) != 1 {
			t.Fatal("expected failure notification to be sent")
		}
		body := rec.lastBody()
		if body.Markdown == nil {
			t.Fatal("Markdown is nil")
		}
		text := body.Markdown.Text
		if text == "" {
			t.Fatal("expected non-empty markdown text")
		}
		// Truncated error must be capped at 200 chars + the truncation marker.
		if !strings.Contains(text, "...") {
			t.Errorf("expected truncation marker '...' in text, got: %s", text)
		}
		// The 300-char error must not appear in full (first 200 'x' may appear,
		// but the 201st..300th 'x' run is replaced by '...').
		if strings.Contains(text, strings.Repeat("x", 201)) {
			t.Error("expected error to be truncated below 201 consecutive 'x' chars")
		}
		if body.Markdown.Title != "🚨 工单执行失败通知" {
			t.Errorf("Title = %s, want 🚨 工单执行失败通知", body.Markdown.Title)
		}
	})
}
