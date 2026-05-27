package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// createTestUser creates a test user and returns the ID.
func createTokenTestUser(t *testing.T, db *sql.DB, username string) int64 {
	t.Helper()
	_, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, 'hashed', 'developer')`, username)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	var id int64
	err = db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id)
	if err != nil {
		t.Fatalf("get test user id: %v", err)
	}
	return id
}

func TestTokenService_CreateToken(t *testing.T) {
	db := setupTestDB(t)
	svc := NewTokenService(db)
	ctx := context.Background()
	userID := createTokenTestUser(t, db, "token_create")

	plainToken, token, err := svc.CreateToken(ctx, userID, "test-token", "test desc", []string{"read:query", "read:ticket"}, time.Now().Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if plainToken == "" {
		t.Fatal("expected non-empty plain token")
	}
	if !startsWithPrefix(plainToken, "sqlflow_") {
		t.Fatalf("expected token to start with sqlflow_, got: %s", plainToken[:20])
	}
	if token.Name != "test-token" {
		t.Errorf("expected name 'test-token', got '%s'", token.Name)
	}
	if token.TokenPrefix == "" {
		t.Error("expected non-empty token prefix")
	}
	if !token.IsActive {
		t.Error("expected token to be active")
	}
	if token.UseCount != 0 {
		t.Errorf("expected use_count 0, got %d", token.UseCount)
	}
}

func TestTokenService_CreateToken_DuplicateName(t *testing.T) {
	db := setupTestDB(t)
	svc := NewTokenService(db)
	ctx := context.Background()
	userID := createTokenTestUser(t, db, "token_dup")

	_, _, err := svc.CreateToken(ctx, userID, "dup-name", "", []string{"read:query"}, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("first CreateToken failed: %v", err)
	}

	_, _, err = svc.CreateToken(ctx, userID, "dup-name", "", []string{"read:query"}, time.Now().Add(24*time.Hour))
	if err != ErrTokenNameExists {
		t.Errorf("expected ErrTokenNameExists, got %v", err)
	}
}

func TestTokenService_ValidateToken(t *testing.T) {
	db := setupTestDB(t)
	svc := NewTokenService(db)
	ctx := context.Background()
	userID := createTokenTestUser(t, db, "token_validate")

	plainToken, _, err := svc.CreateToken(ctx, userID, "validate-test", "", []string{"read:query", "execute:query"}, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	// Valid token
	validUserID, username, scopes, err := svc.ValidateToken(ctx, plainToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if validUserID != userID {
		t.Errorf("expected user ID %d, got %d", userID, validUserID)
	}
	if username != "token_validate" {
		t.Errorf("expected username 'token_validate', got '%s'", username)
	}
	if len(scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(scopes))
	}

	// Invalid token
	_, _, _, err = svc.ValidateToken(ctx, "sqlflow_invalidtoken1234567890abcdef")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestTokenService_ValidateToken_Expired(t *testing.T) {
	db := setupTestDB(t)
	svc := NewTokenService(db)
	ctx := context.Background()
	userID := createTokenTestUser(t, db, "token_expired")

	// Create token that's already expired (use UTC to match SQLite)
	plainToken, _, err := svc.CreateToken(ctx, userID, "expired-token", "", []string{"read:query"}, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	_, _, _, err = svc.ValidateToken(ctx, plainToken)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestTokenService_RevokeToken(t *testing.T) {
	db := setupTestDB(t)
	svc := NewTokenService(db)
	ctx := context.Background()
	userID := createTokenTestUser(t, db, "token_revoke")

	plainToken, token, err := svc.CreateToken(ctx, userID, "revoke-test", "", []string{"read:query"}, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	// Revoke
	err = svc.RevokeToken(ctx, token.ID, userID)
	if err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}

	// Verify revoked
	_, _, _, err = svc.ValidateToken(ctx, plainToken)
	if !errors.Is(err, ErrTokenRevoked) {
		t.Errorf("expected ErrTokenRevoked, got %v", err)
	}

	// Cannot revoke with wrong user
	otherUserID := createTokenTestUser(t, db, "token_revoke_other")
	err = svc.RevokeToken(ctx, token.ID, otherUserID)
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound for wrong user, got %v", err)
	}
}

func TestTokenService_ListTokens(t *testing.T) {
	db := setupTestDB(t)
	svc := NewTokenService(db)
	ctx := context.Background()
	userID := createTokenTestUser(t, db, "token_list")

	// Create 3 tokens
	for i := 0; i < 3; i++ {
		_, _, err := svc.CreateToken(ctx, userID, "list-test-"+string(rune('A'+i)), "", []string{"read:query"}, time.Now().Add(24*time.Hour))
		if err != nil {
			t.Fatalf("CreateToken %d failed: %v", i, err)
		}
	}

	tokens, err := svc.ListTokens(ctx, userID)
	if err != nil {
		t.Fatalf("ListTokens failed: %v", err)
	}
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}

	// Create another user's token — should not appear
	otherID := createTokenTestUser(t, db, "token_list_other")
	_, _, err = svc.CreateToken(ctx, otherID, "other-token", "", []string{"read:query"}, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CreateToken other failed: %v", err)
	}
	tokens, err = svc.ListTokens(ctx, userID)
	if err != nil {
		t.Fatalf("ListTokens after other failed: %v", err)
	}
	if len(tokens) != 3 {
		t.Errorf("expected still 3 tokens for original user, got %d", len(tokens))
	}
}

func TestTokenService_HasScope(t *testing.T) {
	tests := []struct {
		scopes   []string
		required string
		want     bool
	}{
		{[]string{"read:query"}, "read:query", true},
		{[]string{"read:query"}, "execute:query", false},
		{[]string{"admin"}, "read:query", true}, // admin has all scopes
		{[]string{"read:query", "execute:query"}, "execute:query", true},
		{[]string{}, "read:query", false},
	}
	for _, tt := range tests {
		got := HasScope(tt.scopes, tt.required)
		if got != tt.want {
			t.Errorf("HasScope(%v, %s) = %v, want %v", tt.scopes, tt.required, got, tt.want)
		}
	}
}

func TestTokenService_ValidateToken_IncrementsUsage(t *testing.T) {
	db := setupTestDB(t)
	svc := NewTokenService(db)
	ctx := context.Background()
	userID := createTokenTestUser(t, db, "token_usage")

	plainToken, token, err := svc.CreateToken(ctx, userID, "usage-test", "", []string{"read:query"}, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	// Use token 3 times
	for i := 0; i < 3; i++ {
		_, _, _, err = svc.ValidateToken(ctx, plainToken)
		if err != nil {
			t.Fatalf("ValidateToken call %d failed: %v", i+1, err)
		}
	}

	// Verify use_count
	updated, err := svc.GetTokenByID(ctx, token.ID)
	if err != nil {
		t.Fatalf("GetTokenByID failed: %v", err)
	}
	if updated.UseCount != 3 {
		t.Errorf("expected use_count 3, got %d", updated.UseCount)
	}
	if updated.LastUsedAt == nil {
		t.Error("expected last_used_at to be set")
	}
}

// Helper to check string prefix.
func startsWithPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
