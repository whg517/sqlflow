package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entticketnotificationlog "github.com/whg517/sqlflow/internal/db/ent/ticketnotificationlog"
	"github.com/whg517/sqlflow/internal/model"
)

// Service handles webhook (DingTalk-compatible) and Feishu notification logic.
type Service struct {
	webhookURL    string // webhook
	secret        string // webhook secret
	enabled       bool   // webhook enabled
	feishuURL     string // Feishu webhook (legacy single-URL mode)
	feishuEnabled bool   // Feishu
	// entClient backs notification-log dedup; nil disables it. Named apart
	// from client, which is the HTTP client this service posts with.
	entClient        *ent.Client
	client           *http.Client
	feishuWebhookSvc *FeishuService // multi-webhook DB-backed service

	// prefSvc and subSvc are consulted by the dispatcher. Both were fully
	// implemented and unreachable before there was one event identity to key
	// on: preferences could not match the sender's spelling, and outbound
	// webhooks had no event source at all.
	prefSvc *PreferenceService
	subSvc  *WebhookSubscriptionService

	// transportsForTest replaces the real adapter set. Tests need to observe
	// what the decision side produced, and the alternative — asserting on
	// outbound HTTP — is what forced the old suite to race goroutines with a
	// request recorder.
	transportsForTest []transport

	mu sync.RWMutex
}

// SetPreferences wires per-channel event preferences.
func (s *Service) SetPreferences(p *PreferenceService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefSvc = p
}

// SetSubscriptions wires outbound webhook delivery.
func (s *Service) SetSubscriptions(w *WebhookSubscriptionService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subSvc = w
}

// Deps are the collaborators and initial configuration of a notify Service.
//
// The webhook URLs also arrive through UpdateConfig at runtime, from the
// settings page; these are the values the process starts with.
type Deps struct {
	// WebhookURL and Secret configure the DingTalk-compatible webhook. An empty
	// WebhookURL leaves that channel disabled.
	WebhookURL string
	Secret     string
	// FeishuURL is the legacy single-URL Feishu channel. Empty disables it.
	FeishuURL string
	// Feishu is the DB-backed multi-webhook service. Without it only FeishuURL
	// is used.
	Feishu *FeishuService
	// DB backs notification-log deduplication. Without it every notification is
	// sent, including repeats.
	DB *db.DB
}

// entClientOrNil unwraps the platform handle, tolerating a nil one.
//
// Notification dedup is optional — several tests and the no-database
// configuration pass nothing — so this keeps the nil check in one place rather
// than at both call sites.
func entClientOrNil(database *db.DB) *ent.Client {
	if database == nil {
		return nil
	}
	return database.Client()
}

// NewService creates a Service from its dependencies.
func NewService(deps Deps) *Service {
	return &Service{
		webhookURL:       deps.WebhookURL,
		secret:           deps.Secret,
		enabled:          deps.WebhookURL != "",
		feishuURL:        deps.FeishuURL,
		feishuEnabled:    deps.FeishuURL != "",
		feishuWebhookSvc: deps.Feishu,
		entClient:        entClientOrNil(deps.DB),
		client:           &http.Client{Timeout: 5 * time.Second},
	}
}

// IsFeishuEnabled returns whether Feishu notification is enabled.
func (s *Service) IsFeishuEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.feishuEnabled
}

// GetFeishuConfig returns the current Feishu configuration.
func (s *Service) GetFeishuConfig() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]interface{}{
		"webhook_url": s.feishuURL,
		"enabled":     s.feishuEnabled,
	}
}

// UpdateFeishuConfig updates the Feishu webhook configuration.
func (s *Service) UpdateFeishuConfig(webhookURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feishuURL = webhookURL
	s.feishuEnabled = webhookURL != ""
}

// UpdateConfig updates the DingTalk configuration at runtime.
func (s *Service) UpdateConfig(webhookURL, secret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhookURL = webhookURL
	s.secret = secret
	s.enabled = webhookURL != ""
}

// GetConfig returns the current DingTalk configuration (with secret masked).
func (s *Service) GetConfig() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	secret := s.secret
	if len(secret) > 4 {
		secret = secret[:2] + "****" + secret[len(secret)-2:]
	} else if secret != "" {
		secret = "****"
	}

	return map[string]interface{}{
		"webhook_url": s.webhookURL,
		"secret":      secret,
		"enabled":     s.enabled,
	}
}

// IsEnabled returns whether DingTalk notification is enabled.
func (s *Service) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// ---------------------------------------------------------------------------
// Ticket notification methods (with deduplication via notification log)
// ---------------------------------------------------------------------------

// shouldNotify checks if a notification for the given ticket+event has already
// been sent (idempotency). Returns true if the notification should proceed.
func (s *Service) shouldNotify(ctx context.Context, ticketID int64, eventType string) bool {
	s.mu.RLock()
	entClient := s.entClient
	s.mu.RUnlock()

	if entClient == nil {
		return true // no dedup available
	}

	sent, err := entClient.TicketNotificationLog.Query().
		Where(
			entticketnotificationlog.TicketIDEQ(ticketID),
			entticketnotificationlog.EventTypeEQ(eventType),
		).
		Exist(ctx)
	if err != nil {
		// A dedup lookup that fails allows the send: a duplicate notification is
		// a smaller failure than a silently missing one.
		log.Printf("notify: check log: %v", err)
		return true
	}
	return !sent
}

// recordNotification records that a notification was sent for deduplication.
func (s *Service) recordNotification(ctx context.Context, ticketID int64, eventType string) {
	s.mu.RLock()
	entClient := s.entClient
	s.mu.RUnlock()

	if entClient == nil {
		return
	}

	// ON CONFLICT DO NOTHING against the unique index on (ticket_id,
	// event_type). This was written as INSERT OR IGNORE, which is SQLite's
	// spelling and a syntax error on PostgreSQL — so no dedup record was ever
	// written, and every re-trigger sent the notification again.
	err := entClient.TicketNotificationLog.Create().
		SetTicketID(ticketID).
		SetEventType(eventType).
		OnConflictColumns(
			entticketnotificationlog.FieldTicketID,
			entticketnotificationlog.FieldEventType,
		).
		DoNothing().
		Exec(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("notify: record log: %v", err)
	}
}

// The ticket lifecycle notifications.
//
// Each one decides WHAT happened and says nothing about how it is sent. That is
// the whole change: these used to be hand-written fan-outs that also chose the
// channels, rendered twice, and launched their own goroutines — and five of
// them silently reached only one of the two transports, because whoever added
// them copied one `if` block and not the other.

// NotifyTicketCreated sends a notification when a ticket is created.
func (s *Service) NotifyTicketCreated(ctx context.Context, t *model.Ticket) {
	s.dispatch(ctx, Notification{
		Event:    EventTicketCreated,
		TicketID: t.ID,
		Title:    "📋 工单提交通知",
		Summary:  fmt.Sprintf("**工单 #%d 已提交**", t.ID),
		Fields: []Field{
			{"提交人", t.SubmitterName},
			{"数据库", t.Database},
			{"SQL摘要", t.SQLSummary},
			{"风险等级", riskLabel(t.RiskLevel)},
			{"提交时间", t.CreatedAt.Format("2006-01-02 15:04:05")},
		},
	})
}

// NotifyTicketPendingApproval tells approvers a ticket is waiting for them.
func (s *Service) NotifyTicketPendingApproval(ctx context.Context, t *model.Ticket) {
	s.dispatch(ctx, Notification{
		Event:    EventTicketPendingApproval,
		TicketID: t.ID,
		Title:    "📝 待审批工单提醒",
		Summary:  fmt.Sprintf("**工单 #%d 等待审批**", t.ID),
		Fields: []Field{
			{"提交人", t.SubmitterName},
			{"数据库", t.Database},
			{"SQL摘要", t.SQLSummary},
			{"风险等级", riskLabel(t.RiskLevel)},
		},
	})
}

// NotifyTicketApproved reports an approval.
func (s *Service) NotifyTicketApproved(ctx context.Context, t *model.Ticket) {
	s.dispatch(ctx, Notification{
		Event:    EventTicketApproved,
		TicketID: t.ID,
		Title:    "✅ 工单审批通过通知",
		Summary:  fmt.Sprintf("**工单 #%d 已通过审批**", t.ID),
		Fields: []Field{
			{"提交人", t.SubmitterName},
			{"审批人", t.ReviewerName},
			{"数据库", t.Database},
			{"SQL摘要", t.SQLSummary},
			{"审批意见", t.ReviewComment},
		},
	})
}

// NotifyTicketRejected reports a rejection.
func (s *Service) NotifyTicketRejected(ctx context.Context, t *model.Ticket) {
	s.dispatch(ctx, Notification{
		Event:    EventTicketRejected,
		TicketID: t.ID,
		Title:    "❌ 工单驳回通知",
		Summary:  fmt.Sprintf("**工单 #%d 已被驳回**", t.ID),
		Fields: []Field{
			{"提交人", t.SubmitterName},
			{"审批人", t.ReviewerName},
			{"数据库", t.Database},
			{"SQL摘要", t.SQLSummary},
			{"驳回原因", t.ReviewComment},
		},
	})
}

// NotifyTicketScheduled reports that execution has been scheduled.
func (s *Service) NotifyTicketScheduled(ctx context.Context, t *model.Ticket) {
	scheduled := ""
	if t.ScheduledAt != nil {
		scheduled = t.ScheduledAt.Format("2006-01-02 15:04:05")
	}
	s.dispatch(ctx, Notification{
		Event:    EventTicketScheduled,
		TicketID: t.ID,
		Title:    "⏰ 工单定时执行通知",
		Summary:  fmt.Sprintf("**工单 #%d 已安排定时执行**", t.ID),
		Fields: []Field{
			{"提交人", t.SubmitterName},
			{"数据库", t.Database},
			{"SQL摘要", t.SQLSummary},
			{"计划执行时间", scheduled},
		},
	})
}

// NotifyTicketExecuted reports a successful execution.
func (s *Service) NotifyTicketExecuted(ctx context.Context, t *model.Ticket) {
	executed := ""
	if t.ExecutedAt != nil {
		executed = t.ExecutedAt.Format("2006-01-02 15:04:05")
	}
	s.dispatch(ctx, Notification{
		Event:    EventTicketExecuted,
		TicketID: t.ID,
		Title:    "🔧 工单执行完成通知",
		Summary:  fmt.Sprintf("**工单 #%d 已执行**", t.ID),
		Fields: []Field{
			{"提交人", t.SubmitterName},
			{"数据库", t.Database},
			{"SQL摘要", t.SQLSummary},
			{"执行时间", executed},
		},
	})
}

// NotifyTicketFailed reports a failed execution.
func (s *Service) NotifyTicketFailed(ctx context.Context, t *model.Ticket, errMsg string) {
	// A driver error can carry the whole failed statement; the message is a
	// notification, not a log.
	displayErr := errMsg
	if len(displayErr) > 200 {
		displayErr = displayErr[:200] + "..."
	}

	s.dispatch(ctx, Notification{
		Event:    EventTicketFailed,
		TicketID: t.ID,
		Title:    "🚨 工单执行失败通知",
		Summary:  fmt.Sprintf("**工单 #%d 执行失败**", t.ID),
		Fields: []Field{
			{"提交人", t.SubmitterName},
			{"数据库", t.Database},
			{"SQL摘要", t.SQLSummary},
			{"错误信息", displayErr},
			{"失败时间", t.UpdatedAt.Format("2006-01-02 15:04:05")},
		},
	})
}

// NotifyRiskAlert reports a high-risk statement.
//
// No TicketID: this is not about a ticket, so it is not deduplicated by one.
func (s *Service) NotifyRiskAlert(username, sqlSummary, riskLevel string, datasourceID int64, database string) {
	s.dispatch(context.Background(), Notification{
		Event:   EventRiskAlert,
		Title:   "⚠️ 风险操作告警",
		Summary: "**检测到高风险操作**",
		Fields: []Field{
			{"用户", username},
			{"数据源ID", fmt.Sprintf("%d", datasourceID)},
			{"数据库", database},
			{"SQL摘要", sqlSummary},
			{"风险等级", riskLabel(riskLevel)},
		},
	})
}

// SendTestMessage lets an operator confirm the channels are wired.
//
// It now reaches every configured transport rather than only DingTalk, which
// matters most for exactly this event: a test that cannot fail on the channel
// you are testing is not a test.
func (s *Service) SendTestMessage() {
	s.dispatch(context.Background(), Notification{
		Event:   EventTest,
		Title:   "🔔 SQLFlow 测试通知",
		Summary: "**这是一条测试消息**",
		Fields: []Field{
			{"说明", "如果你看到这条消息，说明通知渠道配置正确"},
		},
	})
}

// ---------------------------------------------------------------------------
// DingTalk Webhook API
// ---------------------------------------------------------------------------

// dingTalkRequest is the request body for DingTalk webhook.
type dingTalkRequest struct {
	MsgType  string               `json:"msgtype"`
	Markdown *dingTalkMarkdown    `json:"markdown,omitempty"`
	Text     *dingTalkTextContent `json:"text,omitempty"`
}

type dingTalkMarkdown struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type dingTalkTextContent struct {
	Content string `json:"content"`
}

// dingTalkResponse is the response from DingTalk webhook API.
type dingTalkResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (s *Service) sendMarkdown(title, text string) {
	reqBody := &dingTalkRequest{
		MsgType: "markdown",
		Markdown: &dingTalkMarkdown{
			Title: title,
			Text:  text,
		},
	}
	s.doSend(reqBody)
}

// doSend sends the request to DingTalk webhook.
func (s *Service) doSend(reqBody *dingTalkRequest) {
	s.mu.RLock()
	webhookURL := s.webhookURL
	secret := s.secret
	s.mu.RUnlock()

	if webhookURL == "" {
		return
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("notify: marshal request: %v", err)
		return
	}

	// Build URL with signature if secret is configured
	sendURL := webhookURL
	if secret != "" {
		ts := time.Now().UnixMilli()
		sign := s.sign(ts, secret)
		sep := "&"
		if !strings.Contains(webhookURL, "?") {
			sep = "?"
		}
		sendURL = fmt.Sprintf("%s%stimestamp=%d&sign=%s", webhookURL, sep, ts, url.QueryEscape(sign))
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, sendURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("notify: create request: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		log.Printf("notify: send request: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("notify: read response: %v", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("notify: unexpected status %d: %s", resp.StatusCode, string(respBody))
		return
	}

	var dingResp dingTalkResponse
	if err := json.Unmarshal(respBody, &dingResp); err != nil {
		log.Printf("notify: unmarshal response: %v", err)
		return
	}

	if dingResp.ErrCode != 0 {
		log.Printf("notify: dingtalk error: code=%d msg=%s", dingResp.ErrCode, dingResp.ErrMsg)
	}
}

// sign generates the DingTalk webhook signature.
func (s *Service) sign(timestamp int64, secret string) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// isEnabled is a thread-safe check for whether notifications are enabled.
func (s *Service) isEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// NotifySLAReminderRaw warns that an approval is running out of time.
//
// Every SLA notification reached DingTalk only, because each was written as a
// bare isEnabled + sendMarkdown rather than as a fan-out someone would have had
// to extend. A Feishu-only deployment therefore lost every reminder,
// escalation and auto-rejection — the three events an approver most needs.
func (s *Service) NotifySLAReminderRaw(ticketID int64, elapsedHours, slaHours, approverName string, percent float64) {
	s.dispatch(context.Background(), Notification{
		Event:   EventSLAWarning,
		Title:   "⏰ [SQLFlow] 工单审批提醒",
		Summary: fmt.Sprintf("**工单 #%d 审批提醒**，请及时处理。", ticketID),
		Fields: []Field{
			{"已等待", fmt.Sprintf("%sh / 时限 %sh（%.0f%%）", elapsedHours, slaHours, percent)},
			{"审批人", approverName},
		},
	})
}

// NotifySLAEscalateRaw reports an approval that has blown its deadline.
func (s *Service) NotifySLAEscalateRaw(ticketID int64, slaHours, approverName string) {
	s.dispatch(context.Background(), Notification{
		Event:   EventSLABreached,
		Title:   "🚨 [SQLFlow] 工单审批超时升级",
		Summary: fmt.Sprintf("**工单 #%d 审批已超时**", ticketID),
		Fields: []Field{
			{"时限", fmt.Sprintf("%sh", slaHours)},
			{"审批人", approverName},
		},
	})
}

// NotifySLAAutoReject reports a ticket auto-rejected for running out of time.
func (s *Service) NotifySLAAutoReject(ticketID int64, priority string, timeoutMinutes int, submitterName, sqlSummary string) {
	s.dispatch(context.Background(), Notification{
		Event:   EventSLAAutoRejected,
		Title:   "🚫 [SQLFlow] 工单审批超时自动拒绝",
		Summary: fmt.Sprintf("**工单 #%d 已因审批超时被自动拒绝**", ticketID),
		Fields: []Field{
			{"提交人", submitterName},
			{"优先级", priority},
			{"超时阈值", fmt.Sprintf("%d 分钟", timeoutMinutes)},
			{"SQL 摘要", sqlSummary},
		},
	})
}
func riskLabel(level string) string {
	switch strings.ToLower(level) {
	case "low":
		return "低风险"
	case "medium":
		return "中风险"
	case "high":
		return "高风险"
	default:
		if level == "" {
			return "未评估"
		}
		return level
	}
}

// ---------------------------------------------------------------------------
// Feishu Webhook API
// ---------------------------------------------------------------------------

// feishuCardElement represents a single element in a Feishu Interactive Card.
type feishuCardElement struct {
	Tag  string `json:"tag"`
	Text *struct {
		Tag     string `json:"tag"`
		Content string `json:"content"`
	} `json:"text,omitempty"`
}

// feishuRequest is the request body for Feishu webhook.
type feishuRequest struct {
	MsgType string `json:"msg_type"`
	Card    struct {
		Header   *feishuCardHeader   `json:"header,omitempty"`
		Elements []feishuCardElement `json:"elements"`
	} `json:"card"`
}

type feishuCardHeader struct {
	Title *struct {
		Tag     string `json:"tag"`
		Content string `json:"content"`
	} `json:"title"`
	Template string `json:"template,omitempty"`
}

// escapeFeishuContent escapes user-generated content for safe display in Feishu cards.
// Prevents injection of Feishu card markup.
func escapeFeishuContent(s string) string {
	return html.EscapeString(s)
}

// feishuCardField creates a card element with a key-value field.
func feishuCardField(label, value string) feishuCardElement {
	return feishuCardElement{
		Tag: "div",
		Text: &struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		}{
			Tag:     "lark_md",
			Content: fmt.Sprintf("**%s**: %s", escapeFeishuContent(label), escapeFeishuContent(value)),
		},
	}
}

// isFeishuEnabled is a thread-safe check for whether Feishu notifications are enabled.
func (s *Service) isFeishuEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.feishuEnabled
}

// sendFeishuCard sends an Interactive Card message to Feishu webhook(s).
// If feishuWebhookSvc is set, sends to all enabled DB-backed webhooks.
// Otherwise falls back to the legacy single-URL mode.
func (s *Service) sendFeishuCard(title, summary string, elements []feishuCardElement) {
	// Try DB-backed multi-webhook first
	s.mu.RLock()
	svc := s.feishuWebhookSvc
	s.mu.RUnlock()

	if svc != nil {
		ctx := context.Background()
		webhooks, err := svc.GetEnabledWebhooks(ctx)
		if err != nil {
			log.Printf("feishu: load webhooks from DB: %v", err)
		} else if len(webhooks) > 0 {
			for _, wh := range webhooks {
				// Rate limit check
				if !svc.CheckRateLimit(wh.ID, 1.0) {
					log.Printf("feishu: rate limited for webhook %d", wh.ID)
					continue
				}

				reqBody := s.buildFeishuCardRequest(title, summary, elements)
				bodyBytes, err := json.Marshal(reqBody)
				if err != nil {
					log.Printf("feishu: marshal request: %v", err)
					continue
				}

				s.doSendFeishuRawWithDeadLetter(wh.ID, wh.URL, bodyBytes, svc)
			}
			return
		}
	}

	// Fallback: legacy single URL
	s.mu.RLock()
	webhookURL := s.feishuURL
	s.mu.RUnlock()

	if webhookURL == "" {
		return
	}

	reqBody := s.buildFeishuCardRequest(title, summary, elements)
	s.doSendFeishu(webhookURL, reqBody)
}

// doSendFeishu sends the request to Feishu webhook with retry (3 attempts, exponential backoff).
func (s *Service) doSendFeishu(webhookURL string, reqBody feishuRequest) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("feishu: marshal request: %v", err)
		return
	}

	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second
			log.Printf("feishu: retry %d/%d after %v", attempt+1, maxRetries, backoff)
			time.Sleep(backoff)
		}

		httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, webhookURL, bytes.NewReader(body))
		if err != nil {
			log.Printf("feishu: create request: %v", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(httpReq)
		if err != nil {
			log.Printf("feishu: send request (attempt %d): %v", attempt+1, err)
			continue // retry on network error
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			// Check Feishu-specific error code
			var feishuResp struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(respBody, &feishuResp); err == nil && feishuResp.Code != 0 {
				log.Printf("feishu: api error: code=%d msg=%s", feishuResp.Code, feishuResp.Msg)
				return // API-level error, no retry
			}
			return // success
		}

		log.Printf("feishu: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("feishu: failed after %d retries", maxRetries)
}

// buildFeishuCardRequest constructs a feishuRequest with header, summary, and field elements.
func (s *Service) buildFeishuCardRequest(title, summary string, elements []feishuCardElement) feishuRequest {
	allElements := []feishuCardElement{{
		Tag: "div",
		Text: &struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		}{
			Tag:     "lark_md",
			Content: escapeFeishuContent(summary),
		},
	}}
	allElements = append(allElements, elements...)

	reqBody := feishuRequest{
		MsgType: "interactive",
	}
	reqBody.Card.Header = &feishuCardHeader{
		Title: &struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		}{
			Tag:     "plain_text",
			Content: title,
		},
	}
	reqBody.Card.Elements = allElements
	return reqBody
}

// doSendFeishuRawWithDeadLetter sends raw bytes to a Feishu webhook URL with retry and dead letter recording.
func (s *Service) doSendFeishuRawWithDeadLetter(webhookID int64, webhookURL string, body []byte, svc *FeishuService) {
	maxRetries := 3
	var lastErr string

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second
			log.Printf("feishu: retry %d/%d webhook %d after %v", attempt+1, maxRetries, webhookID, backoff)
			time.Sleep(backoff)
		}

		httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, webhookURL, bytes.NewReader(body))
		if err != nil {
			log.Printf("feishu: create request webhook %d: %v", webhookID, err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(httpReq)
		if err != nil {
			lastErr = err.Error()
			log.Printf("feishu: send webhook %d (attempt %d): %v", webhookID, attempt+1, err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var feishuResp struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(respBody, &feishuResp); err == nil && feishuResp.Code != 0 {
				log.Printf("feishu: api error webhook %d: code=%d msg=%s", webhookID, feishuResp.Code, feishuResp.Msg)
				// Record to dead letter
				if svc != nil {
					_ = svc.RecordDeadLetter(context.Background(), webhookID, string(body), fmt.Sprintf("api error: code=%d msg=%s", feishuResp.Code, feishuResp.Msg))
				}
				return
			}
			return // success
		}

		lastErr = fmt.Sprintf("status %d: %s", resp.StatusCode, string(respBody))
		log.Printf("feishu: unexpected status %d webhook %d: %s", resp.StatusCode, webhookID, string(respBody))
	}

	// All retries failed — record to dead letter
	log.Printf("feishu: failed after %d retries for webhook %d", maxRetries, webhookID)
	if svc != nil {
		_ = svc.RecordDeadLetter(context.Background(), webhookID, string(body), lastErr)
	}
}

// SendFeishuTestMessage sends a test message to verify the Feishu configuration.
func (s *Service) SendFeishuTestMessage() {
	if !s.isFeishuEnabled() {
		return
	}

	go s.sendFeishuCard(
		"🔔 SQLFlow 测试通知",
		"**SQLFlow 飞书通知测试**",
		[]feishuCardElement{
			feishuCardField("发送时间", time.Now().Format("2006-01-02 15:04:05")),
			feishuCardField("状态", "配置成功"),
		},
	)
}
