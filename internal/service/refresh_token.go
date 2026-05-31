package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entRefresh "github.com/whg517/sqlflow/internal/db/ent/refreshtoken"
	"github.com/whg517/sqlflow/internal/model"
)

var (
	ErrRefreshTokenInvalid = errors.New("refresh token 无效或已过期")
	ErrRefreshTokenRevoked = errors.New("refresh token 已被撤销")
)

const refreshDefaultExpiry = 7 * 24 * time.Hour // 7 days

// RefreshTokenService manages refresh token lifecycle: creation, rotation, revocation.
type RefreshTokenService struct {
	database *db.DB
	client   *ent.Client
}

// NewRefreshTokenService creates a new RefreshTokenService.
func NewRefreshTokenService(database *db.DB) *RefreshTokenService {
	return &RefreshTokenService{
		database: database,
		client:   database.Client(),
	}
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

	_, err = s.client.RefreshToken.Create().
		SetUserID(userID).
		SetToken(hashedToken).
		SetExpiresAt(expiresAt.UTC()).
		Save(ctx)
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
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rt, err := tx.RefreshToken.Query().
		Where(entRefresh.Token(hashedToken)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
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
	_, err = tx.RefreshToken.UpdateOneID(rt.ID).
		SetRevoked(true).
		Save(ctx)
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

	_, err = tx.RefreshToken.Create().
		SetUserID(rt.UserID).
		SetToken(newHashedToken).
		SetExpiresAt(newExpiresAt.UTC()).
		Save(ctx)
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
	_, err := s.client.RefreshToken.Update().
		Where(
			entRefresh.UserID(userID),
			entRefresh.Revoked(false),
		).
		SetRevoked(true).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("revoke all refresh tokens: %w", err)
	}
	return nil
}

// RevokeToken revokes a specific refresh token by its raw value.
func (s *RefreshTokenService) RevokeToken(ctx context.Context, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	hashedToken := hex.EncodeToString(hash[:])

	n, err := s.client.RefreshToken.Update().
		Where(
			entRefresh.Token(hashedToken),
			entRefresh.Revoked(false),
		).
		SetRevoked(true).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	if n == 0 {
		return ErrRefreshTokenInvalid
	}
	return nil
}

// CleanupExpiredTokens deletes expired and revoked refresh tokens older than 24 hours.
func (s *RefreshTokenService) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-24 * time.Hour).UTC()
	n, err := s.client.RefreshToken.Delete().
		Where(
			entRefresh.Revoked(true),
			entRefresh.ExpiresAtLT(cutoff),
		).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired tokens: %w", err)
	}
	return int64(n), nil
}

// entRefreshTokenToModel converts an ent RefreshToken entity to a model.RefreshToken.
func entRefreshTokenToModel(rt *ent.RefreshToken) *model.RefreshToken {
	return &model.RefreshToken{
		ID:        int64(rt.ID),
		UserID:    rt.UserID,
		Token:     rt.Token,
		ExpiresAt: rt.ExpiresAt,
		Revoked:   rt.Revoked,
		CreatedAt: rt.CreatedAt,
	}
}
