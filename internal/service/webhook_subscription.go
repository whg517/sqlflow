package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
)

// --- Sentinel Errors ---

var (
	ErrNotFound    = errors.New("订阅不存在")
	ErrValidation  = errors.New("参数校验失败")
	ErrEncryption  = errors.New("加密/解密失败")
)

// --- Constants ---

var AllowedWebhookEvents = map[string]bool{
	"ticket.created":  true,
	"ticket.approved": true,
	"ticket.rejected": true,
	"ticket.executed": true,
	"sla.warning":     true,
	"sla.breached":    true,
}

const (
	MaxConsecutiveFailures = 10
	WebhookMaxAttempts     = 3 // total attempts (not retries)
	WebhookTimeout         = 10 * time.Second
	MaxWebhookURLLength    = 2048
)

// --- Service ---

// WebhookSubscriptionService manages outbound webhook subscriptions.
type WebhookSubscriptionService struct {
	db     *sql.DB
	key    string       // AES encryption key
	client *http.Client // SSRF-safe HTTP client (reused across calls)
}

// NewWebhookSubscriptionService creates a new service instance.
func NewWebhookSubscriptionService(db *sql.DB, encryptionKey string) *WebhookSubscriptionService {
	return &WebhookSubscriptionService{
		db:     db,
		key:    encryptionKey,
		client: newSSRFSafeClient(),
	}
}

// --- Request/Response Types ---

type CreateSubscriptionRequest struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

type UpdateSubscriptionRequest struct {
	Name   *string  `json:"name,omitempty"`
	URL    *string  `json:"url,omitempty"`
	Events []string `json:"events,omitempty"`
}

type WebhookPayload struct {
	EventType string      `json:"event_type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// deliveryTarget holds data needed for webhook delivery (internal use).
type deliveryTarget struct {
	ID          int64
	URL         string
	PlainSecret string
	Events      []string
}

// --- Secret Generation ---

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成密钥失败: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// --- Validation ---

func validateWebhookURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("%w: webhook URL 不能为空", ErrValidation)
	}
	if len(rawURL) > MaxWebhookURLLength {
		return fmt.Errorf("%w: webhook URL 长度不能超过 %d 字符", ErrValidation, MaxWebhookURLLength)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: webhook URL 格式无效: %v", ErrValidation, err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" {
		if scheme != "http" {
			return fmt.Errorf("%w: webhook URL 必须使用 HTTPS 协议", ErrValidation)
		}
		host := parsed.Hostname()
		if host != "localhost" && host != "127.0.0.1" {
			return fmt.Errorf("%w: webhook URL 必须使用 HTTPS 协议（HTTP 仅限本地开发）", ErrValidation)
		}
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("%w: webhook URL 缺少主机名", ErrValidation)
	}

	if host == "localhost" || host == "127.0.0.1" {
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		log.Printf("webhook subscription: DNS lookup failed for %s: %v", host, err)
	} else {
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("%w: webhook URL 指向内网地址 %s，不允许的地址", ErrValidation, ip)
			}
		}
	}

	return nil
}

func validateEvents(events []string) error {
	if len(events) == 0 {
		return fmt.Errorf("%w: 至少需要订阅一个事件类型", ErrValidation)
	}
	for _, e := range events {
		if !AllowedWebhookEvents[e] {
			return fmt.Errorf("%w: 不支持的事件类型: %s", ErrValidation, e)
		}
	}
	return nil
}

// --- SSRF-safe HTTP Client ---

func newSSRFSafeClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("无效的地址: %s", addr)
			}

			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				return dialer.DialContext(ctx, network, addr)
			}

			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS 解析失败: %w", err)
			}

			for _, ipAddr := range ips {
				if isPrivateIP(ipAddr.IP) {
					return nil, fmt.Errorf("连接被拒绝: 目标 IP %s 是内网地址", ipAddr.IP)
				}
			}

			targetAddr := net.JoinHostPort(ips[0].IP.String(), port)
			return dialer.DialContext(ctx, network, targetAddr)
		},
	}
	return &http.Client{
		Timeout:   WebhookTimeout,
		Transport: transport,
	}
}

// --- CRUD ---

func (s *WebhookSubscriptionService) Create(ctx context.Context, req CreateSubscriptionRequest, createdBy string) (*model.WebhookSubscription, string, error) {
	if req.Name == "" {
		return nil, "", fmt.Errorf("%w: name 不能为空", ErrValidation)
	}
	if len(req.Name) > 100 {
		return nil, "", fmt.Errorf("%w: name 长度不能超过 100 字符", ErrValidation)
	}
	if err := validateWebhookURL(req.URL); err != nil {
		return nil, "", err
	}
	if err := validateEvents(req.Events); err != nil {
		return nil, "", err
	}

	plainSecret, err := generateSecret()
	if err != nil {
		return nil, "", err
	}

	encryptedSecret, err := crypto.Encrypt(plainSecret, s.key)
	if err != nil {
		return nil, "", fmt.Errorf("%w: 加密密钥失败: %v", ErrEncryption, err)
	}

	eventsJSON, _ := json.Marshal(req.Events)

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO webhook_subscriptions (name, url, encrypted_secret, events, enabled, failure_count, created_by)
		 VALUES (?, ?, ?, ?, 1, 0, ?)`,
		req.Name, req.URL, encryptedSecret, string(eventsJSON), createdBy,
	)
	if err != nil {
		return nil, "", fmt.Errorf("创建订阅失败: %w", err)
	}

	id, _ := result.LastInsertId()
	sub, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}

	s.writeAuditLog(ctx, "webhook_subscription.create", 0, createdBy, fmt.Sprintf("创建 Webhook 订阅: %s (ID:%d)", req.Name, id))

	return sub, plainSecret, nil
}

func (s *WebhookSubscriptionService) GetByID(ctx context.Context, id int64) (*model.WebhookSubscription, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, encrypted_secret, events, enabled, failure_count, last_triggered_at, created_by, created_at, updated_at
		 FROM webhook_subscriptions WHERE id = ?`, id)

	sub := &model.WebhookSubscription{}
	var lastTriggered sql.NullTime
	var enabled int

	err := row.Scan(&sub.ID, &sub.Name, &sub.URL, &sub.Secret, &sub.Events, &enabled,
		&sub.FailureCount, &lastTriggered, &sub.CreatedBy, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("查询订阅失败: %w", err)
	}

	sub.Enabled = enabled == 1
	if lastTriggered.Valid {
		sub.LastTriggeredAt = &lastTriggered.Time
	}
	sub.Secret = ""
	return sub, nil
}

func (s *WebhookSubscriptionService) List(ctx context.Context) ([]*model.WebhookSubscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, encrypted_secret, events, enabled, failure_count, last_triggered_at, created_by, created_at, updated_at
		 FROM webhook_subscriptions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}
	defer rows.Close()

	var subs []*model.WebhookSubscription
	for rows.Next() {
		sub := &model.WebhookSubscription{}
		var lastTriggered sql.NullTime
		var enabled int
		var encryptedSecret string

		if err := rows.Scan(&sub.ID, &sub.Name, &sub.URL, &encryptedSecret, &sub.Events, &enabled,
			&sub.FailureCount, &lastTriggered, &sub.CreatedBy, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描订阅数据失败: %w", err)
		}

		sub.Enabled = enabled == 1
		if lastTriggered.Valid {
			sub.LastTriggeredAt = &lastTriggered.Time
		}
		sub.Secret = ""
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// listForDelivery returns all enabled subscriptions with decrypted secrets in a single query pass.
// This avoids N+1 queries compared to List + per-item decryptSecret.
func (s *WebhookSubscriptionService) listForDelivery(ctx context.Context) ([]*deliveryTarget, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, url, encrypted_secret, events FROM webhook_subscriptions WHERE enabled = 1`)
	if err != nil {
		return nil, fmt.Errorf("查询订阅失败: %w", err)
	}
	defer rows.Close()

	var targets []*deliveryTarget
	for rows.Next() {
		var id int64
		var targetURL, encryptedSecret, eventsJSON string
		if err := rows.Scan(&id, &targetURL, &encryptedSecret, &eventsJSON); err != nil {
			return nil, fmt.Errorf("扫描订阅数据失败: %w", err)
		}

		plainSecret, err := crypto.Decrypt(encryptedSecret, s.key)
		if err != nil {
			log.Printf("webhook: failed to decrypt secret for subscription %d: %v", id, err)
			continue
		}

		var events []string
		if err := json.Unmarshal([]byte(eventsJSON), &events); err != nil {
			log.Printf("webhook: invalid events JSON for subscription %d: %v", id, err)
			continue
		}

		targets = append(targets, &deliveryTarget{
			ID:          id,
			URL:         targetURL,
			PlainSecret: plainSecret,
			Events:      events,
		})
	}
	return targets, rows.Err()
}

func (s *WebhookSubscriptionService) Update(ctx context.Context, id int64, req UpdateSubscriptionRequest, username string) (*model.WebhookSubscription, error) {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	name := existing.Name
	webURL := existing.URL
	events := existing.Events

	if req.Name != nil {
		if *req.Name == "" {
			return nil, fmt.Errorf("%w: name 不能为空", ErrValidation)
		}
		if len(*req.Name) > 100 {
			return nil, fmt.Errorf("%w: name 长度不能超过 100 字符", ErrValidation)
		}
		name = *req.Name
	}
	if req.URL != nil {
		if err := validateWebhookURL(*req.URL); err != nil {
			return nil, err
		}
		webURL = *req.URL
	}
	if req.Events != nil {
		if err := validateEvents(req.Events); err != nil {
			return nil, err
		}
		eventsJSON, _ := json.Marshal(req.Events)
		events = string(eventsJSON)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE webhook_subscriptions SET name = ?, url = ?, events = ?, updated_at = datetime('now') WHERE id = ?`,
		name, webURL, events, id)
	if err != nil {
		return nil, fmt.Errorf("更新订阅失败: %w", err)
	}

	s.writeAuditLog(ctx, "webhook_subscription.update", 0, username, fmt.Sprintf("更新 Webhook 订阅: %s (ID:%d)", name, id))

	return s.GetByID(ctx, id)
}

func (s *WebhookSubscriptionService) Delete(ctx context.Context, id int64, username string) error {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM webhook_subscriptions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除订阅失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}

	s.writeAuditLog(ctx, "webhook_subscription.delete", 0, username, fmt.Sprintf("删除 Webhook 订阅: %s (ID:%d)", existing.Name, id))

	return nil
}

func (s *WebhookSubscriptionService) Toggle(ctx context.Context, id int64, username string) (*model.WebhookSubscription, error) {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	newEnabled := !existing.Enabled
	// Atomic: reset failure_count on re-enable, keep on disable
	var failureCount int
	if newEnabled {
		failureCount = 0
	} else {
		failureCount = existing.FailureCount
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE webhook_subscriptions SET enabled = ?, failure_count = ?, updated_at = datetime('now') WHERE id = ?`,
		boolToInt(newEnabled), failureCount, id)
	if err != nil {
		return nil, fmt.Errorf("切换订阅状态失败: %w", err)
	}

	action := "禁用"
	if newEnabled {
		action = "启用"
	}
	s.writeAuditLog(ctx, "webhook_subscription.toggle", 0, username, fmt.Sprintf("%s Webhook 订阅: %s (ID:%d)", action, existing.Name, id))

	return s.GetByID(ctx, id)
}

// --- Test Send ---

func (s *WebhookSubscriptionService) TestSend(ctx context.Context, id int64) error {
	sub, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	plainSecret, err := s.decryptSecret(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: 解密密钥失败: %v", ErrEncryption, err)
	}

	payload := WebhookPayload{
		EventType: "test",
		Timestamp: time.Now().UTC(),
		Data: map[string]interface{}{
			"subscription_id":   sub.ID,
			"subscription_name": sub.Name,
			"message":           "This is a test webhook from SQLFlow",
		},
	}

	return s.sendWebhookRequest(sub.URL, plainSecret, payload)
}

// --- Event Delivery ---

func (s *WebhookSubscriptionService) DeliverEvent(ctx context.Context, eventType string, data interface{}) {
	targets, err := s.listForDelivery(ctx)
	if err != nil {
		log.Printf("webhook: failed to list subscriptions for delivery: %v", err)
		return
	}

	payload := WebhookPayload{
		EventType: eventType,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}

	for _, t := range targets {
		subscribed := false
		for _, e := range t.Events {
			if e == eventType {
				subscribed = true
				break
			}
		}
		if !subscribed {
			continue
		}

		go s.deliverWithRetry(t.ID, t.URL, t.PlainSecret, payload)
	}
}

func (s *WebhookSubscriptionService) deliverWithRetry(subID int64, targetURL, plainSecret string, payload WebhookPayload) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("webhook: panic in deliverWithRetry for subscription %d: %v", subID, r)
		}
	}()

	var lastErr error
	for attempt := 1; attempt <= WebhookMaxAttempts; attempt++ {
		if attempt > 1 {
			backoff := time.Duration(1<<uint(attempt-2)) * time.Second // 1s, 2s
			log.Printf("webhook: attempt %d/%d for subscription %d after %v", attempt, WebhookMaxAttempts, subID, backoff)
			time.Sleep(backoff)
		}

		lastErr = s.sendWebhookRequest(targetURL, plainSecret, payload)
		if lastErr == nil {
			s.handleSuccess(subID)
			return
		}
		log.Printf("webhook: delivery failed for subscription %d, attempt %d/%d: %v", subID, attempt, WebhookMaxAttempts, lastErr)
	}

	s.handleFailure(subID)
}

func (s *WebhookSubscriptionService) handleSuccess(subID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`UPDATE webhook_subscriptions SET failure_count = 0, last_triggered_at = datetime('now'), updated_at = datetime('now') WHERE id = ?`,
		subID)
	if err != nil {
		log.Printf("webhook: failed to update success for subscription %d: %v", subID, err)
	}
}

func (s *WebhookSubscriptionService) handleFailure(subID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Atomic: increment and check in single SQL statement
	var result struct {
		FailureCount int
		Enabled      int
	}
	err := s.db.QueryRowContext(ctx,
		`UPDATE webhook_subscriptions SET failure_count = failure_count + 1, updated_at = datetime('now') WHERE id = ?
		 RETURNING failure_count, enabled`, subID).Scan(&result.FailureCount, &result.Enabled)
	if err != nil {
		log.Printf("webhook: failed to increment failure for subscription %d: %v", subID, err)
		return
	}

	if result.FailureCount >= MaxConsecutiveFailures && result.Enabled == 1 {
		_, err = s.db.ExecContext(ctx,
			`UPDATE webhook_subscriptions SET enabled = 0, updated_at = datetime('now') WHERE id = ? AND enabled = 1`,
			subID)
		if err != nil {
			log.Printf("webhook: failed to auto-disable subscription %d: %v", subID, err)
			return
		}
		log.Printf("webhook: subscription %d auto-disabled after %d consecutive failures", subID, result.FailureCount)
	}
}

// decryptSecret decrypts the stored encrypted secret for a subscription.
func (s *WebhookSubscriptionService) decryptSecret(ctx context.Context, id int64) (string, error) {
	var encryptedSecret string
	err := s.db.QueryRowContext(ctx,
		`SELECT encrypted_secret FROM webhook_subscriptions WHERE id = ?`, id).Scan(&encryptedSecret)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("查询加密密钥失败: %w", err)
	}
	plain, err := crypto.Decrypt(encryptedSecret, s.key)
	if err != nil {
		return "", fmt.Errorf("%w: 解密失败: %v", ErrEncryption, err)
	}
	return plain, nil
}

// --- Webhook Sending (SSRF-safe, reuses struct client) ---

func (s *WebhookSubscriptionService) sendWebhookRequest(targetURL, secret string, payload WebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 payload 失败: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest("POST", targetURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SQLFlow-Signature", "sha256="+signature)
	req.Header.Set("X-SQLFlow-Event", payload.EventType)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook 返回非 2xx 状态码: %d", resp.StatusCode)
	}

	return nil
}

// --- Audit Logging ---

func (s *WebhookSubscriptionService) writeAuditLog(ctx context.Context, action string, userID int64, username, details string) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_logs (user_id, action, ip_address, error_message, created_at) VALUES (?, ?, '', ?, datetime('now'))`,
		userID, action, fmt.Sprintf("%s — %s", username, details))
	if err != nil {
		log.Printf("webhook: failed to write audit log: %v", err)
	}
}

// --- Utility ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func maskWebhookURL(rawURL string) string {
	if len(rawURL) <= 12 {
		return "****"
	}
	return rawURL[:8] + "****" + rawURL[len(rawURL)-4:]
}

func ParseWebhookID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// FormatSubscriptionForResponse formats a subscription for API response.
// Secret is never included (only returned once on creation).
func FormatSubscriptionForResponse(sub *model.WebhookSubscription) map[string]interface{} {
	return map[string]interface{}{
		"id":                sub.ID,
		"name":              sub.Name,
		"url":               maskWebhookURL(sub.URL),
		"events":            json.RawMessage(sub.Events),
		"enabled":           sub.Enabled,
		"failure_count":     sub.FailureCount,
		"last_triggered_at": sub.LastTriggeredAt,
		"created_by":        sub.CreatedBy,
		"created_at":        sub.CreatedAt,
		"updated_at":        sub.UpdatedAt,
	}
}

// IsNotFoundError checks if an error is ErrNotFound.
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsValidationError checks if an error is ErrValidation.
func IsValidationError(err error) bool {
	return errors.Is(err, ErrValidation)
}
