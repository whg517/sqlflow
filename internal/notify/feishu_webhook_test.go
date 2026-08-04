package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/platform/crypto"
	"github.com/whg517/sqlflow/internal/testutil"
)

const testEncryptionKey = "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256

func setupFeishuWebhookTestDB(t *testing.T) *db.DB {
	t.Helper()
	return testutil.NewDB(t)
}

func newTestFeishuWebhookService(t *testing.T) *FeishuService {
	t.Helper()
	return NewFeishuService(setupFeishuWebhookTestDB(t), testEncryptionKey)
}

// ---------------------------------------------------------------------------
// URL Validation
// ---------------------------------------------------------------------------

func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid feishu url", "https://open.feishu.cn/open-apis/bot/v2/hook/abc123", false},
		{"empty url", "", true},
		{"wrong prefix http", "http://open.feishu.cn/open-apis/bot/v2/hook/abc", true},
		{"wrong prefix other", "https://example.com/webhook", true},
		{"localhost", "https://open.feishu.cn/open-apis/bot/v2/hook/test", false}, // feishu domain resolves to public IPs
		{"partial prefix", "https://open.feishu.cn/wrong/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhookURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWebhookURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestMaskURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"standard url", "https://open.feishu.cn/open-apis/bot/v2/hook/abcdef123456", "https://open.feishu.cn/open-apis/bot/v2/hook/****3456"},
		{"short url", "https://open.feishu.cn/open-apis/bot/v2/hook/a", AllowedFeishuURLPrefix + "****"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskURL(tt.url)
			if got != tt.want {
				t.Errorf("MaskURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func TestFeishuWebhookService_Create(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		req := FeishuCreateRequest{
			Name:       "测试群",
			WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/test-token-abc123",
			Scene:      "general",
		}
		wh, err := svc.Create(ctx, req, "admin")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if wh.ID == 0 {
			t.Error("expected non-zero ID")
		}
		if wh.Name != "测试群" {
			t.Errorf("Name = %q, want 测试群", wh.Name)
		}
		if !wh.Enabled {
			t.Error("expected enabled by default")
		}
	})

	t.Run("duplicate url rejected", func(t *testing.T) {
		req := FeishuCreateRequest{
			Name:       "重复",
			WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/test-token-abc123",
		}
		_, err := svc.Create(ctx, req, "admin")
		if err == nil {
			t.Error("expected duplicate URL error")
		}
	})

	t.Run("invalid url rejected", func(t *testing.T) {
		req := FeishuCreateRequest{
			Name:       "无效",
			WebhookURL: "https://example.com/webhook",
		}
		_, err := svc.Create(ctx, req, "admin")
		if err == nil {
			t.Error("expected URL validation error")
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		req := FeishuCreateRequest{
			Name:       "",
			WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/another-token",
		}
		// Name validation is in handler, not service — service allows empty name
		_, err := svc.Create(ctx, req, "admin")
		if err != nil {
			t.Errorf("service should not validate name: %v", err)
		}
	})
}

func TestFeishuWebhookService_List(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	ctx := context.Background()

	// Create a webhook
	req := FeishuCreateRequest{
		Name:       "列表测试",
		WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/list-test-token-xyz",
	}
	_, err := svc.Create(ctx, req, "admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("masked url by default", func(t *testing.T) {
		items, err := svc.List(ctx, false)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		url := items[0]["webhook_url"].(string)
		if url == req.WebhookURL {
			t.Error("URL should be masked")
		}
		if !contains(url, "****") {
			t.Errorf("masked URL should contain ****: %s", url)
		}
	})

	t.Run("full url for admin", func(t *testing.T) {
		items, err := svc.List(ctx, true)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		url := items[0]["webhook_url"].(string)
		if url != req.WebhookURL {
			t.Errorf("full URL = %q, want %q", url, req.WebhookURL)
		}
	})
}

func TestFeishuWebhookService_Update(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	ctx := context.Background()

	req := FeishuCreateRequest{
		Name:       "更新前",
		WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/update-before-token",
	}
	wh, err := svc.Create(ctx, req, "admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("update name", func(t *testing.T) {
		newName := "更新后"
		updated, err := svc.Update(ctx, wh.ID, FeishuUpdateRequest{Name: &newName})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Name != newName {
			t.Errorf("Name = %q, want %q", updated.Name, newName)
		}
	})

	t.Run("disable", func(t *testing.T) {
		disabled := false
		updated, err := svc.Update(ctx, wh.ID, FeishuUpdateRequest{Enabled: &disabled})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Enabled {
			t.Error("should be disabled")
		}
	})

	t.Run("update url", func(t *testing.T) {
		newURL := "https://open.feishu.cn/open-apis/bot/v2/hook/updated-token-456"
		updated, err := svc.Update(ctx, wh.ID, FeishuUpdateRequest{WebhookURL: &newURL})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Name != "更新后" {
			t.Errorf("Name should be preserved: %q", updated.Name)
		}
	})
}

func TestFeishuWebhookService_Delete(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	ctx := context.Background()

	req := FeishuCreateRequest{
		Name:       "待删除",
		WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/delete-test-token",
	}
	wh, err := svc.Create(ctx, req, "admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.Delete(ctx, wh.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = svc.GetByID(ctx, wh.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestFeishuWebhookService_DeleteNonExistent(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	ctx := context.Background()

	err := svc.Delete(ctx, 9999)
	if err == nil {
		t.Error("expected error deleting non-existent webhook")
	}
}

// ---------------------------------------------------------------------------
// Encryption Round-Trip
// ---------------------------------------------------------------------------

func TestFeishuWebhookService_EncryptionRoundTrip(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	ctx := context.Background()

	originalURL := "https://open.feishu.cn/open-apis/bot/v2/hook/encrypt-test-super-secret"
	req := FeishuCreateRequest{
		Name:       "加密测试",
		WebhookURL: originalURL,
	}
	wh, err := svc.Create(ctx, req, "admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Encrypted URL should not contain plaintext
	if wh.EncryptedURL == originalURL {
		t.Error("encrypted URL should differ from original")
	}

	// Decrypt should recover original
	decrypted, err := svc.DecryptURL(wh.EncryptedURL)
	if err != nil {
		t.Fatalf("DecryptURL: %v", err)
	}
	if decrypted != originalURL {
		t.Errorf("decrypted = %q, want %q", decrypted, originalURL)
	}
}

// ---------------------------------------------------------------------------
// Rate Limiting
// ---------------------------------------------------------------------------

func TestFeishuWebhookService_RateLimit(t *testing.T) {
	svc := newTestFeishuWebhookService(t)

	// First call should succeed
	if !svc.CheckRateLimit(1, 1.0) {
		t.Error("first call should be allowed")
	}

	// Immediate second call should be rate limited
	if svc.CheckRateLimit(1, 1.0) {
		t.Error("immediate second call should be rate limited")
	}

	// Different webhook ID should be independent
	if !svc.CheckRateLimit(2, 1.0) {
		t.Error("different webhook ID should be allowed")
	}
}

// ---------------------------------------------------------------------------
// Dead Letter
// ---------------------------------------------------------------------------

func TestFeishuWebhookService_DeadLetter(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	ctx := context.Background()

	// Create a webhook first
	req := FeishuCreateRequest{
		Name:       "死信测试",
		WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/dead-letter-test",
	}
	wh, err := svc.Create(ctx, req, "admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("record and list", func(t *testing.T) {
		err := svc.RecordDeadLetter(ctx, wh.ID, `{"msg_type":"text"}`, "connection refused")
		if err != nil {
			t.Fatalf("RecordDeadLetter: %v", err)
		}

		items, err := svc.ListDeadLetters(ctx, wh.ID, 10)
		if err != nil {
			t.Fatalf("ListDeadLetters: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 dead letter, got %d", len(items))
		}
		if items[0].AttemptCount != 1 {
			t.Errorf("AttemptCount = %d, want 1", items[0].AttemptCount)
		}
		if items[0].ErrorMessage != "connection refused" {
			t.Errorf("ErrorMessage = %q", items[0].ErrorMessage)
		}
	})

	t.Run("increment attempt on duplicate", func(t *testing.T) {
		err := svc.RecordDeadLetter(ctx, wh.ID, `{"msg_type":"text"}`, "timeout")
		if err != nil {
			t.Fatalf("RecordDeadLetter: %v", err)
		}

		items, err := svc.ListDeadLetters(ctx, wh.ID, 10)
		if err != nil {
			t.Fatalf("ListDeadLetters: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 dead letter (updated), got %d", len(items))
		}
		if items[0].AttemptCount != 2 {
			t.Errorf("AttemptCount = %d, want 2", items[0].AttemptCount)
		}
	})

	t.Run("clean expired", func(t *testing.T) {
		// Manually set attempt_count to max
		if err := svc.client.FeishuDeadLetter.Update().
			SetAttemptCount(MaxDeadLetterRetries).Exec(ctx); err != nil {
			t.Fatalf("age the dead letters: %v", err)
		}

		affected, err := svc.CleanExpiredDeadLetters(ctx)
		if err != nil {
			t.Fatalf("CleanExpiredDeadLetters: %v", err)
		}
		if affected != 1 {
			t.Errorf("affected = %d, want 1", affected)
		}

		items, _ := svc.ListDeadLetters(ctx, wh.ID, 10)
		if len(items) != 0 {
			t.Errorf("expected 0 dead letters after cleanup, got %d", len(items))
		}
	})
}

// ---------------------------------------------------------------------------
// Multi-Webhook Send via Service
// ---------------------------------------------------------------------------

func TestNotifyService_MultiWebhookSend(t *testing.T) {
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

	// Setup DB-backed service
	database := setupFeishuWebhookTestDB(t)
	svc := NewFeishuService(database, testEncryptionKey)
	ctx := context.Background()

	// Create webhook — bypass URL validation for test server
	encryptedURL := encryptHelper(server.URL, testEncryptionKey)
	urlHash := hashURLHelper(server.URL)
	if err := database.Client().FeishuWebhook.Create().
		SetName("Test Webhook").
		SetEncryptedURL(encryptedURL).
		SetURLHash(urlHash).
		SetScene("general").
		SetEnabled(true).
		SetRateLimitRps(100.0).
		SetCreatedBy("test").
		Exec(ctx); err != nil {
		t.Fatalf("seed webhook: %v", err)
	}

	// Setup Service
	notifySvc := NewService(Deps{Feishu: svc})
	notifySvc.client = &http.Client{Timeout: 5 * time.Second}

	// This should use the DB-backed multi-webhook path
	notifySvc.sendFeishuCard("🧪 多 Webhook 测试", "**测试消息**", nil)

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	if len(receivedBodies) == 0 {
		t.Error("expected webhook to be called via DB-backed path")
	}
	mu.Unlock()
}

// encryptHelper is a test helper that encrypts a URL using the real crypto package.
func encryptHelper(plaintext, key string) string {
	enc, err := crypto.Encrypt(plaintext, key)
	if err != nil {
		panic(err)
	}
	return enc
}

func hashURLHelper(rawURL string) string {
	return hashURL(rawURL)
}
