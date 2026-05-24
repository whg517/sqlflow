package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/whg517/sqlflow/internal/db"
)

func setupDingTalkTest(t *testing.T) (*sql.DB, *DingTalkOAuthService) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	authSvc, _ := newTestAuthService(t)
	svc := NewDingTalkOAuthService(database.DB, authSvc, "test-key", "test-secret", "http://localhost/callback", true)

	return database.DB, svc
}

func TestDingTalkOAuthService_Enabled(t *testing.T) {
	_, svc := setupDingTalkTest(t)

	if !svc.Enabled() {
		t.Error("expected Enabled() = true")
	}

	// Test disabled
	disabledSvc := NewDingTalkOAuthService(nil, nil, "", "", "", false)
	if disabledSvc.Enabled() {
		t.Error("expected Enabled() = false when disabled")
	}
}

func TestDingTalkOAuthService_AuthURL(t *testing.T) {
	_, svc := setupDingTalkTest(t)

	authURL, state, err := svc.AuthURL()
	if err != nil {
		t.Fatalf("AuthURL: %v", err)
	}
	if authURL == "" {
		t.Error("expected non-empty auth URL")
	}
	if state == "" {
		t.Error("expected non-empty state")
	}
	if len(state) != 32 {
		t.Errorf("state length = %d, want 32", len(state))
	}
}

func TestDingTalkOAuthService_AuthURL_Disabled(t *testing.T) {
	disabledSvc := NewDingTalkOAuthService(nil, nil, "", "", "", false)

	_, _, err := disabledSvc.AuthURL()
	if err != ErrDingTalkDisabled {
		t.Errorf("err = %v, want ErrDingTalkDisabled", err)
	}
}

func TestDingTalkOAuthService_HandleCallback_Disabled(t *testing.T) {
	disabledSvc := NewDingTalkOAuthService(nil, nil, "", "", "", false)

	_, _, err := disabledSvc.HandleCallback(context.Background(), "code")
	if err != ErrDingTalkDisabled {
		t.Errorf("err = %v, want ErrDingTalkDisabled", err)
	}
}

func TestDingTalkOAuthService_FindByDingTalkOpenID_NotFound(t *testing.T) {
	_, svc := setupDingTalkTest(t)

	_, err := svc.findByDingTalkOpenID(context.Background(), "nonexistent-openid")
	if err != sql.ErrNoRows {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestDingTalkOAuthService_FindByDingTalkUnionID_NotFound(t *testing.T) {
	_, svc := setupDingTalkTest(t)

	_, err := svc.findByDingTalkUnionID(context.Background(), "nonexistent-unionid")
	if err != sql.ErrNoRows {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestSanitizeUsername(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello", "Hello"},
		{"hello_world", "hello_world"},
		{"你好世界", "____"},
		{"user@name!", "user_name_"},
		{"a", "a"},
		{"ab", "ab"},
		{"", ""},
		{"very_long_username_that_exceeds_twenty_eight_chars", "very_long_username_that_exce"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeUsername(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeUsername(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateDingTalkUsername(t *testing.T) {
	tests := []struct {
		name   string
		nick   string
		openID string
		want   string
	}{
		{"with_nick", "JohnDoe", "abc123", "dt_JohnDoe"},
		{"short_nick", "ab", "abc123", "dt_abc123"},                      // nick too short, falls back
		{"empty_nick", "", "def456ghi", "dt_def456ghi"},                  // no nick
		{"long_openid", "nick", "abcdefghijklmnopqrstuvwxyz", "dt_nick"}, // openid truncated
		{"chinese_nick", "张三", "abc123", "dt_abc123"},                    // non-ascii nick
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &DingTalkUser{Nick: tt.nick, OpenID: tt.openID}
			got := generateDingTalkUsername(user)
			if got != tt.want {
				t.Errorf("generateDingTalkUsername(nick=%q, openID=%q) = %q, want %q", tt.nick, tt.openID, got, tt.want)
			}
		})
	}
}

func TestGenerateState(t *testing.T) {
	state1 := generateState()
	state2 := generateState()

	if state1 == "" {
		t.Error("state should not be empty")
	}
	if len(state1) != 32 {
		t.Errorf("state length = %d, want 32", len(state1))
	}
	if state1 == state2 {
		t.Error("two generated states should be different")
	}
}

func TestGenerateRandomPassword(t *testing.T) {
	pw1 := generateRandomPassword(32)
	pw2 := generateRandomPassword(32)

	if len(pw1) != 64 { // hex encoded 32 bytes = 64 chars
		t.Errorf("password length = %d, want 64", len(pw1))
	}
	if pw1 == pw2 {
		t.Error("two generated passwords should be different")
	}
}
