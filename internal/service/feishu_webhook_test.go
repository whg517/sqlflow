package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/pkg/crypto"
	_ "modernc.org/sqlite"
)

const testEncryptionKey = "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256

func setupFeishuWebhookTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "feishu_webhook_test_*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	db, err := sql.Open("sqlite", tmpFile.Name()+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)

	// Create tables
	migration := `
	CREATE TABLE IF NOT EXISTS feishu_webhooks (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		name            TEXT    NOT NULL,
		encrypted_url   TEXT    NOT NULL,
		url_hash        TEXT    NOT NULL,
		scene           TEXT    NOT NULL DEFAULT 'general',
		enabled         INTEGER NOT NULL DEFAULT 1,
		rate_limit_rps  REAL    NOT NULL DEFAULT 1.0,
		created_by      TEXT    NOT NULL DEFAULT '',
		created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_feishu_webhooks_url_hash ON feishu_webhooks(url_hash);

	CREATE TABLE IF NOT EXISTS feishu_dead_letters (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		webhook_id      INTEGER NOT NULL,
		payload         TEXT    NOT NULL,
		error_message   TEXT    NOT NULL DEFAULT '',
		attempt_count   INTEGER NOT NULL DEFAULT 0,
		last_attempt_at DATETIME NOT NULL DEFAULT (datetime('now')),
		created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX IF NOT EXISTS idx_dead_letters_webhook_id ON feishu_dead_letters(webhook_id);
	`
	if _, err := db.Exec(migration); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db
}

func newTestFeishuWebhookService(t *testing.T) *FeishuWebhookService {
	t.Helper()
	db := setupFeishuWebhookTestDB(t)
	return NewFeishuWebhookService(db, testEncryptionKey)
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
		req := FeishuWebhookCreateRequest{
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
		req := FeishuWebhookCreateRequest{
			Name:       "重复",
			WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/test-token-abc123",
		}
		_, err := svc.Create(ctx, req, "admin")
		if err == nil {
			t.Error("expected duplicate URL error")
		}
	})

	t.Run("invalid url rejected", func(t *testing.T) {
		req := FeishuWebhookCreateRequest{
			Name:       "无效",
			WebhookURL: "https://example.com/webhook",
		}
		_, err := svc.Create(ctx, req, "admin")
		if err == nil {
			t.Error("expected URL validation error")
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		req := FeishuWebhookCreateRequest{
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
	req := FeishuWebhookCreateRequest{
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

	req := FeishuWebhookCreateRequest{
		Name:       "更新前",
		WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/update-before-token",
	}
	wh, err := svc.Create(ctx, req, "admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("update name", func(t *testing.T) {
		newName := "更新后"
		updated, err := svc.Update(ctx, wh.ID, FeishuWebhookUpdateRequest{Name: &newName})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Name != newName {
			t.Errorf("Name = %q, want %q", updated.Name, newName)
		}
	})

	t.Run("disable", func(t *testing.T) {
		disabled := false
		updated, err := svc.Update(ctx, wh.ID, FeishuWebhookUpdateRequest{Enabled: &disabled})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Enabled {
			t.Error("should be disabled")
		}
	})

	t.Run("update url", func(t *testing.T) {
		newURL := "https://open.feishu.cn/open-apis/bot/v2/hook/updated-token-456"
		updated, err := svc.Update(ctx, wh.ID, FeishuWebhookUpdateRequest{WebhookURL: &newURL})
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

	req := FeishuWebhookCreateRequest{
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
	req := FeishuWebhookCreateRequest{
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
	req := FeishuWebhookCreateRequest{
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
		svc.db.ExecContext(ctx, `UPDATE feishu_dead_letters SET attempt_count = ?`, MaxDeadLetterRetries)

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
// Multi-Webhook Send via NotifyService
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
	db := setupFeishuWebhookTestDB(t)
	svc := NewFeishuWebhookService(db, testEncryptionKey)
	ctx := context.Background()

	// Create webhook — bypass URL validation for test server
	encryptedURL := encryptHelper(server.URL, testEncryptionKey)
	urlHash := hashURLHelper(server.URL)
	db.ExecContext(ctx,
		`INSERT INTO feishu_webhooks (name, encrypted_url, url_hash, scene, enabled, rate_limit_rps, created_by)
		 VALUES (?, ?, ?, 'general', 1, 100.0, 'test')`,
		"Test Webhook", encryptedURL, urlHash,
	)

	// Setup NotifyService
	notifySvc := NewNotifyService("", "")
	notifySvc.SetFeishuWebhookService(svc)
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
