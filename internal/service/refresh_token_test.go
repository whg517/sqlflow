package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

// setupRefreshTokenTest creates a test database with a user for refresh token tests.
func setupRefreshTokenTest(t *testing.T) (*db.DB, int64) {
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

	// Insert a test user
	result, err := database.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)", "testuser", "hash", "developer")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, _ := result.LastInsertId()

	return database, userID
}

func TestRefreshTokenService_GenerateToken(t *testing.T) {
	testDB, userID := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	rawToken, err := svc.GenerateToken(context.Background(), userID)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if rawToken == "" {
		t.Fatal("expected non-empty raw token")
	}
	if len(rawToken) < 32 {
		t.Errorf("token too short: %d chars", len(rawToken))
	}
}

func TestRefreshTokenService_GenerateToken_MultipleUsers(t *testing.T) {
	testDB, userID := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	// Insert another user
	result, _ := testDB.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)", "user2", "hash2", "admin")
	userID2, _ := result.LastInsertId()

	token1, err := svc.GenerateToken(context.Background(), userID)
	if err != nil {
		t.Fatalf("GenerateToken user1: %v", err)
	}
	token2, err := svc.GenerateToken(context.Background(), userID2)
	if err != nil {
		t.Fatalf("GenerateToken user2: %v", err)
	}

	if token1 == token2 {
		t.Error("tokens for different users should be different")
	}

	// Generate another token for same user — should be different
	token3, err := svc.GenerateToken(context.Background(), userID)
	if err != nil {
		t.Fatalf("GenerateToken user1 again: %v", err)
	}
	if token1 == token3 {
		t.Error("consecutive tokens for same user should be different")
	}
}

func TestRefreshTokenService_RotateToken(t *testing.T) {
	testDB, userID := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	ctx := context.Background()

	// Generate initial token
	oldToken, err := svc.GenerateToken(ctx, userID)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Rotate to get a new token
	newToken, rotatedUserID, err := svc.RotateToken(ctx, oldToken)
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if newToken == "" {
		t.Fatal("expected non-empty new token")
	}
	if newToken == oldToken {
		t.Error("new token should differ from old token")
	}
	if rotatedUserID != userID {
		t.Errorf("rotated userID = %d, want %d", rotatedUserID, userID)
	}

	// Old token should be revoked
	_, _, err = svc.RotateToken(ctx, oldToken)
	if err != ErrRefreshTokenRevoked {
		t.Errorf("reusing old token: err = %v, want ErrRefreshTokenRevoked", err)
	}

	// New token should work
	newToken2, _, err := svc.RotateToken(ctx, newToken)
	if err != nil {
		t.Fatalf("RotateToken with new token: %v", err)
	}
	if newToken2 == "" {
		t.Fatal("expected non-empty token after second rotation")
	}
}

func TestRefreshTokenService_RotateToken_InvalidToken(t *testing.T) {
	testDB, _ := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	_, _, err := svc.RotateToken(context.Background(), "nonexistent-token-value")
	if err != ErrRefreshTokenInvalid {
		t.Errorf("err = %v, want ErrRefreshTokenInvalid", err)
	}
}

func TestRefreshTokenService_RotateToken_EmptyToken(t *testing.T) {
	testDB, _ := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	_, _, err := svc.RotateToken(context.Background(), "")
	if err != ErrRefreshTokenInvalid {
		t.Errorf("err = %v, want ErrRefreshTokenInvalid", err)
	}
}

func TestRefreshTokenService_RevokeToken(t *testing.T) {
	testDB, userID := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	ctx := context.Background()

	rawToken, err := svc.GenerateToken(ctx, userID)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Revoke the token
	err = svc.RevokeToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	// Rotating a revoked token should fail with Revoked error
	_, _, err = svc.RotateToken(ctx, rawToken)
	if err != ErrRefreshTokenRevoked {
		t.Errorf("after revoke, rotate err = %v, want ErrRefreshTokenRevoked", err)
	}
}

func TestRefreshTokenService_RevokeToken_InvalidToken(t *testing.T) {
	testDB, _ := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	err := svc.RevokeToken(context.Background(), "invalid-token")
	if err != ErrRefreshTokenInvalid {
		t.Errorf("err = %v, want ErrRefreshTokenInvalid", err)
	}
}

func TestRefreshTokenService_RevokeAllTokens(t *testing.T) {
	testDB, userID := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	ctx := context.Background()

	// Generate multiple tokens
	tokens := make([]string, 3)
	for i := 0; i < 3; i++ {
		raw, err := svc.GenerateToken(ctx, userID)
		if err != nil {
			t.Fatalf("GenerateToken[%d]: %v", i, err)
		}
		tokens[i] = raw
	}

	// Revoke all tokens for this user
	err := svc.RevokeAllTokens(ctx, userID)
	if err != nil {
		t.Fatalf("RevokeAllTokens: %v", err)
	}

	// All tokens should now be revoked
	for i, token := range tokens {
		_, _, err := svc.RotateToken(ctx, token)
		if err != ErrRefreshTokenRevoked {
			t.Errorf("token[%d] after revoke all: err = %v, want ErrRefreshTokenRevoked", i, err)
		}
	}
}

func TestRefreshTokenService_RevokeAllTokens_NonExistentUser(t *testing.T) {
	testDB, _ := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	// Revoking all tokens for a user with no tokens should succeed (no-op)
	err := svc.RevokeAllTokens(context.Background(), 99999)
	if err != nil {
		t.Fatalf("RevokeAllTokens for nonexistent user: %v", err)
	}
}

func TestRefreshTokenService_CleanupExpiredTokens(t *testing.T) {
	testDB, userID := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	ctx := context.Background()

	// Generate a token
	_, err := svc.GenerateToken(ctx, userID)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Manually insert an already-expired and revoked token
	expiredTime := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	_, err = testDB.Exec(
		`INSERT INTO refresh_tokens (user_id, token, expires_at, revoked) VALUES (?, ?, ?, 1)`,
		userID, "expired-revoked-token-hash", expiredTime,
	)
	if err != nil {
		t.Fatalf("insert expired token: %v", err)
	}

	// Run cleanup
	deleted, err := svc.CleanupExpiredTokens(ctx)
	if err != nil {
		t.Fatalf("CleanupExpiredTokens: %v", err)
	}
	if deleted < 1 {
		t.Errorf("expected at least 1 deleted, got %d", deleted)
	}
}

func TestRefreshTokenService_CleanupExpiredTokens_NoExpired(t *testing.T) {
	testDB, userID := setupRefreshTokenTest(t)
	svc := NewRefreshTokenService(testDB)

	ctx := context.Background()

	// Generate a fresh (non-expired) token
	_, err := svc.GenerateToken(ctx, userID)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Cleanup should delete nothing
	deleted, err := svc.CleanupExpiredTokens(ctx)
	if err != nil {
		t.Fatalf("CleanupExpiredTokens: %v", err)
	}
	// Fresh token should not be cleaned up (not expired and not revoked)
	if deleted != 0 {
		t.Errorf("expected 0 deleted for fresh tokens, got %d", deleted)
	}
}
