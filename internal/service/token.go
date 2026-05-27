package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

var (
	ErrTokenNotFound   = errors.New("API token not found")
	ErrTokenExpired    = errors.New("API token has expired")
	ErrTokenRevoked    = errors.New("API token has been revoked")
	ErrTokenNameExists = errors.New("token name already exists for this user")
)

// ValidAPITokenScopes lists allowed scopes for API tokens.
var ValidAPITokenScopes = []string{
	"read:query",       // read query results
	"execute:query",    // execute queries
	"read:ticket",      // read tickets
	"write:ticket",     // create/manage tickets
	"read:datasource",  // read datasource metadata
	"read:audit",       // read audit logs
	"admin",            // full admin access (admin users only)
}

// TokenService manages API tokens for external integrations.
type TokenService struct {
	db *sql.DB
}

// NewTokenService creates a new TokenService.
func NewTokenService(db *sql.DB) *TokenService {
	return &TokenService{db: db}
}

// CreateToken generates a new API token and stores its hash.
// Returns the plaintext token (only shown once at creation).
func (s *TokenService) CreateToken(ctx context.Context, userID int64, name, description string, scopes []string, expiresAt time.Time) (plainToken string, token *model.APIToken, err error) {
	// Validate scopes
	if err := validateScopes(scopes); err != nil {
		return "", nil, err
	}

	// Check duplicate name for same user
	var count int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_tokens WHERE user_id = ? AND name = ? AND is_active = 1`,
		userID, name,
	).Scan(&count)
	if err != nil {
		return "", nil, fmt.Errorf("check token name: %w", err)
	}
	if count > 0 {
		return "", nil, ErrTokenNameExists
	}

	// Generate random token: "sqlflow_" + 32 random hex bytes
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	plainToken = "sqlflow_" + hex.EncodeToString(rawBytes)

	// Hash the token for storage
	hash := sha256.Sum256([]byte(plainToken))
	tokenHash := hex.EncodeToString(hash[:])

	// Prefix for display (first 8 chars after prefix)
	prefix := plainToken[:15] // "sqlflow_" + first 8 hex chars

	scopeStr := strings.Join(scopes, ",")

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash, token_prefix, scopes, expires_at, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, name, tokenHash, prefix, scopeStr, expiresAt.Format("2006-01-02 15:04:05"), description,
	)
	if err != nil {
		return "", nil, fmt.Errorf("insert token: %w", err)
	}

	id, _ := result.LastInsertId()
	token, err = s.GetTokenByID(ctx, id)
	if err != nil {
		return plainToken, nil, nil // token saved but lookup failed, still return plaintext
	}

	return plainToken, token, nil
}

// GetTokenByID retrieves a token by its ID.
func (s *TokenService) GetTokenByID(ctx context.Context, id int64) (*model.APIToken, error) {
	t := &model.APIToken{}
	var isActive int
	var lastUsedAt sql.NullTime
	var username sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT t.id, t.user_id, u.username, t.name, t.token_hash, t.token_prefix,
		        t.scopes, t.expires_at, t.last_used_at, t.use_count, t.is_active,
		        t.description, t.created_at, t.updated_at
		 FROM api_tokens t
		 LEFT JOIN users u ON u.id = t.user_id
		 WHERE t.id = ?`,
		id,
	).Scan(&t.ID, &t.UserID, &username, &t.Name, &t.TokenHash, &t.TokenPrefix,
		&t.Scopes, &t.ExpiresAt, &lastUsedAt, &t.UseCount, &isActive,
		&t.Description, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("query token: %w", err)
	}

	t.IsActive = isActive == 1
	if username.Valid {
		t.Username = username.String
	}
	if lastUsedAt.Valid {
		t.LastUsedAt = &lastUsedAt.Time
	}

	return t, nil
}

// ListTokens returns all active tokens for a user.
func (s *TokenService) ListTokens(ctx context.Context, userID int64) ([]*model.APIToken, error) {
	query := `SELECT t.id, t.user_id, COALESCE(u.username, ''), t.name, t.token_hash, t.token_prefix,
			        t.scopes, t.expires_at, t.last_used_at, t.use_count, t.is_active,
			        COALESCE(t.description, ''), t.created_at, t.updated_at
			 FROM api_tokens t
			 LEFT JOIN users u ON u.id = t.user_id
			 WHERE t.user_id = ?
			 ORDER BY t.created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query tokens: %w", err)
	}
	defer rows.Close()

	return scanTokens(rows)
}

// ListAllTokens returns all tokens (admin view).
func (s *TokenService) ListAllTokens(ctx context.Context, page, pageSize int) ([]*model.APIToken, int64, error) {
	p := ParsePagination(page, pageSize)

	var total int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_tokens`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count tokens: %w", err)
	}

	querySQL := PaginatedQuerySQL(
		`SELECT t.id, t.user_id, COALESCE(u.username, ''), t.name, t.token_hash, t.token_prefix,
		        t.scopes, t.expires_at, t.last_used_at, t.use_count, t.is_active,
		        COALESCE(t.description, ''), t.created_at, t.updated_at
		 FROM api_tokens t
		 LEFT JOIN users u ON u.id = t.user_id`,
		"", "", "t.id", p,
	)
	queryArgs := AppendLimitArgs(nil, p)
	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query tokens: %w", err)
	}
	defer rows.Close()

	tokens, err := scanTokens(rows)
	if err != nil {
		return nil, 0, err
	}
	return tokens, total, nil
}

// RevokeToken deactivates a token.
func (s *TokenService) RevokeToken(ctx context.Context, tokenID, userID int64) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE api_tokens SET is_active = 0, updated_at = datetime('now') WHERE id = ? AND user_id = ?`,
		tokenID, userID,
	)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrTokenNotFound
	}
	return nil
}

// RevokeTokenAdmin deactivates any token (admin operation).
func (s *TokenService) RevokeTokenAdmin(ctx context.Context, tokenID int64) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE api_tokens SET is_active = 0, updated_at = datetime('now') WHERE id = ?`,
		tokenID,
	)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrTokenNotFound
	}
	return nil
}

// ValidateToken checks if a plaintext token is valid and returns the associated user info.
// It also records usage (last_used_at, use_count).
func (s *TokenService) ValidateToken(ctx context.Context, plainToken string) (userID int64, username string, scopes []string, err error) {
	hash := sha256.Sum256([]byte(plainToken))
	tokenHash := hex.EncodeToString(hash[:])

	var isActive int
	var scopeStr string
	var expiresAt time.Time
	var lastUsedAt sql.NullTime

	err = s.db.QueryRowContext(ctx,
		`SELECT t.user_id, COALESCE(u.username, ''), t.scopes, t.expires_at, t.is_active, t.last_used_at
		 FROM api_tokens t
		 LEFT JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = ?`,
		tokenHash,
	).Scan(&userID, &username, &scopeStr, &expiresAt, &isActive, &lastUsedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", nil, ErrTokenNotFound
		}
		return 0, "", nil, fmt.Errorf("validate token: %w", err)
	}

	if isActive != 1 {
		return 0, "", nil, ErrTokenRevoked
	}

	if time.Now().After(expiresAt) {
		return 0, "", nil, ErrTokenExpired
	}

	// Update usage (best-effort, don't fail auth on update error)
	_, _ = s.db.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = datetime('now'), use_count = use_count + 1, updated_at = datetime('now') WHERE token_hash = ?`,
		tokenHash,
	)

	if scopeStr != "" {
		scopes = strings.Split(scopeStr, ",")
	}
	return userID, username, scopes, nil
}

// HasScope checks if the given scopes contain the required scope.
func HasScope(scopes []string, required string) bool {
	for _, s := range scopes {
		if s == "admin" || s == required {
			return true
		}
	}
	return false
}

// validateScopes checks that all provided scopes are valid.
func validateScopes(scopes []string) error {
	for _, s := range scopes {
		valid := false
		for _, v := range ValidAPITokenScopes {
			if s == v {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid scope: %s", s)
		}
	}
	return nil
}

// scanTokens scans token rows from a prepared query.
func scanTokens(rows *sql.Rows) ([]*model.APIToken, error) {
	var tokens []*model.APIToken
	for rows.Next() {
		t := &model.APIToken{}
		var isActive int
		var lastUsedAt sql.NullTime

		err := rows.Scan(&t.ID, &t.UserID, &t.Username, &t.Name, &t.TokenHash, &t.TokenPrefix,
			&t.Scopes, &t.ExpiresAt, &lastUsedAt, &t.UseCount, &isActive,
			&t.Description, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}

		t.IsActive = isActive == 1
		if lastUsedAt.Valid {
			t.LastUsedAt = &lastUsedAt.Time
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tokens: %w", err)
	}
	return tokens, nil
}

// GetTokenStats returns aggregate token statistics for a user.
func (s *TokenService) GetTokenStats(ctx context.Context, userID int64) (totalCount, activeCount, totalUsage int64, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), SUM(CASE WHEN is_active = 1 THEN 1 ELSE 0 END), COALESCE(SUM(use_count), 0)
		 FROM api_tokens WHERE user_id = ?`, userID,
	).Scan(&totalCount, &activeCount, &totalUsage)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("query token stats: %w", err)
	}
	return totalCount, activeCount, totalUsage, nil
}
