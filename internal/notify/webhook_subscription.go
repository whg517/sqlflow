package notify

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entwebhooksubscription "github.com/whg517/sqlflow/internal/db/ent/webhooksubscription"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/crypto"
)

// --- Sentinel Errors ---

var (
	ErrNotFound   = errors.New("订阅不存在")
	ErrValidation = errors.New("参数校验失败")
	ErrEncryption = errors.New("加密/解密失败")
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

// --- datasource.Service ---

// WebhookSubscriptionService manages outbound webhook subscriptions.
type WebhookSubscriptionService struct {
	// entClient reads and writes subscriptions; client posts to them. The names
	// are kept distinct because both are "the client" in their own layer.
	entClient *ent.Client
	key       string       // AES encryption key
	client    *http.Client // SSRF-safe HTTP client (reused across calls)
}

// NewWebhookSubscriptionService creates a new service instance.
func NewWebhookSubscriptionService(database *db.DB, encryptionKey string) *WebhookSubscriptionService {
	return &WebhookSubscriptionService{
		entClient: database.Client(),
		key:       encryptionKey,
		client:    newSSRFSafeClient(),
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

func (s *WebhookSubscriptionService) Create(ctx context.Context, req CreateSubscriptionRequest, actorID int64, actorName string) (*model.WebhookSubscription, string, error) {
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

	created, err := s.entClient.WebhookSubscription.Create().
		SetName(req.Name).
		SetURL(req.URL).
		SetEncryptedSecret(encryptedSecret).
		SetEvents(string(eventsJSON)).
		SetEnabled(true).
		SetFailureCount(0).
		SetCreatedBy(actorName).
		Save(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("创建订阅失败: %w", err)
	}
	id := int64(created.ID)
	sub := entWebhookSubscriptionToModel(created)

	s.writeAuditLog(ctx, "webhook_subscription.create", actorID, actorName, fmt.Sprintf("创建 Webhook 订阅: %s (ID:%d)", req.Name, id))

	return sub, plainSecret, nil
}

// entWebhookSubscriptionToModel converts a generated row to the API shape.
//
// The secret is deliberately left empty: it is encrypted at rest and no read
// path returns it. Only delivery decrypts, and it does so into a value that
// never reaches a response.
func entWebhookSubscriptionToModel(w *ent.WebhookSubscription) *model.WebhookSubscription {
	return &model.WebhookSubscription{
		ID:              int64(w.ID),
		Name:            w.Name,
		URL:             w.URL,
		Secret:          "",
		Events:          w.Events,
		Enabled:         w.Enabled,
		FailureCount:    int(w.FailureCount),
		LastTriggeredAt: w.LastTriggeredAt,
		CreatedBy:       w.CreatedBy,
		CreatedAt:       w.CreatedAt,
		UpdatedAt:       w.UpdatedAt,
	}
}

func (s *WebhookSubscriptionService) GetByID(ctx context.Context, id int64) (*model.WebhookSubscription, error) {
	w, err := s.entClient.WebhookSubscription.Get(ctx, int(id))
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询订阅失败: %w", err)
	}
	return entWebhookSubscriptionToModel(w), nil
}

func (s *WebhookSubscriptionService) List(ctx context.Context) ([]*model.WebhookSubscription, error) {
	rows, err := s.entClient.WebhookSubscription.Query().
		Order(ent.Desc(entwebhooksubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}
	subs := make([]*model.WebhookSubscription, 0, len(rows))
	for _, w := range rows {
		subs = append(subs, entWebhookSubscriptionToModel(w))
	}
	return subs, nil
}

// listForDelivery returns all enabled subscriptions with decrypted secrets in a single query pass.
// This avoids N+1 queries compared to List + per-item decryptSecret.
func (s *WebhookSubscriptionService) listForDelivery(ctx context.Context) ([]*deliveryTarget, error) {
	rows, err := s.entClient.WebhookSubscription.Query().
		Where(entwebhooksubscription.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询订阅失败: %w", err)
	}

	targets := make([]*deliveryTarget, 0, len(rows))
	for _, w := range rows {
		id := int64(w.ID)
		targetURL := w.URL

		plainSecret, err := crypto.Decrypt(w.EncryptedSecret, s.key)
		if err != nil {
			// Skipped rather than failed: one unreadable secret must not stop
			// delivery to every other subscriber.
			log.Printf("webhook: failed to decrypt secret for subscription %d: %v", id, err)
			continue
		}

		var events []string
		if err := json.Unmarshal([]byte(w.Events), &events); err != nil {
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
	return targets, nil
}

func (s *WebhookSubscriptionService) Update(ctx context.Context, id int64, req UpdateSubscriptionRequest, actorID int64, actorName string) (*model.WebhookSubscription, error) {
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

	updated, err := s.entClient.WebhookSubscription.UpdateOneID(int(id)).
		SetName(name).
		SetURL(webURL).
		SetEvents(events).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("更新订阅失败: %w", err)
	}

	s.writeAuditLog(ctx, "webhook_subscription.update", actorID, actorName, fmt.Sprintf("更新 Webhook 订阅: %s (ID:%d)", name, id))

	return entWebhookSubscriptionToModel(updated), nil
}

func (s *WebhookSubscriptionService) Delete(ctx context.Context, id int64, actorID int64, actorName string) error {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	err = s.entClient.WebhookSubscription.DeleteOneID(int(id)).Exec(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("删除订阅失败: %w", err)
	}

	s.writeAuditLog(ctx, "webhook_subscription.delete", actorID, actorName, fmt.Sprintf("删除 Webhook 订阅: %s (ID:%d)", existing.Name, id))

	return nil
}

func (s *WebhookSubscriptionService) Toggle(ctx context.Context, id int64, actorID int64, actorName string) (*model.WebhookSubscription, error) {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	newEnabled := !existing.Enabled
	// Atomic: reset failure_count on re-enable, keep on disable
	var failureCount int64
	if newEnabled {
		failureCount = 0
	} else {
		failureCount = int64(existing.FailureCount)
	}

	// The boolean goes through as a boolean. It used to be converted to 0/1,
	// which PostgreSQL rejects outright for a boolean column — so toggling a
	// subscription failed every time.
	toggled, err := s.entClient.WebhookSubscription.UpdateOneID(int(id)).
		SetEnabled(newEnabled).
		SetFailureCount(failureCount).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("切换订阅状态失败: %w", err)
	}

	action := "禁用"
	if newEnabled {
		action = "启用"
	}
	s.writeAuditLog(ctx, "webhook_subscription.toggle", actorID, actorName, fmt.Sprintf("%s Webhook 订阅: %s (ID:%d)", action, existing.Name, id))

	return entWebhookSubscriptionToModel(toggled), nil
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
	now := time.Now()
	err := s.entClient.WebhookSubscription.UpdateOneID(int(subID)).
		SetFailureCount(0).
		SetLastTriggeredAt(now).
		SetUpdatedAt(now).
		Exec(ctx)
	if err != nil {
		log.Printf("webhook: failed to update success for subscription %d: %v", subID, err)
	}
}

func (s *WebhookSubscriptionService) handleFailure(subID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// AddFailureCount is an atomic increment, so two failing deliveries do not
	// lose a count between them.
	updated, err := s.entClient.WebhookSubscription.UpdateOneID(int(subID)).
		AddFailureCount(1).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		log.Printf("webhook: failed to increment failure for subscription %d: %v", subID, err)
		return
	}

	if updated.FailureCount < MaxConsecutiveFailures || !updated.Enabled {
		return
	}

	// Guarded on enabled so that two deliveries failing at once disable it
	// once, and a subscription an operator disabled meanwhile is left alone.
	// The comparison read enabled into an int, which PostgreSQL will not scan a
	// boolean into — so auto-disable never ran at all.
	n, err := s.entClient.WebhookSubscription.Update().
		Where(
			entwebhooksubscription.IDEQ(int(subID)),
			entwebhooksubscription.EnabledEQ(true),
		).
		SetEnabled(false).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		log.Printf("webhook: failed to auto-disable subscription %d: %v", subID, err)
		return
	}
	if n > 0 {
		log.Printf("webhook: subscription %d auto-disabled after %d consecutive failures", subID, updated.FailureCount)
	}
}

// decryptSecret decrypts the stored encrypted secret for a subscription.
func (s *WebhookSubscriptionService) decryptSecret(ctx context.Context, id int64) (string, error) {
	encryptedSecret, err := s.entClient.WebhookSubscription.Query().
		Where(entwebhooksubscription.IDEQ(int(id))).
		Select(entwebhooksubscription.FieldEncryptedSecret).
		String(ctx)
	if ent.IsNotFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
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

// writeAuditLog records who changed what.
//
// actorID used to be 0 at every call site, with the name formatted into
// error_message. So these rows had no actor: filtering the audit log by user
// never returned them, and the per-user reports counted them against nobody.
func (s *WebhookSubscriptionService) writeAuditLog(ctx context.Context, action string, actorID int64, actorName, details string) {
	err := s.entClient.AuditLog.Create().
		SetUserID(actorID).
		SetAction(action).
		SetIPAddress("").
		SetErrorMessage(fmt.Sprintf("%s — %s", actorName, details)).
		SetCreatedAt(time.Now()).
		Exec(ctx)
	if err != nil {
		// A failed audit write must not fail the operation it describes, but it
		// must be visible: the records nobody can find are the ones operations
		// needs most.
		log.Printf("webhook: failed to write audit log: %v", err)
	}
}

// --- Utility ---

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
