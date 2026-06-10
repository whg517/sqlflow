package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

// ---------------------------------------------------------------------------
// Constants & types
// ---------------------------------------------------------------------------

// Allowed notification event types (whitelist).
var AllowedEventTypes = map[string]string{
	"ticket_created":    "工单创建",
	"ticket_approved":   "审批通过",
	"ticket_rejected":   "审批驳回",
	"sla_warning":       "SLA 预警",
	"sla_breached":      "SLA 违规",
	"execution_complete": "执行完成",
	"execution_failed":  "执行失败",
}

// AllowedChannels are the globally configured notification channels.
var AllowedChannels = map[string]bool{
	"dingtalk": true,
	"feishu":   true,
}

// NotificationPreference represents a single user-event preference row.
type NotificationPreference struct {
	ID        int64    `json:"id"`
	UserID    int64    `json:"user_id"`
	EventType string   `json:"event_type"`
	Channels  []string `json:"channels"`
	UpdatedAt string   `json:"updated_at"`
}

// NotificationPreferenceService manages per-user notification preferences.
type NotificationPreferenceService struct {
	database *db.DB

	// prefCache caches per-user preferences to avoid N+1 in NotifyService.
	// Key: "userID:eventType", Value: channels slice (nil = use defaults).
	prefCache   map[string][]string
	prefCacheAt time.Time
	prefMu      sync.RWMutex
}

// NewNotificationPreferenceService creates a new service.
func NewNotificationPreferenceService(database *db.DB) *NotificationPreferenceService {
	return &NotificationPreferenceService{
		database:  database,
		prefCache: make(map[string][]string),
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// GetPreferences returns all preferences for a user.
// If the user has no saved preferences, returns defaults for all event types.
func (s *NotificationPreferenceService) GetPreferences(ctx context.Context, userID int64) ([]NotificationPreference, error) {
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT id, user_id, event_type, channels, updated_at
		 FROM notification_preferences
		 WHERE user_id = ?
		 ORDER BY event_type`, userID)
	if err != nil {
		return nil, fmt.Errorf("查询通知偏好失败: %w", err)
	}
	defer rows.Close()

	prefs := make([]NotificationPreference, 0)
	for rows.Next() {
		var p NotificationPreference
		var channelsJSON string
		if err := rows.Scan(&p.ID, &p.UserID, &p.EventType, &channelsJSON, &p.UpdatedAt); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(channelsJSON), &p.Channels)
		prefs = append(prefs, p)
	}

	// If no prefs saved, return defaults
	if len(prefs) == 0 {
		return s.defaultPreferences(userID), nil
	}

	// Fill in missing event types with defaults
	return s.fillDefaults(prefs, userID), nil
}

// UpdatePreferencesRequest is the batch update request body.
type UpdatePreferencesRequest struct {
	Preferences []struct {
		EventType string   `json:"event_type"`
		Channels  []string `json:"channels"`
	} `json:"preferences"`
}

// UpdatePreferences batch-updates preferences for a user.
func (s *NotificationPreferenceService) UpdatePreferences(ctx context.Context, userID int64, req UpdatePreferencesRequest) error {
	for _, pref := range req.Preferences {
		// Validate event type
		if _, ok := AllowedEventTypes[pref.EventType]; !ok {
			return fmt.Errorf("不支持的事件类型: %q", pref.EventType)
		}

		// Validate channels
		for _, ch := range pref.Channels {
			if !AllowedChannels[ch] {
				return fmt.Errorf("不支持的通知渠道: %q（可选: dingtalk, feishu）", ch)
			}
		}

		channelsJSON, _ := json.Marshal(pref.Channels)
		now := time.Now().Format("2006-01-02 15:04:05")

		_, err := s.database.DB.ExecContext(ctx,
			`INSERT INTO notification_preferences (user_id, event_type, channels, updated_at)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT(user_id, event_type) DO UPDATE SET channels = ?, updated_at = ?`,
			userID, pref.EventType, string(channelsJSON), now,
			string(channelsJSON), now,
		)
		if err != nil {
			return fmt.Errorf("更新通知偏好失败: %w", err)
		}
	}

	s.invalidateCache()
	return nil
}

// ---------------------------------------------------------------------------
// NotifyService integration: preference check
// ---------------------------------------------------------------------------

// ShouldNotify checks if a user wants to receive a specific event type notification
// on a specific channel. Returns true if the user has no explicit preference (default=on).
func (s *NotificationPreferenceService) ShouldNotify(ctx context.Context, userID int64, eventType, channel string) bool {
	prefs, err := s.GetPreferencesCached(ctx, userID)
	if err != nil {
		return true // on error, default to notify
	}

	// Find the pref for this event type
	for _, p := range prefs {
		if p.EventType == eventType {
			// Empty channels array = disabled
			if len(p.Channels) == 0 {
				return false
			}
			// Check if channel is in the list
			for _, ch := range p.Channels {
				if ch == channel {
					return true
				}
			}
			return false
		}
	}

	// No explicit pref = default = notify
	return true
}

// GetPreferencesCached returns preferences with a 5-minute cache to prevent N+1.
func (s *NotificationPreferenceService) GetPreferencesCached(ctx context.Context, userID int64) ([]NotificationPreference, error) {
	s.prefMu.RLock()
	if time.Since(s.prefCacheAt) < 5*time.Minute {
		// Check cache hit for this user
		if cached, ok := s.getCachedForUser(userID); ok {
			s.prefMu.RUnlock()
			return cached, nil
		}
	}
	s.prefMu.RUnlock()

	prefs, err := s.GetPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Update cache
	s.prefMu.Lock()
	for _, p := range prefs {
		key := fmt.Sprintf("%d:%s", p.UserID, p.EventType)
		s.prefCache[key] = p.Channels
	}
	s.prefCacheAt = time.Now()
	s.prefMu.Unlock()

	return prefs, nil
}

func (s *NotificationPreferenceService) getCachedForUser(userID int64) ([]NotificationPreference, bool) {
	prefs := make([]NotificationPreference, 0)
	found := false
	for evtType := range AllowedEventTypes {
		key := fmt.Sprintf("%d:%s", userID, evtType)
		if channels, ok := s.prefCache[key]; ok {
			found = true
			prefs = append(prefs, NotificationPreference{
				UserID:    userID,
				EventType: evtType,
				Channels:  channels,
			})
		}
	}
	if !found {
		return nil, false
	}
	return prefs, true
}

func (s *NotificationPreferenceService) invalidateCache() {
	s.prefMu.Lock()
	s.prefCache = make(map[string][]string)
	s.prefCacheAt = time.Time{}
	s.prefMu.Unlock()
}

// defaultPreferences returns the default preference for all event types (all channels enabled).
func (s *NotificationPreferenceService) defaultPreferences(userID int64) []NotificationPreference {
	defaultChannels := []string{"dingtalk", "feishu"}
	prefs := make([]NotificationPreference, 0, len(AllowedEventTypes))
	for evtType := range AllowedEventTypes {
		prefs = append(prefs, NotificationPreference{
			UserID:    userID,
			EventType: evtType,
			Channels:  defaultChannels,
		})
	}
	return prefs
}

// fillDefaults adds default preferences for event types that are not in the saved list.
func (s *NotificationPreferenceService) fillDefaults(saved []NotificationPreference, userID int64) []NotificationPreference {
	savedMap := make(map[string]bool, len(saved))
	for _, p := range saved {
		savedMap[p.EventType] = true
	}
	defaultChannels := []string{"dingtalk", "feishu"}
	for evtType := range AllowedEventTypes {
		if !savedMap[evtType] {
			saved = append(saved, NotificationPreference{
				UserID:    userID,
				EventType: evtType,
				Channels:  defaultChannels,
			})
		}
	}
	return saved
}

// ValidateEventType checks if the event type is in the whitelist.
func ValidateEventType(et string) error {
	if _, ok := AllowedEventTypes[et]; !ok {
		allowed := make([]string, 0, len(AllowedEventTypes))
		for k := range AllowedEventTypes {
			allowed = append(allowed, k)
		}
		return fmt.Errorf("不支持的事件类型: %q（可选: %s）", et, strings.Join(allowed, ", "))
	}
	return nil
}

// ValidateChannel checks if the channel is allowed.
func ValidateChannel(ch string) error {
	if !AllowedChannels[ch] {
		return fmt.Errorf("不支持的通知渠道: %q（可选: dingtalk, feishu）", ch)
	}
	return nil
}
