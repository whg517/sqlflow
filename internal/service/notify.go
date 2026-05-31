package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// NotifyService handles webhook (DingTalk-compatible) and Feishu notification logic.
type NotifyService struct {
	webhookURL    string // webhook
	secret        string // webhook secret
	enabled       bool   // webhook enabled
	feishuURL     string // Feishu webhook
	feishuEnabled bool   // Feishu
	db            *sql.DB // notification log (nil = no dedup)
	client        *http.Client
	mu            sync.RWMutex
}

// NewNotifyService creates a new NotifyService.
func NewNotifyService(webhookURL, secret string) *NotifyService {
	enabled := webhookURL != ""
	return &NotifyService{
		webhookURL: webhookURL,
		secret:     secret,
		enabled:    enabled,
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

// SetDB sets the database for notification logging (deduplication).
func (s *NotifyService) SetDB(db *sql.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
}

// SetFeishuWebhook sets the Feishu webhook URL.
func (s *NotifyService) SetFeishuWebhook(u string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feishuURL = u
	s.feishuEnabled = u != ""
}

// IsFeishuEnabled returns whether Feishu notification is enabled.
func (s *NotifyService) IsFeishuEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.feishuEnabled
}

// GetFeishuConfig returns the current Feishu configuration.
func (s *NotifyService) GetFeishuConfig() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]interface{}{
		"webhook_url": s.feishuURL,
		"enabled":     s.feishuEnabled,
	}
}

// UpdateFeishuConfig updates the Feishu webhook configuration.
func (s *NotifyService) UpdateFeishuConfig(webhookURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feishuURL = webhookURL
	s.feishuEnabled = webhookURL != ""
}

// UpdateConfig updates the DingTalk configuration at runtime.
func (s *NotifyService) UpdateConfig(webhookURL, secret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhookURL = webhookURL
	s.secret = secret
	s.enabled = webhookURL != ""
}

// GetConfig returns the current DingTalk configuration (with secret masked).
func (s *NotifyService) GetConfig() map[string]interface{} {
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
func (s *NotifyService) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// ---------------------------------------------------------------------------
// Ticket notification methods (with deduplication via notification log)
// ---------------------------------------------------------------------------

// shouldNotify checks if a notification for the given ticket+event has already
// been sent (idempotency). Returns true if the notification should proceed.
func (s *NotifyService) shouldNotify(ctx context.Context, ticketID int64, eventType string) bool {
	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()

	if db == nil {
		return true // no dedup available
	}

	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ticket_notification_logs WHERE ticket_id = ? AND event_type = ?`,
		ticketID, eventType,
	).Scan(&count)
	if err != nil {
		log.Printf("notify: check log: %v", err)
		return true // on error, allow notification
	}
	return count == 0
}

// recordNotification records that a notification was sent for deduplication.
func (s *NotifyService) recordNotification(ctx context.Context, ticketID int64, eventType string) {
	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()

	if db == nil {
		return
	}

	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO ticket_notification_logs (ticket_id, event_type) VALUES (?, ?)`,
		ticketID, eventType,
	)
	if err != nil {
		log.Printf("notify: record log: %v", err)
	}
}

// notifyWithDedup wraps a notification send with idempotency check and logging.
func (s *NotifyService) notifyWithDedup(ctx context.Context, ticketID int64, eventType string, sendFn func()) {
	if s.shouldNotify(ctx, ticketID, eventType) {
		sendFn()
		s.recordNotification(ctx, ticketID, eventType)
	}
}

// NotifyTicketCreated sends a notification when a ticket is created.
func (s *NotifyService) NotifyTicketCreated(ctx context.Context, t *model.Ticket) {
	s.notifyWithDedup(ctx, t.ID, "created", func() {
		if s.isEnabled() {
			title := "📋 工单提交通知"
			text := fmt.Sprintf(
				"**工单 #%d 已提交**\n\n"+
					"- **提交人**: %s\n"+
					"- **数据源ID**: %d\n"+
					"- **数据库**: %s\n"+
					"- **SQL摘要**: %s\n"+
					"- **风险等级**: %s\n"+
					"- **提交时间**: %s",
				t.ID, t.SubmitterName, t.DatasourceID, t.Database,
				t.SQLSummary, riskLabel(t.RiskLevel),
				t.CreatedAt.Format("2006-01-02 15:04:05"),
			)
			go s.sendMarkdown(title, text)
		}

		if s.isFeishuEnabled() {
			go s.sendFeishuCard(
				"📋 工单提交通知",
				fmt.Sprintf("**工单 #%d 已提交**", t.ID),
				[]feishuCardElement{
					feishuCardField("提交人", t.SubmitterName),
					feishuCardField("数据库", t.Database),
					feishuCardField("SQL摘要", t.SQLSummary),
					feishuCardField("风险等级", riskLabel(t.RiskLevel)),
					feishuCardField("提交时间", t.CreatedAt.Format("2006-01-02 15:04:05")),
				},
			)
		}
	})
}

// NotifyTicketPendingApproval sends a notification to approvers when a ticket enters the approval queue.
func (s *NotifyService) NotifyTicketPendingApproval(ctx context.Context, t *model.Ticket) {
	s.notifyWithDedup(ctx, t.ID, "pending_approval", func() {
		if s.isEnabled() {
			title := "📝 待审批工单提醒"
			text := fmt.Sprintf(
				"**工单 #%d 进入审批队列**\n\n"+
					"- **提交人**: %s\n"+
					"- **数据库**: %s\n"+
					"- **SQL摘要**: %s\n"+
					"- **风险等级**: %s\n"+
					"- **提交时间**: %s\n\n"+
					"请及时审批。",
				t.ID, t.SubmitterName, t.Database,
				t.SQLSummary, riskLabel(t.RiskLevel),
				t.CreatedAt.Format("2006-01-02 15:04:05"),
			)
			go s.sendMarkdown(title, text)
		}

		if s.isFeishuEnabled() {
			go s.sendFeishuCard(
				"📝 待审批工单提醒",
				fmt.Sprintf("**工单 #%d 进入审批队列，请及时处理**", t.ID),
				[]feishuCardElement{
					feishuCardField("提交人", t.SubmitterName),
					feishuCardField("数据库", t.Database),
					feishuCardField("SQL摘要", t.SQLSummary),
					feishuCardField("风险等级", riskLabel(t.RiskLevel)),
					feishuCardField("提交时间", t.CreatedAt.Format("2006-01-02 15:04:05")),
				},
			)
		}
	})
}

// NotifyTicketApproved sends a notification when a ticket is approved.
func (s *NotifyService) NotifyTicketApproved(ctx context.Context, t *model.Ticket) {
	s.notifyWithDedup(ctx, t.ID, "approved", func() {
		if s.isEnabled() {
			title := "✅ 工单审批通过通知"
			text := fmt.Sprintf(
				"**工单 #%d 已审批通过**\n\n"+
					"- **提交人**: %s\n"+
					"- **审批人**: %s\n"+
					"- **SQL摘要**: %s\n"+
					"- **风险等级**: %s\n"+
					"- **审批时间**: %s",
				t.ID, t.SubmitterName, t.ReviewerName,
				t.SQLSummary, riskLabel(t.RiskLevel),
				t.UpdatedAt.Format("2006-01-02 15:04:05"),
			)
			go s.sendMarkdown(title, text)
		}

		if s.isFeishuEnabled() {
			go s.sendFeishuCard(
				"✅ 工单审批通过通知",
				fmt.Sprintf("**工单 #%d 已审批通过**", t.ID),
				[]feishuCardElement{
					feishuCardField("提交人", t.SubmitterName),
					feishuCardField("审批人", t.ReviewerName),
					feishuCardField("SQL摘要", t.SQLSummary),
					feishuCardField("风险等级", riskLabel(t.RiskLevel)),
					feishuCardField("审批时间", t.UpdatedAt.Format("2006-01-02 15:04:05")),
				},
			)
		}
	})
}

// NotifyTicketRejected sends a notification when a ticket is rejected.
func (s *NotifyService) NotifyTicketRejected(ctx context.Context, t *model.Ticket) {
	s.notifyWithDedup(ctx, t.ID, "rejected", func() {
		if s.isEnabled() {
			title := "❌ 工单驳回通知"
			text := fmt.Sprintf(
				"**工单 #%d 已驳回**\n\n"+
					"- **提交人**: %s\n"+
					"- **审批人**: %s\n"+
					"- **SQL摘要**: %s\n"+
					"- **驳回原因**: %s\n"+
					"- **驳回时间**: %s",
				t.ID, t.SubmitterName, t.ReviewerName,
				t.SQLSummary, t.ReviewComment,
				t.UpdatedAt.Format("2006-01-02 15:04:05"),
			)
			go s.sendMarkdown(title, text)
		}

		if s.isFeishuEnabled() {
			go s.sendFeishuCard(
				"❌ 工单驳回通知",
				fmt.Sprintf("**工单 #%d 已驳回**", t.ID),
				[]feishuCardElement{
					feishuCardField("提交人", t.SubmitterName),
					feishuCardField("审批人", t.ReviewerName),
					feishuCardField("SQL摘要", t.SQLSummary),
					feishuCardField("驳回原因", t.ReviewComment),
					feishuCardField("驳回时间", t.UpdatedAt.Format("2006-01-02 15:04:05")),
				},
			)
		}
	})
}

// NotifyTicketScheduled sends a notification when a ticket is scheduled for execution.
func (s *NotifyService) NotifyTicketScheduled(ctx context.Context, t *model.Ticket) {
	s.notifyWithDedup(ctx, t.ID, "scheduled", func() {
		var scheduledTime string
		if t.ScheduledAt != nil {
			scheduledTime = t.ScheduledAt.Format("2006-01-02 15:04:05")
		} else {
			scheduledTime = "未指定"
		}

		if s.isEnabled() {
			title := "⏰ 工单定时执行通知"
			text := fmt.Sprintf(
				"**工单 #%d 已设置定时执行**\n\n"+
					"- **提交人**: %s\n"+
					"- **SQL摘要**: %s\n"+
					"- **风险等级**: %s\n"+
					"- **计划执行时间**: %s",
				t.ID, t.SubmitterName,
				t.SQLSummary, riskLabel(t.RiskLevel),
				scheduledTime,
			)
			go s.sendMarkdown(title, text)
		}

		if s.isFeishuEnabled() {
			go s.sendFeishuCard(
				"⏰ 工单定时执行通知",
				fmt.Sprintf("**工单 #%d 已设置定时执行**", t.ID),
				[]feishuCardElement{
					feishuCardField("提交人", t.SubmitterName),
					feishuCardField("SQL摘要", t.SQLSummary),
					feishuCardField("风险等级", riskLabel(t.RiskLevel)),
					feishuCardField("计划执行时间", scheduledTime),
				},
			)
		}
	})
}

// NotifyTicketExecuted sends a notification when a ticket SQL is executed successfully.
func (s *NotifyService) NotifyTicketExecuted(ctx context.Context, t *model.Ticket) {
	s.notifyWithDedup(ctx, t.ID, "executed", func() {
		if s.isEnabled() {
			title := "🔧 工单执行完成通知"
			text := fmt.Sprintf(
				"**工单 #%d 已执行完成**\n\n"+
					"- **提交人**: %s\n"+
					"- **SQL摘要**: %s\n"+
					"- **执行时间**: %s",
				t.ID, t.SubmitterName,
				t.SQLSummary,
				t.UpdatedAt.Format("2006-01-02 15:04:05"),
			)
			go s.sendMarkdown(title, text)
		}

		if s.isFeishuEnabled() {
			go s.sendFeishuCard(
				"🔧 工单执行完成通知",
				fmt.Sprintf("**工单 #%d 已执行完成**", t.ID),
				[]feishuCardElement{
					feishuCardField("提交人", t.SubmitterName),
					feishuCardField("SQL摘要", t.SQLSummary),
					feishuCardField("执行时间", t.UpdatedAt.Format("2006-01-02 15:04:05")),
				},
			)
		}
	})
}

// NotifyTicketFailed sends a notification when a ticket execution fails.
func (s *NotifyService) NotifyTicketFailed(ctx context.Context, t *model.Ticket, errMsg string) {
	s.notifyWithDedup(ctx, t.ID, "failed", func() {
		// Truncate error message for display
		displayErr := errMsg
		if len(displayErr) > 200 {
			displayErr = displayErr[:200] + "..."
		}

		if s.isEnabled() {
			title := "🚨 工单执行失败通知"
			text := fmt.Sprintf(
				"**工单 #%d 执行失败**\n\n"+
					"- **提交人**: %s\n"+
					"- **SQL摘要**: %s\n"+
					"- **错误信息**: %s\n"+
					"- **失败时间**: %s",
				t.ID, t.SubmitterName,
				t.SQLSummary, displayErr,
				t.UpdatedAt.Format("2006-01-02 15:04:05"),
			)
			go s.sendMarkdown(title, text)
		}

		if s.isFeishuEnabled() {
			go s.sendFeishuCard(
				"🚨 工单执行失败通知",
				fmt.Sprintf("**工单 #%d 执行失败**", t.ID),
				[]feishuCardElement{
					feishuCardField("提交人", t.SubmitterName),
					feishuCardField("SQL摘要", t.SQLSummary),
					feishuCardField("错误信息", displayErr),
					feishuCardField("失败时间", t.UpdatedAt.Format("2006-01-02 15:04:05")),
				},
			)
		}
	})
}

// NotifyRiskAlert sends a real-time alert for medium/high risk operations.
func (s *NotifyService) NotifyRiskAlert(username, sqlSummary, riskLevel string, datasourceID int64, database string) {
	if !s.isEnabled() {
		return
	}

	title := "⚠️ 风险操作告警"
	text := fmt.Sprintf(
		"**检测到%s操作**\n\n"+
			"- **操作人**: %s\n"+
			"- **数据源ID**: %d\n"+
			"- **数据库**: %s\n"+
			"- **SQL摘要**: %s\n"+
			"- **风险等级**: %s\n"+
			"- **告警时间**: %s",
		riskLabel(riskLevel), username, datasourceID, database,
		sqlSummary, riskLabel(riskLevel),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	go s.sendMarkdown(title, text)
}

// SendTestMessage sends a test message to verify the DingTalk configuration.
func (s *NotifyService) SendTestMessage() {
	if !s.isEnabled() {
		return
	}

	title := "🔔 SQLFlow 测试通知"
	text := fmt.Sprintf(
		"**SQLFlow 钉钉通知测试**\n\n"+
			"- **发送时间**: %s\n"+
			"- **状态**: 配置成功",
		time.Now().Format("2006-01-02 15:04:05"),
	)

	go s.sendMarkdown(title, text)
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

// sendMarkdown sends a markdown message to DingTalk.
func (s *NotifyService) sendMarkdown(title, text string) {
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
func (s *NotifyService) doSend(reqBody *dingTalkRequest) {
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
func (s *NotifyService) sign(timestamp int64, secret string) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// isEnabled is a thread-safe check for whether notifications are enabled.
func (s *NotifyService) isEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// NotifySLAReminder sends a DingTalk notification for SLA reminder (80% elapsed).
func (s *NotifyService) NotifySLAReminder(ticket *model.Ticket, elapsedHours, slaHours, approverName, submitterName string, percent float64) {
	if !s.isEnabled() {
		return
	}

	title := "⏰ [SQLFlow] 工单审批提醒"
	text := fmt.Sprintf(
		"**工单 #%d 审批提醒**\n\n"+
			"- **SQL 摘要**: %s\n"+
			"- **提交人**: %s\n"+
			"- **风险等级**: %s\n"+
			"- **审批人**: %s\n"+
			"- **已等待**: %sh / 时限 %sh（%.0f%%）\n"+
			"- **提交时间**: %s\n\n"+
			"请及时处理该工单。",
		ticket.ID, ticket.SQLSummary, submitterName, riskLabel(ticket.RiskLevel),
		approverName, elapsedHours, slaHours, percent,
		ticket.CreatedAt.Format("2006-01-02 15:04:05"),
	)

	go s.sendMarkdown(title, text)
}

// NotifySLAEscalate sends a DingTalk notification for SLA escalation (100% elapsed).
func (s *NotifyService) NotifySLAEscalate(ticket *model.Ticket, slaHours, approverName, submitterName string) {
	if !s.isEnabled() {
		return
	}

	title := "🚨 [SQLFlow] 工单审批超时升级"
	text := fmt.Sprintf(
		"**工单 #%d 审批超时升级**\n\n"+
			"- **SQL 摘要**: %s\n"+
			"- **提交人**: %s\n"+
			"- **风险等级**: %s\n"+
			"- **超时时限**: %sh\n"+
			"- **审批人**: %s（已提醒未处理）\n"+
			"- **提交时间**: %s\n\n"+
			"请立即处理该工单。",
		ticket.ID, ticket.SQLSummary, submitterName, riskLabel(ticket.RiskLevel),
		slaHours, approverName,
		ticket.CreatedAt.Format("2006-01-02 15:04:05"),
	)

	go s.sendMarkdown(title, text)
}

// NotifySLAReminderRaw sends an SLA reminder notification using minimal fields.
// This decouples NotifyService from the Ticket model — SLAService only provides
// what's needed for the notification, not a full Ticket struct.
func (s *NotifyService) NotifySLAReminderRaw(ticketID int64, elapsedHours, slaHours, approverName string, percent float64) {
	if !s.isEnabled() {
		return
	}

	title := "\u23f0 [SQLFlow] 工单审批提醒"
	text := fmt.Sprintf(
		"**工单 #%d 审批提醒**\n\n"+
			"- **已等待**: %sh / 时限 %sh（%.0f%%）\n"+
			"- **审批人**: %s\n\n"+
			"请及时处理该工单。",
		ticketID, elapsedHours, slaHours, percent, approverName,
	)

	go s.sendMarkdown(title, text)
}

// NotifySLAEscalateRaw sends an SLA escalation notification using minimal fields.
func (s *NotifyService) NotifySLAEscalateRaw(ticketID int64, slaHours, approverName string) {
	if !s.isEnabled() {
		return
	}

	title := "\U0001f6a8 [SQLFlow] 工单审批超时升级"
	text := fmt.Sprintf(
		"**工单 #%d 审批超时升级**\n\n"+
			"- **超时时限**: %sh\n"+
			"- **审批人**: %s（已提醒未处理）\n\n"+
			"请立即处理该工单。",
		ticketID, slaHours, approverName,
	)

	go s.sendMarkdown(title, text)
}

// NotifySLAAutoReject sends a notification to the submitter when their ticket
// is automatically rejected due to SLA timeout.
func (s *NotifyService) NotifySLAAutoReject(ticketID int64, priority string, timeoutMinutes int, submitterName, sqlSummary string) {
	if !s.isEnabled() {
		return
	}

	title := "\U0001f6ab [SQLFlow] 工单审批超时自动拒绝"
	text := fmt.Sprintf(
		"**工单 #%d 已因审批超时被自动拒绝**\n\n"+
			"- **优先级**: %s\n"+
			"- **超时阈值**: %d 分钟\n"+
			"- **SQL 摘要**: %s\n\n"+
			"如有疑问请联系审批人或管理员。",
		ticketID, priority, timeoutMinutes, sqlSummary,
	)

	go s.sendMarkdown(title, text)
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

// feishuCardField creates a card element with a key-value field.
func feishuCardField(label, value string) feishuCardElement {
	return feishuCardElement{
		Tag: "div",
		Text: &struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		}{
			Tag:     "lark_md",
			Content: fmt.Sprintf("**%s**: %s", label, value),
		},
	}
}

// isFeishuEnabled is a thread-safe check for whether Feishu notifications are enabled.
func (s *NotifyService) isFeishuEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.feishuEnabled
}

// sendFeishuCard sends an Interactive Card message to Feishu webhook.
func (s *NotifyService) sendFeishuCard(title, summary string, elements []feishuCardElement) {
	s.mu.RLock()
	webhookURL := s.feishuURL
	s.mu.RUnlock()

	if webhookURL == "" {
		return
	}

	// Prepend summary element
	allElements := append([]feishuCardElement{{
		Tag: "div",
		Text: &struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		}{
			Tag:     "lark_md",
			Content: summary,
		},
	}}, elements...)

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

	s.doSendFeishu(webhookURL, reqBody)
}

// doSendFeishu sends the request to Feishu webhook with retry (3 attempts, exponential backoff).
func (s *NotifyService) doSendFeishu(webhookURL string, reqBody feishuRequest) {
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

// SendFeishuTestMessage sends a test message to verify the Feishu configuration.
func (s *NotifyService) SendFeishuTestMessage() {
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
