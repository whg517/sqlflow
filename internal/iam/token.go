package iam

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entToken "github.com/whg517/sqlflow/internal/db/ent/apitoken"
	entRole "github.com/whg517/sqlflow/internal/db/ent/role"
	entUser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"
)

var (
	ErrTokenNotFound     = errors.New("API token not found")
	ErrTokenExpired      = errors.New("API token has expired")
	ErrTokenRevoked      = errors.New("API token has been revoked")
	ErrTokenNameExists   = errors.New("token name already exists for this user")
	ErrTokenInvalidScope = errors.New("invalid token scope")
	ErrTokenScopeDenied  = errors.New("token scope exceeds user permissions")
)

// ValidAPITokenScopes lists allowed scopes for API tokens.
var ValidAPITokenScopes = []string{
	"read:query",      // read query results
	"execute:query",   // execute queries
	"read:ticket",     // read tickets
	"write:ticket",    // create/manage tickets
	"read:datasource", // read datasource metadata
	"read:audit",      // read audit logs
	"admin",           // full admin access (admin users only)
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
	if HasScope(scopes, "admin") {
		u, err := s.client.User.Get(ctx, int(userID))
		if err != nil {
			return "", nil, fmt.Errorf("check token owner role: %w", err)
		}
		if u.Role != "admin" {
			return "", nil, ErrTokenScopeDenied
		}
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
		// The row is written but unreadable — returning (plaintext, nil, nil)
		// handed callers a nil token with no error, which they dereferenced.
		// The plaintext still comes back so a caller that wants to show it can,
		// but the failure is no longer silent.
		return plainToken, nil, fmt.Errorf("read back created token: %w", err)
	}

	return plainToken, token, nil
}

// tokenRow is an api_tokens row with the owner's username joined on.
//
// The username is not a token column and there is no ent edge to users, so it
// arrives through a join and lands here rather than in the generated entity.
type tokenRow struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Username    string     `json:"username"`
	Name        string     `json:"name"`
	TokenHash   string     `json:"token_hash"`
	TokenPrefix string     `json:"token_prefix"`
	Scopes      string     `json:"scopes"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	UseCount    int64      `json:"use_count"`
	IsActive    bool       `json:"is_active"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (r tokenRow) toModel() *model.APIToken {
	return &model.APIToken{
		ID: r.ID, UserID: r.UserID, Username: r.Username, Name: r.Name,
		TokenHash: r.TokenHash, TokenPrefix: r.TokenPrefix, Scopes: r.Scopes,
		ExpiresAt: r.ExpiresAt, LastUsedAt: r.LastUsedAt, UseCount: r.UseCount,
		IsActive: r.IsActive, Description: r.Description,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// selectTokensWithUsername joins users and selects every token column plus the
// owner's username.
//
// Every listing path needs the same shape, and the column list comes from ent's
// own metadata so adding a token field cannot leave one of them behind.
func selectTokensWithUsername(sel *entsql.Selector) {
	u := entsql.Table(entUser.Table).As("u")
	sel.LeftJoin(u).On(sel.C(entToken.FieldUserID), u.C(entUser.FieldID))
	cols := make([]string, 0, len(entToken.Columns)+1)
	for _, c := range entToken.Columns {
		cols = append(cols, sel.C(c))
	}
	sel.Select(cols...).AppendSelect(entsql.As("COALESCE("+u.C(entUser.FieldUsername)+", '')", "username"))
}

// GetTokenByID retrieves a token by its ID.
func (s *TokenService) GetTokenByID(ctx context.Context, id int64) (*model.APIToken, error) {
	var rows []tokenRow
	err := s.client.APIToken.Query().
		Where(entToken.IDEQ(int(id))).
		Modify(selectTokensWithUsername).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("query token: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrTokenNotFound
	}
	return rows[0].toModel(), nil
}

// ListTokens returns all tokens for a user.
func (s *TokenService) ListTokens(ctx context.Context, userID int64) ([]*model.APIToken, error) {
	var rows []tokenRow
	err := s.client.APIToken.Query().
		Where(entToken.UserIDEQ(userID)).
		Modify(func(sel *entsql.Selector) {
			selectTokensWithUsername(sel)
			sel.OrderBy(entsql.Desc(sel.C(entToken.FieldCreatedAt)))
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("query tokens: %w", err)
	}
	return tokenRowsToModels(rows), nil
}

// tokenRowsToModels converts scanned rows to the API shape.
func tokenRowsToModels(rows []tokenRow) []*model.APIToken {
	tokens := make([]*model.APIToken, 0, len(rows))
	for _, r := range rows {
		tokens = append(tokens, r.toModel())
	}
	return tokens
}

// ListAllTokens returns all tokens (admin view).
func (s *TokenService) ListAllTokens(ctx context.Context, page, pageSize int) ([]*model.APIToken, int64, error) {
	p := sqlutil.ParsePagination(page, pageSize)

	total, err := s.client.APIToken.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count tokens: %w", err)
	}

	var rows []tokenRow
	err = s.client.APIToken.Query().
		Modify(func(sel *entsql.Selector) {
			selectTokensWithUsername(sel)
			sel.OrderBy(entsql.Asc(sel.C(entToken.FieldID))).
				Limit(p.PageSize).
				Offset(p.Offset)
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, 0, fmt.Errorf("query tokens: %w", err)
	}
	return tokenRowsToModels(rows), int64(total), nil
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
	userID, username, _, scopes, err = s.ValidateTokenWithRole(ctx, plainToken)
	return
}

// ValidateTokenWithRole validates a token and returns the owner's current role.
// Token authentication never replaces RBAC: scopes can only narrow this role.
func (s *TokenService) ValidateTokenWithRole(ctx context.Context, plainToken string) (userID int64, username, role string, scopes []string, err error) {
	hash := sha256.Sum256([]byte(plainToken))
	tokenHash := hex.EncodeToString(hash[:])

	// The role is read through a join on roles so that a disabled role denies
	// the token immediately; carrying the role from users alone would let a
	// deactivated role keep authenticating until someone reissued the token.
	var rows []struct {
		UserID    int64     `json:"user_id"`
		Username  string    `json:"username"`
		Role      string    `json:"role"`
		Scopes    string    `json:"scopes"`
		ExpiresAt time.Time `json:"expires_at"`
		IsActive  bool      `json:"is_active"`
	}
	err = s.client.APIToken.Query().
		Where(entToken.TokenHashEQ(tokenHash)).
		Modify(func(sel *entsql.Selector) {
			u := entsql.Table(entUser.Table).As("u")
			r := entsql.Table(entRole.Table).As("r")
			sel.Join(u).On(sel.C(entToken.FieldUserID), u.C(entUser.FieldID))
			sel.Join(r).OnP(entsql.And(
				entsql.ColumnsEQ(r.C(entRole.FieldName), u.C(entUser.FieldRole)),
				entsql.EQ(r.C(entRole.FieldStatus), "active"),
			))
			sel.Select(
				sel.C(entToken.FieldUserID),
				sel.C(entToken.FieldScopes),
				sel.C(entToken.FieldExpiresAt),
				sel.C(entToken.FieldIsActive),
			).AppendSelect(
				entsql.As("COALESCE("+u.C(entUser.FieldUsername)+", '')", "username"),
				entsql.As("COALESCE("+u.C(entUser.FieldRole)+", '')", "role"),
			)
		}).
		Scan(ctx, &rows)
	if err != nil {
		return 0, "", "", nil, fmt.Errorf("validate token: %w", err)
	}
	if len(rows) == 0 {
		return 0, "", "", nil, ErrTokenNotFound
	}
	t := rows[0]
	userID, username, role, scopeStr := t.UserID, t.Username, t.Role, t.Scopes

	if !t.IsActive {
		return 0, "", "", nil, ErrTokenRevoked
	}
	if time.Now().After(t.ExpiresAt) {
		return 0, "", "", nil, ErrTokenExpired
	}

	// Usage tracking is best effort: a failure here must not deny a valid token.
	// AddUseCount is an atomic increment, so concurrent requests do not lose
	// counts the way a read-modify-write would.
	_ = s.client.APIToken.Update().
		Where(entToken.TokenHashEQ(tokenHash)).
		SetLastUsedAt(time.Now()).
		AddUseCount(1).
		SetUpdatedAt(time.Now()).
		Exec(ctx)

	if scopeStr != "" {
		scopes = strings.Split(scopeStr, ",")
	}
	return userID, username, role, scopes, nil
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
			return fmt.Errorf("%w: %s", ErrTokenInvalidScope, s)
		}
	}
	return nil
}

// GetTokenStats returns aggregate token statistics for a user.
func (s *TokenService) GetTokenStats(ctx context.Context, userID int64) (totalCount, activeCount, totalUsage int64, err error) {
	owned := s.client.APIToken.Query().Where(entToken.UserIDEQ(userID))

	total, err := owned.Clone().Count(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count tokens: %w", err)
	}
	active, err := owned.Clone().Where(entToken.IsActiveEQ(true)).Count(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count active tokens: %w", err)
	}

	// Sum over an empty set is NULL, which will not scan into an int64, so the
	// aggregate is skipped when there is nothing to add up.
	if total == 0 {
		return 0, 0, 0, nil
	}
	var usage []struct {
		Sum int64 `json:"sum"`
	}
	if err := owned.Clone().
		Aggregate(ent.Sum(entToken.FieldUseCount)).
		Scan(ctx, &usage); err != nil {
		return 0, 0, 0, fmt.Errorf("sum token usage: %w", err)
	}
	if len(usage) > 0 {
		totalUsage = usage[0].Sum
	}
	return int64(total), int64(active), totalUsage, nil
}
