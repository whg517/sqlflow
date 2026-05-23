package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

var (
	ErrRefreshTokenInvalid = errors.New("refresh token 无效或已过期")
	ErrRefreshTokenRevoked = errors.New("refresh token 已被撤销")
)

const refreshDefaultExpiry = 7 * 24 * time.Hour // 7 days

// RefreshTokenService manages refresh token lifecycle: creation, rotation, revocation.
type RefreshTokenService struct {
	db *sql.DB
}

// NewRefreshTokenService creates a new RefreshTokenService.
func NewRefreshTokenService(db *sql.DB) *RefreshTokenService {
	return &RefreshTokenService{db: db}
}

// GenerateToken generates a new refresh token for the given user.
// Returns the raw token (to be sent to the client) and the stored record.
func (s *RefreshTokenService) GenerateToken(ctx context.Context, userID int64) (rawToken string, err error) {
	// Generate a cryptographically secure random token
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	rawToken = hex.EncodeToString(bytes)

	// Hash the token for storage (never store raw token)
	hash := sha256.Sum256([]byte(rawToken))
	hashedToken := hex.EncodeToString(hash[:])

	expiresAt := time.Now().Add(refreshDefaultExpiry)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES (?, ?, ?)`,
		userID, hashedToken, expiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("store refresh token: %w", err)
	}

	return rawToken, nil
}

// RotateToken validates the old refresh token, revokes it, and issues a new one.
// Returns a new raw token and the user ID.
func (s *RefreshTokenService) RotateToken(ctx context.Context, oldRawToken string) (newRawToken string, userID int64, err error) {
	// Hash the incoming token for lookup
	hash := sha256.Sum256([]byte(oldRawToken))
	hashedToken := hex.EncodeToString(hash[:])

	// Look up and validate the token in a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	rt := &model.RefreshToken{}
	err = tx.QueryRowContext(ctx,
		`SELECT id, user_id, token, expires_at, revoked, created_at FROM refresh_tokens WHERE token = ?`,
		hashedToken,
	).Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &rt.Revoked, &rt.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, ErrRefreshTokenInvalid
		}
		return "", 0, fmt.Errorf("lookup refresh token: %w", err)
	}

	// Check if already revoked
	if rt.Revoked {
		return "", 0, ErrRefreshTokenRevoked
	}

	// Check expiration
	if time.Now().After(rt.ExpiresAt) {
		return "", 0, ErrRefreshTokenInvalid
	}

	// Revoke the old token (token rotation)
	_, err = tx.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked = 1 WHERE id = ?`,
		rt.ID,
	)
	if err != nil {
		return "", 0, fmt.Errorf("revoke old refresh token: %w", err)
	}

	// Generate new refresh token
	newBytes := make([]byte, 32)
	if _, err := rand.Read(newBytes); err != nil {
		return "", 0, fmt.Errorf("generate random token: %w", err)
	}
	newRawToken = hex.EncodeToString(newBytes)

	newHash := sha256.Sum256([]byte(newRawToken))
	newHashedToken := hex.EncodeToString(newHash[:])

	newExpiresAt := time.Now().Add(refreshDefaultExpiry)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES (?, ?, ?)`,
		rt.UserID, newHashedToken, newExpiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return "", 0, fmt.Errorf("store new refresh token: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", 0, fmt.Errorf("commit transaction: %w", err)
	}

	return newRawToken, rt.UserID, nil
}

// RevokeAllTokens revokes all refresh tokens for a given user (e.g., on password change).
func (s *RefreshTokenService) RevokeAllTokens(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked = 1 WHERE user_id = ? AND revoked = 0`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("revoke all refresh tokens: %w", err)
	}
	return nil
}

// RevokeToken revokes a specific refresh token by its raw value.
func (s *RefreshTokenService) RevokeToken(ctx context.Context, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	hashedToken := hex.EncodeToString(hash[:])

	result, err := s.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked = 1 WHERE token = ? AND revoked = 0`,
		hashedToken,
	)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrRefreshTokenInvalid
	}
	return nil
}

// CleanupExpiredTokens deletes expired and revoked refresh tokens older than 24 hours.
func (s *RefreshTokenService) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM refresh_tokens WHERE revoked = 1 AND expires_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired tokens: %w", err)
	}
	return result.RowsAffected()
}
