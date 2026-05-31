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

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entToken "github.com/whg517/sqlflow/internal/db/ent/apitoken"
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
	database *db.DB
	client   *ent.Client
}

// NewTokenService creates a new TokenService.
func NewTokenService(database *db.DB) *TokenService {
	return &TokenService{
		database: database,
		client:   database.Client(),
	}
}

// CreateToken generates a new API token and stores its hash.
// Returns the plaintext token (only shown once at creation).
func (s *TokenService) CreateToken(ctx context.Context, userID int64, name, description string, scopes []string, expiresAt time.Time) (plainToken string, token *model.APIToken, err error) {
	// Validate scopes
	if err := validateScopes(scopes); err != nil {
		return "", nil, err
	}

	// Check duplicate name for same user
	count, err := s.client.APIToken.Query().
		Where(
			entToken.UserID(userID),
			entToken.Name(name),
			entToken.IsActive(true),
		).
		Count(ctx)
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

	created, err := s.client.APIToken.Create().
		SetUserID(userID).
		SetName(name).
		SetTokenHash(tokenHash).
		SetTokenPrefix(prefix).
		SetScopes(scopeStr).
		SetExpiresAt(expiresAt.UTC()).
		SetDescription(description).
		Save(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("insert token: %w", err)
	}

	token, err = s.GetTokenByID(ctx, int64(created.ID))
	if err != nil {
		return plainToken, nil, nil // token saved but lookup failed, still return plaintext
	}

	return plainToken, token, nil
}

// GetTokenByID retrieves a token by its ID.
// RAW_SQL: LEFT JOIN users for username — no ent edge defined for this relation.
func (s *TokenService) GetTokenByID(ctx context.Context, id int64) (*model.APIToken, error) {
	t := &model.APIToken{}
	var isActive int
	var lastUsedAt sql.NullTime
	var username sql.NullString

	err := s.database.DB.QueryRowContext(ctx,
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
// RAW_SQL: LEFT JOIN users for username — no ent edge defined for this relation.
func (s *TokenService) ListTokens(ctx context.Context, userID int64) ([]*model.APIToken, error) {
	query := `SELECT t.id, t.user_id, COALESCE(u.username, ''), t.name, t.token_hash, t.token_prefix,
			        t.scopes, t.expires_at, t.last_used_at, t.use_count, t.is_active,
			        COALESCE(t.description, ''), t.created_at, t.updated_at
			 FROM api_tokens t
			 LEFT JOIN users u ON u.id = t.user_id
			 WHERE t.user_id = ?
			 ORDER BY t.created_at DESC`

	rows, err := s.database.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query tokens: %w", err)
	}
	defer rows.Close()

	return scanTokens(rows)
}

// ListAllTokens returns all tokens (admin view).
// RAW_SQL: LEFT JOIN users for username — no ent edge defined for this relation.
func (s *TokenService) ListAllTokens(ctx context.Context, page, pageSize int) ([]*model.APIToken, int64, error) {
	p := ParsePagination(page, pageSize)

	var total int64
	count, err := s.client.APIToken.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count tokens: %w", err)
	}
	total = int64(count)

	querySQL := PaginatedQuerySQL(
		`SELECT t.id, t.user_id, COALESCE(u.username, ''), t.name, t.token_hash, t.token_prefix,
		        t.scopes, t.expires_at, t.last_used_at, t.use_count, t.is_active,
		        COALESCE(t.description, ''), t.created_at, t.updated_at
		 FROM api_tokens t
		 LEFT JOIN users u ON u.id = t.user_id`,
		"", "", "t.id", p,
	)
	queryArgs := AppendLimitArgs(nil, p)
	rows, err := s.database.DB.QueryContext(ctx, querySQL, queryArgs...)
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
	// First verify the token belongs to the user
	t, err := s.client.APIToken.Get(ctx, int(tokenID))
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("revoke token: %w", err)
	}
	if t.UserID != userID {
		return ErrTokenNotFound
	}
	return s.client.APIToken.UpdateOneID(int(tokenID)).
		SetIsActive(false).
		Exec(ctx)
}

// RevokeTokenAdmin deactivates any token (admin operation, soft delete).
func (s *TokenService) RevokeTokenAdmin(ctx context.Context, tokenID int64) error {
	err := s.client.APIToken.UpdateOneID(int(tokenID)).
		SetIsActive(false).
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

// ValidateToken checks if a plaintext token is valid and returns the associated user info.
// It also records usage (last_used_at, use_count).
// RAW_SQL: LEFT JOIN users for username — no ent edge defined for this relation.
func (s *TokenService) ValidateToken(ctx context.Context, plainToken string) (userID int64, username string, scopes []string, err error) {
	hash := sha256.Sum256([]byte(plainToken))
	tokenHash := hex.EncodeToString(hash[:])

	var isActive int
	var scopeStr string
	var expiresAt time.Time
	var lastUsedAt sql.NullTime

	err = s.database.DB.QueryRowContext(ctx,
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
	// RAW_SQL: atomic increment via raw SQL (use_count = use_count + 1)
	_, _ = s.database.DB.ExecContext(ctx,
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
// RAW_SQL: aggregate query with SUM/CASE — ent query builder can express this
// but raw SQL is clearer for this specific aggregation pattern.
func (s *TokenService) GetTokenStats(ctx context.Context, userID int64) (totalCount, activeCount, totalUsage int64, err error) {
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT COUNT(*), SUM(CASE WHEN is_active = 1 THEN 1 ELSE 0 END), COALESCE(SUM(use_count), 0)
		 FROM api_tokens WHERE user_id = ?`, userID,
	).Scan(&totalCount, &activeCount, &totalUsage)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("query token stats: %w", err)
	}
	return totalCount, activeCount, totalUsage, nil
}
