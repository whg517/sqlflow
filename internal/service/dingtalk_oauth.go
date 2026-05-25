package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDingTalkDisabled   = errors.New("dingtalk oauth is not enabled")
	ErrDingTalkAuthFailed = errors.New("dingtalk authentication failed")
	ErrDingTalkGetUser    = errors.New("failed to get dingtalk user info")
)

// DingTalkUser represents the user info returned by DingTalk OAuth2 userinfo endpoint.
type DingTalkUser struct {
	OpenID    string `json:"openId"`
	UnionID   string `json:"unionId"`
	Nick      string `json:"nick"`
	Avatar    string `json:"avatar"`
	Email     string `json:"email"`
	Mobile    string `json:"mobile"`
	StateCode string `json:"stateCode"`
}

// DingTalkTokenResponse represents the token response from DingTalk OAuth2.
type DingTalkTokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpireIn    int64  `json:"expireIn"`
}

// DingTalkOAuthService handles DingTalk OAuth2 authentication.
type DingTalkOAuthService struct {
	db          *sql.DB
	appKey      string
	appSecret   string
	redirectURL string
	enabled     bool
	httpClient  *http.Client
	authSvc     *AuthService
}

// NewDingTalkOAuthService creates a new DingTalkOAuthService.
func NewDingTalkOAuthService(db *sql.DB, authSvc *AuthService, appKey, appSecret, redirectURL string, enabled bool) *DingTalkOAuthService {
	return &DingTalkOAuthService{
		db:          db,
		authSvc:     authSvc,
		appKey:      appKey,
		appSecret:   appSecret,
		redirectURL: redirectURL,
		enabled:     enabled,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled returns whether DingTalk OAuth is enabled.
func (s *DingTalkOAuthService) Enabled() bool {
	return s.enabled
}

// AuthURL returns the DingTalk OAuth2 authorization URL with a random state parameter.
func (s *DingTalkOAuthService) AuthURL() (string, string, error) {
	if !s.enabled {
		return "", "", ErrDingTalkDisabled
	}

	state := generateState()
	authURL := fmt.Sprintf(
		"https://login.dingtalk.com/oauth2/auth?redirect_uri=%s&response_type=code&client_id=%s&scope=openid&state=%s&prompt=consent",
		url.QueryEscape(s.redirectURL),
		s.appKey,
		state,
	)
	return authURL, state, nil
}

// HandleCallback exchanges the authorization code for a token, fetches user info,
// creates or links the user, and returns a JWT token.
func (s *DingTalkOAuthService) HandleCallback(ctx context.Context, code string) (string, *model.User, error) {
	if !s.enabled {
		return "", nil, ErrDingTalkDisabled
	}

	// Step 1: Exchange code for access token.
	accessToken, err := s.getAccessToken(code)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrDingTalkAuthFailed, err)
	}

	// Step 2: Get user info from DingTalk.
	dingUser, err := s.getUserInfo(accessToken)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrDingTalkGetUser, err)
	}

	if dingUser.OpenID == "" && dingUser.UnionID == "" {
		return "", nil, ErrDingTalkGetUser
	}

	// Step 3: Find or create user by DingTalk openID/unionID.
	user, err := s.findOrCreateUser(ctx, dingUser)
	if err != nil {
		return "", nil, fmt.Errorf("find or create user: %w", err)
	}

	// Step 4: Generate JWT token.
	token, err := s.authSvc.generateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}

	return token, user, nil
}

// getAccessToken exchanges the authorization code for an access token.
func (s *DingTalkOAuthService) getAccessToken(code string) (string, error) {
	tokenURL := "https://api.dingtalk.com/v1.0/oauth2/accessToken"

	data := url.Values{}
	data.Set("clientId", s.appKey)
	data.Set("clientSecret", s.appSecret)
	data.Set("code", code)
	data.Set("grantType", "authorization_code")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp DingTalkTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}

	return tokenResp.AccessToken, nil
}

// getUserInfo fetches the authenticated user's info from DingTalk.
func (s *DingTalkOAuthService) getUserInfo(accessToken string) (*DingTalkUser, error) {
	userURL := "https://api.dingtalk.com/v1.0/contact/users/me"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, userURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-acs-dingtalk-access-token", accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var user DingTalkUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &user, nil
}

// findOrCreateUser looks up an existing user by DingTalk openID or unionID,
// or creates a new one if this is a first-time login.
func (s *DingTalkOAuthService) findOrCreateUser(ctx context.Context, dingUser *DingTalkUser) (*model.User, error) {
	// Try to find existing user by openID first, then unionID.
	u, err := s.findByDingTalkOpenID(ctx, dingUser.OpenID)
	if err == nil {
		// Update union_id if it was previously empty and now available.
		if u.DingTalkUnionID == "" && dingUser.UnionID != "" {
			_, _ = s.db.ExecContext(ctx,
				`UPDATE users SET dingtalk_union_id = ?, updated_at = datetime('now') WHERE id = ?`,
				dingUser.UnionID, u.ID,
			)
			u.DingTalkUnionID = dingUser.UnionID
		}
		return u, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("query by openid: %w", err)
	}

	if dingUser.UnionID != "" {
		u, err = s.findByDingTalkUnionID(ctx, dingUser.UnionID)
		if err == nil {
			// Link openID to the existing account.
			if u.DingTalkUserID == "" && dingUser.OpenID != "" {
				_, _ = s.db.ExecContext(ctx,
					`UPDATE users SET dingtalk_user_id = ?, updated_at = datetime('now') WHERE id = ?`,
					dingUser.OpenID, u.ID,
				)
				u.DingTalkUserID = dingUser.OpenID
			}
			return u, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("query by unionid: %w", err)
		}
	}

	// No existing account found — create a new one.
	return s.createUserFromDingTalk(ctx, dingUser)
}

// findByDingTalkOpenID finds a user by their DingTalk openID.
func (s *DingTalkOAuthService) findByDingTalkOpenID(ctx context.Context, openID string) (*model.User, error) {
	if openID == "" {
		return nil, sql.ErrNoRows
	}
	u := &model.User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, dingtalk_user_id, dingtalk_union_id, created_at, updated_at
		 FROM users WHERE dingtalk_user_id = ?`,
		openID,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DingTalkUserID, &u.DingTalkUnionID, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

// findByDingTalkUnionID finds a user by their DingTalk unionID.
func (s *DingTalkOAuthService) findByDingTalkUnionID(ctx context.Context, unionID string) (*model.User, error) {
	if unionID == "" {
		return nil, sql.ErrNoRows
	}
	u := &model.User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, dingtalk_user_id, dingtalk_union_id, created_at, updated_at
		 FROM users WHERE dingtalk_union_id = ?`,
		unionID,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DingTalkUserID, &u.DingTalkUnionID, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

// createUserFromDingTalk creates a new user from DingTalk profile info.
// Username is generated as "dingtalk_<openID_prefix>" to ensure uniqueness.
func (s *DingTalkOAuthService) createUserFromDingTalk(ctx context.Context, dingUser *DingTalkUser) (*model.User, error) {
	// Generate a unique username from dingtalk info.
	username := generateDingTalkUsername(dingUser)

	// Generate a random password (user authenticates via OAuth, not password).
	randomPassword := generateRandomPassword(32)
	hash, err := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Use the dingtalk nick as display name if available, stored in username.
	// The generated username is guaranteed unique.
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role, dingtalk_user_id, dingtalk_union_id)
		 VALUES (?, ?, 'developer', ?, ?)`,
		username, string(hash), dingUser.OpenID, dingUser.UnionID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	id, _ := result.LastInsertId()
	_ = id
	return s.findOrCreateUser(ctx, dingUser) // re-fetch with proper columns
}

// generateDingTalkUsername creates a unique username for a DingTalk user.
func generateDingTalkUsername(dingUser *DingTalkUser) string {
	if dingUser.Nick != "" {
		// Try to use nick, but ensure it's safe for our username regex.
		safe := sanitizeUsername(dingUser.Nick)
		if len(safe) >= 3 {
			return "dt_" + safe
		}
	}
	// Fallback: use openID prefix.
	prefix := dingUser.OpenID
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	return "dt_" + prefix
}

// sanitizeUsername makes a dingtalk nick safe for use as a username suffix.
func sanitizeUsername(nick string) string {
	var sb strings.Builder
	for _, ch := range nick {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			sb.WriteRune(ch)
		} else {
			sb.WriteRune('_')
		}
	}
	result := sb.String()
	if len(result) > 28 {
		result = result[:28]
	}
	return result
}

// generateState creates a cryptographically random state string for CSRF protection.
func generateState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// generateRandomPassword creates a random password of the given length.
func generateRandomPassword(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
