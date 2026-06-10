package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/pkg/crypto"
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
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// FeishuWebhookCreateRequest is the input for creating a new webhook.
type FeishuWebhookCreateRequest struct {
	Name         string  `json:"name"`
	WebhookURL   string  `json:"webhook_url"`
	Scene        string  `json:"scene"`
	RateLimitRPS float64 `json:"rate_limit_rps"`
}

// FeishuWebhookUpdateRequest is the input for updating an existing webhook.
type FeishuWebhookUpdateRequest struct {
	Name         *string  `json:"name,omitempty"`
	WebhookURL   *string  `json:"webhook_url,omitempty"`
	Scene        *string  `json:"scene,omitempty"`
	Enabled      *bool    `json:"enabled,omitempty"`
	RateLimitRPS *float64 `json:"rate_limit_rps,omitempty"`
}

// FeishuDeadLetter represents a failed notification queued for retry.
type FeishuDeadLetter struct {
	ID            int64  `json:"id"`
	WebhookID     int64  `json:"webhook_id"`
	Payload       string `json:"payload"`
	ErrorMessage  string `json:"error_message"`
	AttemptCount  int    `json:"attempt_count"`
	LastAttemptAt string `json:"last_attempt_at"`
	CreatedAt     string `json:"created_at"`
}

const (
	// AllowedFeishuURLPrefix is the only permitted webhook URL prefix.
	AllowedFeishuURLPrefix = "https://open.feishu.cn/open-apis/bot/v2/hook/"

	// MaxDeadLetterRetries is the maximum number of retry attempts before giving up.
	MaxDeadLetterRetries = 5
)

// FeishuWebhookService manages Feishu webhook CRUD, encryption, and SSRF protection.
type FeishuWebhookService struct {
	db  *sql.DB
	key string // AES encryption key (same as config.EncryptionKey)

	// In-memory rate limiters per webhook ID
	mu       sync.Mutex
	limiters map[int64]*rateLimiter
}

type rateLimiter struct {
	lastSent time.Time
	interval time.Duration // minimum interval between sends
}

// NewFeishuWebhookService creates a new service instance.
func NewFeishuWebhookService(db *sql.DB, encryptionKey string) *FeishuWebhookService {
	return &FeishuWebhookService{
		db:       db,
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
func (s *FeishuWebhookService) Create(ctx context.Context, req FeishuWebhookCreateRequest, createdBy string) (*FeishuWebhook, error) {
	// Validate URL
	if err := ValidateWebhookURL(req.WebhookURL); err != nil {
		return nil, fmt.Errorf("URL 校验失败: %w", err)
	}

	// Check duplicate by URL hash
	urlHash := hashURL(req.WebhookURL)
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM feishu_webhooks WHERE url_hash = ?`, urlHash,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("检查重复 URL: %w", err)
	}
	if count > 0 {
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

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO feishu_webhooks (name, encrypted_url, url_hash, scene, enabled, rate_limit_rps, created_by)
		 VALUES (?, ?, ?, ?, 1, ?, ?)`,
		req.Name, encryptedURL, urlHash, req.Scene, req.RateLimitRPS, createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("插入记录失败: %w", err)
	}

	id, _ := result.LastInsertId()
	return s.GetByID(ctx, id)
}

// GetByID returns a webhook by ID.
func (s *FeishuWebhookService) GetByID(ctx context.Context, id int64) (*FeishuWebhook, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, encrypted_url, url_hash, scene, enabled, rate_limit_rps, created_by, created_at, updated_at
		 FROM feishu_webhooks WHERE id = ?`, id,
	)

	var wh FeishuWebhook
	var enabled int
	if err := row.Scan(&wh.ID, &wh.Name, &wh.EncryptedURL, &wh.URLHash, &wh.Scene, &enabled, &wh.RateLimitRPS, &wh.CreatedBy, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("webhook 不存在 (id=%d)", id)
		}
		return nil, err
	}
	wh.Enabled = enabled == 1
	return &wh, nil
}

// List returns all webhooks (URLs masked for non-admin views).
func (s *FeishuWebhookService) List(ctx context.Context, showFullURL bool) ([]map[string]interface{}, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, encrypted_url, scene, enabled, rate_limit_rps, created_by, created_at, updated_at
		 FROM feishu_webhooks ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var name, encryptedURL, scene, createdBy, createdAt, updatedAt string
		var enabled int
		var rateLimitRPS float64

		if err := rows.Scan(&id, &name, &encryptedURL, &scene, &enabled, &rateLimitRPS, &createdBy, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		// Decrypt URL for potential display
		decryptedURL, err := crypto.Decrypt(encryptedURL, s.key)
		if err != nil {
			log.Printf("feishu webhook: decrypt url id=%d: %v", id, err)
			decryptedURL = "[解密失败]"
		}

		displayURL := MaskURL(decryptedURL)
		if showFullURL {
			displayURL = decryptedURL
		}

		results = append(results, map[string]interface{}{
			"id":             id,
			"name":           name,
			"webhook_url":    displayURL,
			"scene":          scene,
			"enabled":        enabled == 1,
			"rate_limit_rps": rateLimitRPS,
			"created_by":     createdBy,
			"created_at":     createdAt,
			"updated_at":     updatedAt,
		})
	}
	return results, nil
}

// Update modifies an existing webhook.
func (s *FeishuWebhookService) Update(ctx context.Context, id int64, req FeishuWebhookUpdateRequest) (*FeishuWebhook, error) {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	sets := []string{}
	args := []interface{}{}

	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}

	if req.WebhookURL != nil {
		if err := ValidateWebhookURL(*req.WebhookURL); err != nil {
			return nil, fmt.Errorf("URL 校验失败: %w", err)
		}
		urlHash := hashURL(*req.WebhookURL)
		// Check duplicate (excluding self)
		var count int
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM feishu_webhooks WHERE url_hash = ? AND id != ?`, urlHash, id,
		).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("检查重复 URL: %w", err)
		}
		if count > 0 {
			return nil, fmt.Errorf("该 Webhook URL 已被其他配置使用")
		}
		encryptedURL, err := crypto.Encrypt(*req.WebhookURL, s.key)
		if err != nil {
			return nil, fmt.Errorf("加密 Webhook URL 失败: %w", err)
		}
		sets = append(sets, "encrypted_url = ?", "url_hash = ?")
		args = append(args, encryptedURL, urlHash)
		_ = existing // suppress unused
	}

	if req.Scene != nil {
		sets = append(sets, "scene = ?")
		args = append(args, *req.Scene)
	}

	if req.Enabled != nil {
		enabledVal := 0
		if *req.Enabled {
			enabledVal = 1
		}
		sets = append(sets, "enabled = ?")
		args = append(args, enabledVal)
	}

	if req.RateLimitRPS != nil {
		sets = append(sets, "rate_limit_rps = ?")
		args = append(args, *req.RateLimitRPS)
	}

	if len(sets) == 0 {
		return existing, nil
	}

	sets = append(sets, "updated_at = datetime('now')")
	args = append(args, id)

	query := "UPDATE feishu_webhooks SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("更新失败: %w", err)
	}

	return s.GetByID(ctx, id)
}

// Delete removes a webhook and its dead letter entries.
func (s *FeishuWebhookService) Delete(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM feishu_dead_letters WHERE webhook_id = ?`, id); err != nil {
		log.Printf("feishu webhook: delete dead letters for webhook %d: %v", id, err)
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM feishu_webhooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("webhook 不存在 (id=%d)", id)
	}

	// Clean up rate limiter
	s.mu.Lock()
	delete(s.limiters, id)
	s.mu.Unlock()

	return nil
}

// DecryptURL decrypts the stored URL for a webhook (admin only).
func (s *FeishuWebhookService) DecryptURL(encryptedURL string) (string, error) {
	return crypto.Decrypt(encryptedURL, s.key)
}

// GetEnabledWebhooks loads all enabled webhooks with decrypted URLs.
func (s *FeishuWebhookService) GetEnabledWebhooks(ctx context.Context) ([]struct {
	ID  int64
	URL string
}, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, encrypted_url FROM feishu_webhooks WHERE enabled = 1`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct {
		ID  int64
		URL string
	}

	for rows.Next() {
		var id int64
		var encryptedURL string
		if err := rows.Scan(&id, &encryptedURL); err != nil {
			return nil, err
		}

		decryptedURL, err := crypto.Decrypt(encryptedURL, s.key)
		if err != nil {
			log.Printf("feishu webhook: decrypt url id=%d: %v", id, err)
			continue
		}

		results = append(results, struct {
			ID  int64
			URL string
		}{ID: id, URL: decryptedURL})
	}
	return results, nil
}

// CheckRateLimit returns true if the webhook is allowed to send (rate limit not exceeded).
// If allowed, it records the send time.
func (s *FeishuWebhookService) CheckRateLimit(webhookID int64, rps float64) bool {
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
func (s *FeishuWebhookService) RecordDeadLetter(ctx context.Context, webhookID int64, payload, errMsg string) error {
	// Check if there's an existing entry for this payload
	var existingID int64
	var attemptCount int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, attempt_count FROM feishu_dead_letters WHERE webhook_id = ? AND payload = ?`,
		webhookID, payload,
	).Scan(&existingID, &attemptCount)

	if err == sql.ErrNoRows {
		// New dead letter
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO feishu_dead_letters (webhook_id, payload, error_message, attempt_count, last_attempt_at)
			 VALUES (?, ?, ?, 1, datetime('now'))`,
			webhookID, payload, errMsg,
		)
		return err
	}

	if err != nil {
		return err
	}

	// Update existing
	_, err = s.db.ExecContext(ctx,
		`UPDATE feishu_dead_letters SET attempt_count = ?, error_message = ?, last_attempt_at = datetime('now')
		 WHERE id = ?`,
		attemptCount+1, errMsg, existingID,
	)
	return err
}

// ListDeadLetters returns dead letter entries, optionally filtered by webhook ID.
func (s *FeishuWebhookService) ListDeadLetters(ctx context.Context, webhookID int64, limit int) ([]FeishuDeadLetter, error) {
	if limit <= 0 {
		limit = 50
	}

	var rows *sql.Rows
	var err error

	if webhookID > 0 {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, webhook_id, payload, error_message, attempt_count, last_attempt_at, created_at
			 FROM feishu_dead_letters WHERE webhook_id = ? ORDER BY created_at DESC LIMIT ?`,
			webhookID, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, webhook_id, payload, error_message, attempt_count, last_attempt_at, created_at
			 FROM feishu_dead_letters ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FeishuDeadLetter
	for rows.Next() {
		var dl FeishuDeadLetter
		if err := rows.Scan(&dl.ID, &dl.WebhookID, &dl.Payload, &dl.ErrorMessage, &dl.AttemptCount, &dl.LastAttemptAt, &dl.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, dl)
	}
	return results, nil
}

// CleanExpiredDeadLetters removes dead letters that have exceeded max retries.
func (s *FeishuWebhookService) CleanExpiredDeadLetters(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM feishu_dead_letters WHERE attempt_count >= ?`,
		MaxDeadLetterRetries,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
