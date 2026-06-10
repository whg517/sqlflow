package service

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/db"
)

func setupNotifPrefDB(t *testing.T) *db.DB {
	t.Helper()
	conn := setupTestDB(t)
	database, err := db.WrapSQL(conn)
	if err != nil {
		t.Fatalf("WrapSQL: %v", err)
	}
	return database
}

func TestGetPreferences_Defaults(t *testing.T) {
	database := setupNotifPrefDB(t)
	svc := NewNotificationPreferenceService(database)
	ctx := context.Background()

	prefs, err := svc.GetPreferences(ctx, 1)
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if len(prefs) != len(AllowedEventTypes) {
		t.Errorf("expected %d default prefs, got %d", len(AllowedEventTypes), len(prefs))
	}
	// All should have both channels
	for _, p := range prefs {
		if len(p.Channels) != 2 {
			t.Errorf("event %s: expected 2 channels, got %d", p.EventType, len(p.Channels))
		}
	}
}

func TestUpdatePreferences(t *testing.T) {
	database := setupNotifPrefDB(t)
	svc := NewNotificationPreferenceService(database)
	ctx := context.Background()

	err := svc.UpdatePreferences(ctx, 1, UpdatePreferencesRequest{
		Preferences: []struct {
			EventType string   `json:"event_type"`
			Channels  []string `json:"channels"`
		}{
			{EventType: "ticket_created", Channels: []string{"feishu"}},
			{EventType: "sla_warning", Channels: []string{}}, // disabled
		},
	})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}

	prefs, _ := svc.GetPreferences(ctx, 1)
	prefMap := make(map[string]NotificationPreference)
	for _, p := range prefs {
		prefMap[p.EventType] = p
	}

	if len(prefMap["ticket_created"].Channels) != 1 || prefMap["ticket_created"].Channels[0] != "feishu" {
		t.Errorf("ticket_created channels: %v", prefMap["ticket_created"].Channels)
	}
	if len(prefMap["sla_warning"].Channels) != 0 {
		t.Errorf("sla_warning should be disabled, got: %v", prefMap["sla_warning"].Channels)
	}
	// Other event types should still have defaults
	if len(prefMap["execution_complete"].Channels) != 2 {
		t.Errorf("execution_complete should default to 2 channels, got: %v", prefMap["execution_complete"].Channels)
	}
}

func TestUpdatePreferences_InvalidEventType(t *testing.T) {
	database := setupNotifPrefDB(t)
	svc := NewNotificationPreferenceService(database)
	ctx := context.Background()

	err := svc.UpdatePreferences(ctx, 1, UpdatePreferencesRequest{
		Preferences: []struct {
			EventType string   `json:"event_type"`
			Channels  []string `json:"channels"`
		}{
			{EventType: "hacked_event", Channels: []string{"dingtalk"}},
		},
	})
	if err == nil {
		t.Error("expected error for invalid event type")
	}
}

func TestUpdatePreferences_InvalidChannel(t *testing.T) {
	database := setupNotifPrefDB(t)
	svc := NewNotificationPreferenceService(database)
	ctx := context.Background()

	err := svc.UpdatePreferences(ctx, 1, UpdatePreferencesRequest{
		Preferences: []struct {
			EventType string   `json:"event_type"`
			Channels  []string `json:"channels"`
		}{
			{EventType: "ticket_created", Channels: []string{"sms"}},
		},
	})
	if err == nil {
		t.Error("expected error for invalid channel")
	}
}

func TestShouldNotify(t *testing.T) {
	database := setupNotifPrefDB(t)
	svc := NewNotificationPreferenceService(database)
	ctx := context.Background()

	// Default: should notify all
	if !svc.ShouldNotify(ctx, 1, "ticket_created", "dingtalk") {
		t.Error("default should notify")
	}

	// Update to only feishu
	svc.UpdatePreferences(ctx, 1, UpdatePreferencesRequest{
		Preferences: []struct {
			EventType string   `json:"event_type"`
			Channels  []string `json:"channels"`
		}{
			{EventType: "ticket_created", Channels: []string{"feishu"}},
		},
	})

	// Invalidate cache to force fresh read
	svc.invalidateCache()

	// dingtalk should be blocked
	if svc.ShouldNotify(ctx, 1, "ticket_created", "dingtalk") {
		t.Error("dingtalk should be blocked after update")
	}
	// feishu should pass
	if !svc.ShouldNotify(ctx, 1, "ticket_created", "feishu") {
		t.Error("feishu should pass")
	}

	// Empty channels = disabled
	svc.UpdatePreferences(ctx, 1, UpdatePreferencesRequest{
		Preferences: []struct {
			EventType string   `json:"event_type"`
			Channels  []string `json:"channels"`
		}{
			{EventType: "sla_warning", Channels: []string{}},
		},
	})
	svc.invalidateCache()
	if svc.ShouldNotify(ctx, 1, "sla_warning", "dingtalk") {
		t.Error("disabled event should not notify")
	}
}

func TestValidateEventType(t *testing.T) {
	if err := ValidateEventType("ticket_created"); err != nil {
		t.Errorf("valid type rejected: %v", err)
	}
	if err := ValidateEventType("invalid"); err == nil {
		t.Error("invalid type should be rejected")
	}
}

func TestValidateChannel(t *testing.T) {
	if err := ValidateChannel("dingtalk"); err != nil {
		t.Errorf("valid channel rejected: %v", err)
	}
	if err := ValidateChannel("email"); err == nil {
		t.Error("invalid channel should be rejected")
	}
}
