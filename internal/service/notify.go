package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

// NotifyService handles DingTalk notification logic.
type NotifyService struct {
	webhookURL string
	secret     string
	enabled    bool
	client     *http.Client
	mu         sync.RWMutex
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

// NotifyTicketCreated sends a notification when a ticket is created.
func (s *NotifyService) NotifyTicketCreated(t *model.Ticket) {
	if !s.isEnabled() {
		return
	}

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

// NotifyTicketApproved sends a notification when a ticket is approved.
func (s *NotifyService) NotifyTicketApproved(t *model.Ticket) {
	if !s.isEnabled() {
		return
	}

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

// NotifyTicketRejected sends a notification when a ticket is rejected.
func (s *NotifyService) NotifyTicketRejected(t *model.Ticket) {
	if !s.isEnabled() {
		return
	}

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

// NotifyTicketExecuted sends a notification when a ticket SQL is executed.
func (s *NotifyService) NotifyTicketExecuted(t *model.Ticket) {
	if !s.isEnabled() {
		return
	}

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
	MsgType  string              `json:"msgtype"`
	Markdown *dingTalkMarkdown   `json:"markdown,omitempty"`
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
	defer resp.Body.Close()

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

// riskLabel returns a human-readable risk level label.
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
