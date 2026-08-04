package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entfeishudeadletter "github.com/whg517/sqlflow/internal/db/ent/feishudeadletter"
	entfeishuwebhook "github.com/whg517/sqlflow/internal/db/ent/feishuwebhook"
	"github.com/whg517/sqlflow/internal/platform/crypto"
)

// FeishuWebhook represents a stored Feishu webhook configuration.
type FeishuWebhook struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	EncryptedURL string  `json:"-"` // never exposed in JSON
	URLHash      string  `json:"-"` // never exposed in JSON
	Scene        string  `json:"scene"`
	Enabled      bool    `json:"enabled"`
	RateLimitRPS float64 `json:"rate_limit_rps"`
	CreatedBy    string  `json:"created_by"`
	// time.Time rather than the driver's text, matching every other timestamp
	// the API returns.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FeishuCreateRequest is the input for creating a new webhook.
type FeishuCreateRequest struct {
	Name         string  `json:"name"`
	WebhookURL   string  `json:"webhook_url"`
	Scene        string  `json:"scene"`
	RateLimitRPS float64 `json:"rate_limit_rps"`
}

// FeishuUpdateRequest is the input for updating an existing webhook.
type FeishuUpdateRequest struct {
	Name         *string  `json:"name,omitempty"`
	WebhookURL   *string  `json:"webhook_url,omitempty"`
	Scene        *string  `json:"scene,omitempty"`
	Enabled      *bool    `json:"enabled,omitempty"`
	RateLimitRPS *float64 `json:"rate_limit_rps,omitempty"`
}

// FeishuDeadLetter represents a failed notification queued for retry.
type FeishuDeadLetter struct {
	ID            int64     `json:"id"`
	WebhookID     int64     `json:"webhook_id"`
	Payload       string    `json:"payload"`
	ErrorMessage  string    `json:"error_message"`
	AttemptCount  int64     `json:"attempt_count"`
	LastAttemptAt time.Time `json:"last_attempt_at"`
	CreatedAt     time.Time `json:"created_at"`
}

const (
	// AllowedFeishuURLPrefix is the only permitted webhook URL prefix.
	AllowedFeishuURLPrefix = "https://open.feishu.cn/open-apis/bot/v2/hook/"

	// MaxDeadLetterRetries is the maximum number of retry attempts before giving up.
	MaxDeadLetterRetries = 5
)

// FeishuService manages Feishu webhook CRUD, encryption, and SSRF protection.
type FeishuService struct {
	client *ent.Client
	key    string // AES encryption key (same as config.EncryptionKey)

	// In-memory rate limiters per webhook ID
	mu       sync.Mutex
	limiters map[int64]*rateLimiter
}

type rateLimiter struct {
	lastSent time.Time
	interval time.Duration // minimum interval between sends
}

// NewFeishuService creates a new service instance.
func NewFeishuService(database *db.DB, encryptionKey string) *FeishuService {
	return &FeishuService{
		client:   database.Client(),
		key:      encryptionKey,
		limiters: make(map[int64]*rateLimiter),
	}
}

// ValidateWebhookURL checks that the URL is a valid Feishu webhook URL and not an internal address.
func ValidateWebhookURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("webhook URL 不能为空")
	}

	if !strings.HasPrefix(rawURL, AllowedFeishuURLPrefix) {
		return fmt.Errorf("webhook URL 必须以 %s 开头", AllowedFeishuURLPrefix)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("webhook URL 格式无效: %w", err)
	}

	// Resolve host to check for internal/private IPs (SSRF prevention)
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL 缺少主机名")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// If we can't resolve, let it through — DNS resolution might work in production
		log.Printf("feishu webhook: DNS lookup failed for %s: %v", host, err)
	} else {
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("webhook URL 指向内网地址 %s，不允许的地址", ip)
			}
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is a private/reserved address.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network *net.IPNet
	}{
		{mustParseCIDR("10.0.0.0/8")},
		{mustParseCIDR("172.16.0.0/12")},
		{mustParseCIDR("192.168.0.0/16")},
		{mustParseCIDR("127.0.0.0/8")},
		{mustParseCIDR("169.254.0.0/16")},
		{mustParseCIDR("::1/128")},
		{mustParseCIDR("fc00::/7")},
		{mustParseCIDR("fe80::/10")},
	}
	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

func mustParseCIDR(s string) *net.IPNet {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return network
}

// hashURL returns a SHA-256 hash of the URL for deduplication.
func hashURL(rawURL string) string {
	h := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(h[:])
}

// MaskURL masks the webhook URL for display (only show last 8 chars of token).
func MaskURL(rawURL string) string {
	if len(rawURL) <= len(AllowedFeishuURLPrefix)+8 {
		return AllowedFeishuURLPrefix + "****"
	}
	return rawURL[:len(AllowedFeishuURLPrefix)] + "****" + rawURL[len(rawURL)-4:]
}

// Create stores a new encrypted Feishu webhook.
func (s *FeishuService) Create(ctx context.Context, req FeishuCreateRequest, createdBy string) (*FeishuWebhook, error) {
	// Validate URL
	if err := ValidateWebhookURL(req.WebhookURL); err != nil {
		return nil, fmt.Errorf("URL 校验失败: %w", err)
	}

	// Check duplicate by URL hash
	urlHash := hashURL(req.WebhookURL)
	exists, err := s.client.FeishuWebhook.Query().
		Where(entfeishuwebhook.URLHashEQ(urlHash)).
		Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("检查重复 URL: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("该 Webhook URL 已存在")
	}

	// Encrypt the URL
	encryptedURL, err := crypto.Encrypt(req.WebhookURL, s.key)
	if err != nil {
		return nil, fmt.Errorf("加密 Webhook URL 失败: %w", err)
	}

	// Defaults
	if req.Scene == "" {
		req.Scene = "general"
	}
	if req.RateLimitRPS <= 0 {
		req.RateLimitRPS = 1.0
	}

	created, err := s.client.FeishuWebhook.Create().
		SetName(req.Name).
		SetEncryptedURL(encryptedURL).
		SetURLHash(urlHash).
		SetScene(req.Scene).
		SetEnabled(true).
		SetRateLimitRps(req.RateLimitRPS).
		SetCreatedBy(createdBy).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("插入记录失败: %w", err)
	}
	return entFeishuWebhookToModel(created), nil
}

// GetByID returns a webhook by ID.
func (s *FeishuService) GetByID(ctx context.Context, id int64) (*FeishuWebhook, error) {
	wh, err := s.client.FeishuWebhook.Get(ctx, int(id))
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("webhook 不存在 (id=%d)", id)
	}
	if err != nil {
		return nil, err
	}
	return entFeishuWebhookToModel(wh), nil
}

// entFeishuWebhookToModel converts a generated webhook to the service shape.
func entFeishuWebhookToModel(w *ent.FeishuWebhook) *FeishuWebhook {
	return &FeishuWebhook{
		ID:           int64(w.ID),
		Name:         w.Name,
		EncryptedURL: w.EncryptedURL,
		URLHash:      w.URLHash,
		Scene:        w.Scene,
		Enabled:      w.Enabled,
		RateLimitRPS: w.RateLimitRps,
		CreatedBy:    w.CreatedBy,
		CreatedAt:    w.CreatedAt,
		UpdatedAt:    w.UpdatedAt,
	}
}

// List returns all webhooks (URLs masked for non-admin views).
func (s *FeishuService) List(ctx context.Context, showFullURL bool) ([]map[string]interface{}, error) {
	rows, err := s.client.FeishuWebhook.Query().
		Order(ent.Asc(entfeishuwebhook.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0, len(rows))
	for _, w := range rows {
		// A webhook whose URL will not decrypt is still listed: hiding it would
		// leave an entry the operator cannot see or delete.
		decryptedURL, err := crypto.Decrypt(w.EncryptedURL, s.key)
		if err != nil {
			log.Printf("feishu webhook: decrypt url id=%d: %v", w.ID, err)
			decryptedURL = "[解密失败]"
		}

		displayURL := MaskURL(decryptedURL)
		if showFullURL {
			displayURL = decryptedURL
		}

		results = append(results, map[string]interface{}{
			"id":             int64(w.ID),
			"name":           w.Name,
			"webhook_url":    displayURL,
			"scene":          w.Scene,
			"enabled":        w.Enabled,
			"rate_limit_rps": w.RateLimitRps,
			"created_by":     w.CreatedBy,
			"created_at":     w.CreatedAt,
			"updated_at":     w.UpdatedAt,
		})
	}
	return results, nil
}

// Update modifies an existing webhook.
func (s *FeishuService) Update(ctx context.Context, id int64, req FeishuUpdateRequest) (*FeishuWebhook, error) {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// The builder replaces a slice of "column = ?" fragments whose placeholders
	// were numbered after assembly. Only fields the request set are touched, so
	// a partial update still cannot blank the rest.
	upd := s.client.FeishuWebhook.UpdateOneID(int(id))
	changed := false

	if req.Name != nil {
		upd = upd.SetName(*req.Name)
		changed = true
	}
	if req.WebhookURL != nil {
		if err := ValidateWebhookURL(*req.WebhookURL); err != nil {
			return nil, fmt.Errorf("URL 校验失败: %w", err)
		}
		urlHash := hashURL(*req.WebhookURL)
		// Excluding self: re-saving a webhook without changing its URL must not
		// collide with its own row.
		taken, err := s.client.FeishuWebhook.Query().
			Where(
				entfeishuwebhook.URLHashEQ(urlHash),
				entfeishuwebhook.IDNEQ(int(id)),
			).
			Exist(ctx)
		if err != nil {
			return nil, fmt.Errorf("检查重复 URL: %w", err)
		}
		if taken {
			return nil, fmt.Errorf("该 Webhook URL 已被其他配置使用")
		}
		encryptedURL, err := crypto.Encrypt(*req.WebhookURL, s.key)
		if err != nil {
			return nil, fmt.Errorf("加密 Webhook URL 失败: %w", err)
		}
		upd = upd.SetEncryptedURL(encryptedURL).SetURLHash(urlHash)
		changed = true
	}
	if req.Scene != nil {
		upd = upd.SetScene(*req.Scene)
		changed = true
	}
	if req.Enabled != nil {
		upd = upd.SetEnabled(*req.Enabled)
		changed = true
	}
	if req.RateLimitRPS != nil {
		upd = upd.SetRateLimitRps(*req.RateLimitRPS)
		changed = true
	}

	if !changed {
		return existing, nil
	}

	updated, err := upd.SetUpdatedAt(time.Now()).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("更新失败: %w", err)
	}
	return entFeishuWebhookToModel(updated), nil
}

// Delete removes a webhook and its dead letter entries.
func (s *FeishuService) Delete(ctx context.Context, id int64) error {
	// Dead letters first: they reference the webhook, and leaving them would
	// keep a retry queue pointing at a configuration that no longer exists.
	if _, err := s.client.FeishuDeadLetter.Delete().
		Where(entfeishudeadletter.WebhookIDEQ(id)).
		Exec(ctx); err != nil {
		log.Printf("feishu webhook: delete dead letters for webhook %d: %v", id, err)
	}

	err := s.client.FeishuWebhook.DeleteOneID(int(id)).Exec(ctx)
	if ent.IsNotFound(err) {
		return fmt.Errorf("webhook 不存在 (id=%d)", id)
	}
	if err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}

	// Clean up rate limiter
	s.mu.Lock()
	delete(s.limiters, id)
	s.mu.Unlock()

	return nil
}

// DecryptURL decrypts the stored URL for a webhook (admin only).
func (s *FeishuService) DecryptURL(encryptedURL string) (string, error) {
	return crypto.Decrypt(encryptedURL, s.key)
}

// GetEnabledWebhooks loads all enabled webhooks with decrypted URLs.
func (s *FeishuService) GetEnabledWebhooks(ctx context.Context) ([]struct {
	ID  int64
	URL string
}, error) {
	rows, err := s.client.FeishuWebhook.Query().
		Where(entfeishuwebhook.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]struct {
		ID  int64
		URL string
	}, 0, len(rows))
	for _, w := range rows {
		decryptedURL, err := crypto.Decrypt(w.EncryptedURL, s.key)
		if err != nil {
			// Skipped rather than failed: one unreadable webhook must not stop
			// the notification reaching every other destination.
			log.Printf("feishu webhook: decrypt url id=%d: %v", w.ID, err)
			continue
		}
		results = append(results, struct {
			ID  int64
			URL string
		}{ID: int64(w.ID), URL: decryptedURL})
	}
	return results, nil
}

// CheckRateLimit returns true if the webhook is allowed to send (rate limit not exceeded).
// If allowed, it records the send time.
func (s *FeishuService) CheckRateLimit(webhookID int64, rps float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	rl, ok := s.limiters[webhookID]
	if !ok {
		interval := time.Duration(float64(time.Second) / rps)
		if interval < time.Second {
			interval = time.Second // minimum 1s between sends
		}
		rl = &rateLimiter{interval: interval}
		s.limiters[webhookID] = rl
	}

	now := time.Now()
	if now.Sub(rl.lastSent) < rl.interval {
		return false
	}
	rl.lastSent = now
	return true
}

// RecordDeadLetter stores a failed notification for later retry.
func (s *FeishuService) RecordDeadLetter(ctx context.Context, webhookID int64, payload, errMsg string) error {
	// Read then write, because (webhook_id, payload) carries no unique
	// constraint. Two workers failing the same notification at once can
	// therefore create two rows, which shows up as a duplicated retry. Closing
	// that needs a schema change rather than a different query here.
	existing, err := s.client.FeishuDeadLetter.Query().
		Where(
			entfeishudeadletter.WebhookIDEQ(webhookID),
			entfeishudeadletter.PayloadEQ(payload),
		).
		First(ctx)
	if ent.IsNotFound(err) {
		return s.client.FeishuDeadLetter.Create().
			SetWebhookID(webhookID).
			SetPayload(payload).
			SetErrorMessage(errMsg).
			SetAttemptCount(1).
			SetLastAttemptAt(time.Now()).
			Exec(ctx)
	}
	if err != nil {
		return err
	}

	// AddAttemptCount is an atomic increment, so two workers updating the same
	// row do not lose a count between them.
	return s.client.FeishuDeadLetter.UpdateOneID(existing.ID).
		AddAttemptCount(1).
		SetErrorMessage(errMsg).
		SetLastAttemptAt(time.Now()).
		Exec(ctx)
}

// ListDeadLetters returns dead letter entries, optionally filtered by webhook ID.
func (s *FeishuService) ListDeadLetters(ctx context.Context, webhookID int64, limit int) ([]FeishuDeadLetter, error) {
	if limit <= 0 {
		limit = 50
	}

	q := s.client.FeishuDeadLetter.Query()
	if webhookID > 0 {
		q = q.Where(entfeishudeadletter.WebhookIDEQ(webhookID))
	}
	rows, err := q.
		Order(ent.Desc(entfeishudeadletter.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]FeishuDeadLetter, 0, len(rows))
	for _, dl := range rows {
		results = append(results, FeishuDeadLetter{
			ID:            int64(dl.ID),
			WebhookID:     dl.WebhookID,
			Payload:       dl.Payload,
			ErrorMessage:  dl.ErrorMessage,
			AttemptCount:  dl.AttemptCount,
			LastAttemptAt: dl.LastAttemptAt,
			CreatedAt:     dl.CreatedAt,
		})
	}
	return results, nil
}

// CleanExpiredDeadLetters removes dead letters that have exceeded max retries.
func (s *FeishuService) CleanExpiredDeadLetters(ctx context.Context) (int64, error) {
	n, err := s.client.FeishuDeadLetter.Delete().
		Where(entfeishudeadletter.AttemptCountGTE(MaxDeadLetterRetries)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}
