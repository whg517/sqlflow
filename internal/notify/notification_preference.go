package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entnotificationpreference "github.com/whg517/sqlflow/internal/db/ent/notificationpreference"
)

// ---------------------------------------------------------------------------
// Constants & types
// ---------------------------------------------------------------------------

// Allowed notification event types (whitelist).
var AllowedEventTypes = map[string]string{
	"ticket_created":     "工单创建",
	"ticket_approved":    "审批通过",
	"ticket_rejected":    "审批驳回",
	"sla_warning":        "SLA 预警",
	"sla_breached":       "SLA 违规",
	"execution_complete": "执行完成",
	"execution_failed":   "执行失败",
}

// AllowedChannels are the globally configured notification channels.
var AllowedChannels = map[string]bool{
	"dingtalk": true,
	"feishu":   true,
}

// Preference represents a single user-event preference row.
type Preference struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	EventType string    `json:"event_type"`
	Channels  []string  `json:"channels"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PreferenceService manages per-user notification preferences.
type PreferenceService struct {
	client *ent.Client

	// prefCache caches per-user preferences to avoid N+1 in Service.
	// Key: "userID:eventType", Value: channels slice (nil = use defaults).
	prefCache   map[string][]string
	prefCacheAt time.Time
	prefMu      sync.RWMutex
}

// NewPreferenceService creates a new service.
func NewPreferenceService(database *db.DB) *PreferenceService {
	return &PreferenceService{
		client:    database.Client(),
		prefCache: make(map[string][]string),
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// GetPreferences returns all preferences for a user.
// If the user has no saved preferences, returns defaults for all event types.
func (s *PreferenceService) GetPreferences(ctx context.Context, userID int64) ([]Preference, error) {
	rows, err := s.client.NotificationPreference.Query().
		Where(entnotificationpreference.UserIDEQ(userID)).
		Order(ent.Asc(entnotificationpreference.FieldEventType)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询通知偏好失败: %w", err)
	}

	prefs := make([]Preference, 0, len(rows))
	for _, r := range rows {
		p := Preference{
			ID:        int64(r.ID),
			UserID:    r.UserID,
			EventType: r.EventType,
			UpdatedAt: r.UpdatedAt,
		}
		// Channels are stored as a JSON array. A row that will not parse is
		// carried through with no channels rather than dropped, so the user can
		// still see and correct the entry.
		_ = json.Unmarshal([]byte(r.Channels), &p.Channels)
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
func (s *PreferenceService) UpdatePreferences(ctx context.Context, userID int64, req UpdatePreferencesRequest) error {
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
		now := time.Now()

		// Upsert on (user_id, event_type): the UI submits the whole set every
		// time without knowing which rows already exist, so a plain insert would
		// collide and a plain update would silently do nothing for a new one.
		err := s.client.NotificationPreference.Create().
			SetUserID(userID).
			SetEventType(pref.EventType).
			SetChannels(string(channelsJSON)).
			SetUpdatedAt(now).
			OnConflictColumns(
				entnotificationpreference.FieldUserID,
				entnotificationpreference.FieldEventType,
			).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("更新通知偏好失败: %w", err)
		}
	}

	s.invalidateCache()
	return nil
}

// ---------------------------------------------------------------------------
// Service integration: preference check
// ---------------------------------------------------------------------------

// ShouldNotify checks if a user wants to receive a specific event type notification
// on a specific channel. Returns true if the user has no explicit preference (default=on).
func (s *PreferenceService) ShouldNotify(ctx context.Context, userID int64, eventType, channel string) bool {
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
func (s *PreferenceService) GetPreferencesCached(ctx context.Context, userID int64) ([]Preference, error) {
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

func (s *PreferenceService) getCachedForUser(userID int64) ([]Preference, bool) {
	prefs := make([]Preference, 0)
	found := false
	for evtType := range AllowedEventTypes {
		key := fmt.Sprintf("%d:%s", userID, evtType)
		if channels, ok := s.prefCache[key]; ok {
			found = true
			prefs = append(prefs, Preference{
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

func (s *PreferenceService) invalidateCache() {
	s.prefMu.Lock()
	s.prefCache = make(map[string][]string)
	s.prefCacheAt = time.Time{}
	s.prefMu.Unlock()
}

// defaultPreferences returns the default preference for all event types (all channels enabled).
func (s *PreferenceService) defaultPreferences(userID int64) []Preference {
	defaultChannels := []string{"dingtalk", "feishu"}
	prefs := make([]Preference, 0, len(AllowedEventTypes))
	for evtType := range AllowedEventTypes {
		prefs = append(prefs, Preference{
			UserID:    userID,
			EventType: evtType,
			Channels:  defaultChannels,
		})
	}
	return prefs
}

// fillDefaults adds default preferences for event types that are not in the saved list.
func (s *PreferenceService) fillDefaults(saved []Preference, userID int64) []Preference {
	savedMap := make(map[string]bool, len(saved))
	for _, p := range saved {
		savedMap[p.EventType] = true
	}
	defaultChannels := []string{"dingtalk", "feishu"}
	for evtType := range AllowedEventTypes {
		if !savedMap[evtType] {
			saved = append(saved, Preference{
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
