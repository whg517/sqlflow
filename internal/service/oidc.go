package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entOIDC "github.com/whg517/sqlflow/internal/db/ent/oidcprovider"
	entUser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/model"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrOIDCProviderNotFound = errors.New("OIDC 提供者不存在")
	ErrOIDCProviderDisabled = errors.New("OIDC 提供者已禁用")
	ErrOIDCAuthFailed       = errors.New("OIDC 认证失败")
	ErrOIDCGetUser          = errors.New("获取 OIDC 用户信息失败")
	ErrOIDCDiscoveryFailed  = errors.New("OIDC 发现端点请求失败")
)

// OIDCClaims represents standard OIDC ID token claims used for user mapping.
type OIDCClaims struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Picture           string `json:"picture"`
}

// oidcDiscovery represents the OIDC discovery document.
type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// oidcTokenResponse represents the token response from an OIDC provider.
type oidcTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// OIDCService provides generic OIDC Authorization Code Flow + PKCE authentication.
type OIDCService struct {
	database   *db.DB
	client     *ent.Client
	authSvc    *AuthService
	httpClient *http.Client

	mu        sync.RWMutex
	discovery map[string]*oidcDiscovery         // cached per provider name
	providers map[string]*model.OIDCProvider // cached per provider name
}

// NewOIDCService creates a new OIDCService.
func NewOIDCService(database *db.DB, authSvc *AuthService) *OIDCService {
	return &OIDCService{
		database:   database,
		client:     database.Client(),
		authSvc:    authSvc,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		discovery:  make(map[string]*oidcDiscovery),
		providers:  make(map[string]*model.OIDCProvider),
	}
}

// LoadProviders loads all enabled OIDC providers from the database.
func (s *OIDCService) LoadProviders(ctx context.Context) error {
	all, err := s.client.OIDCProvider.Query().
		Where(entOIDC.Enabled(true)).
		Order(ent.Asc(entOIDC.FieldName)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query oidc_providers: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear existing cache
	s.providers = make(map[string]*model.OIDCProvider)
	s.discovery = make(map[string]*oidcDiscovery)

	for _, p := range all {
		s.providers[p.Name] = entOIDCProviderToModel(p)
	}
	return nil
}

// LoadConfigProviders loads providers from config (for bootstrapping when DB is empty).
func (s *OIDCService) LoadConfigProviders(ctx context.Context, providers []ConfigOIDCProvider) error {
	for _, cp := range providers {
		if !cp.Enabled {
			continue
		}
		// Check if already exists in DB
		count, err := s.client.OIDCProvider.Query().
			Where(entOIDC.Name(cp.Name)).
			Count(ctx)
		if err != nil {
			return fmt.Errorf("check provider %s: %w", cp.Name, err)
		}
		if count > 0 {
			continue
		}
		scopes := cp.Scopes
		if scopes == "" {
			scopes = "openid profile email"
		}
		_, err = s.client.OIDCProvider.Create().
			SetName(cp.Name).
			SetIssuer(cp.Issuer).
			SetClientID(cp.ClientID).
			SetClientSecret(cp.ClientSecret).
			SetScopes(scopes).
			SetEnabled(true).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("insert provider %s: %w", cp.Name, err)
		}
	}
	return s.LoadProviders(ctx)
}

// configOIDCProvider is used internally to receive config providers.
type ConfigOIDCProvider struct {
	Name         string
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       string
	Enabled      bool
}

// ListProviders returns all enabled providers (without secrets).
func (s *OIDCService) ListProviders() []model.OIDCProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.OIDCProvider, 0, len(s.providers))
	for _, p := range s.providers {
		result = append(result, *p)
	}
	return result
}

// GetProvider returns a provider by name.
func (s *OIDCService) GetProvider(name string) (*model.OIDCProvider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.providers[name]
	if !ok {
		return nil, ErrOIDCProviderNotFound
	}
	if !p.Enabled {
		return nil, ErrOIDCProviderDisabled
	}
	return p, nil
}

// getDiscovery fetches and caches the OIDC discovery document.
func (s *OIDCService) getDiscovery(ctx context.Context, providerName string) (*oidcDiscovery, error) {
	s.mu.RLock()
	if d, ok := s.discovery[providerName]; ok {
		s.mu.RUnlock()
		return d, nil
	}
	s.mu.RUnlock()

	p, err := s.GetProvider(providerName)
	if err != nil {
		return nil, err
	}

	wellKnown := strings.TrimRight(p.Issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return nil, fmt.Errorf("create discovery request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOIDCDiscoveryFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: %d %s", ErrOIDCDiscoveryFailed, resp.StatusCode, string(body))
	}

	var disc oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		return nil, fmt.Errorf("decode discovery: %w", err)
	}

	s.mu.Lock()
	s.discovery[providerName] = &disc
	s.mu.Unlock()

	return &disc, nil
}

// PKCECodeVerifier generates a PKCE code verifier (43-128 chars, base64url).
func PKCECodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// PKCECodeChallenge computes the S256 code challenge from a verifier.
func PKCECodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// AuthURLParams holds the result of generating an authorization URL.
type AuthURLParams struct {
	AuthURL      string `json:"auth_url"`
	State        string `json:"state"`
	CodeVerifier string `json:"code_verifier,omitempty"`
}

// AuthURL generates the OIDC authorization URL with PKCE for the given provider.
func (s *OIDCService) AuthURL(ctx context.Context, providerName, redirectURL string) (*AuthURLParams, error) {
	disc, err := s.getDiscovery(ctx, providerName)
	if err != nil {
		return nil, err
	}

	p, err := s.GetProvider(providerName)
	if err != nil {
		return nil, err
	}

	state := generateState()
	verifier, err := PKCECodeVerifier()
	if err != nil {
		return nil, err
	}
	challenge := PKCECodeChallenge(verifier)

	scopes := p.Scopes
	if scopes == "" {
		scopes = "openid profile email"
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", p.ClientID)
	params.Set("redirect_uri", redirectURL)
	params.Set("scope", scopes)
	params.Set("state", state)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")

	authURL := disc.AuthorizationEndpoint + "?" + params.Encode()

	return &AuthURLParams{
		AuthURL:      authURL,
		State:        state,
		CodeVerifier: verifier,
	}, nil
}

// HandleCallback exchanges the authorization code for tokens, fetches user info,
// maps to a local user, and returns a JWT token.
func (s *OIDCService) HandleCallback(ctx context.Context, providerName, code, codeVerifier, redirectURL string) (string, *model.User, error) {
	disc, err := s.getDiscovery(ctx, providerName)
	if err != nil {
		return "", nil, err
	}

	p, err := s.GetProvider(providerName)
	if err != nil {
		return "", nil, err
	}

	// Step 1: Exchange code for tokens.
	tokenResp, err := s.exchangeCode(ctx, disc.TokenEndpoint, p, code, codeVerifier, redirectURL)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrOIDCAuthFailed, err)
	}

	// Step 2: Get user info.
	claims, err := s.getUserInfo(ctx, disc.UserinfoEndpoint, tokenResp.AccessToken)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrOIDCGetUser, err)
	}

	if claims.Sub == "" {
		return "", nil, ErrOIDCGetUser
	}

	// Step 3: Find or create user.
	user, err := s.findOrCreateUser(ctx, p.Name, claims)
	if err != nil {
		return "", nil, fmt.Errorf("find or create user: %w", err)
	}

	// Step 4: Generate JWT.
	token, err := s.authSvc.generateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}

	return token, user, nil
}

// exchangeCode exchanges the authorization code for tokens using PKCE.
func (s *OIDCService) exchangeCode(ctx context.Context, tokenEndpoint string, p *model.OIDCProvider, code, codeVerifier, redirectURL string) (*oidcTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", p.ClientID)
	data.Set("client_secret", p.ClientSecret)
	data.Set("redirect_uri", redirectURL)
	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp oidcTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response")
	}

	return &tokenResp, nil
}

// getUserInfo fetches user claims from the OIDC userinfo endpoint.
func (s *OIDCService) getUserInfo(ctx context.Context, userinfoEndpoint, accessToken string) (*OIDCClaims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send userinfo request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var claims OIDCClaims
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}

	return &claims, nil
}

// findOrCreateUser maps an OIDC identity to a local user.
func (s *OIDCService) findOrCreateUser(ctx context.Context, providerName string, claims *OIDCClaims) (*model.User, error) {
	// Try to find existing user by OIDC subject + provider.
	u, err := s.findByOIDCSubject(ctx, claims.Sub, providerName)
	if err == nil {
		return u, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("query by oidc subject: %w", err)
	}

	// Try to find by email as fallback for account linking.
	if claims.Email != "" {
		u, err = s.findByEmail(ctx, claims.Email)
		if err == nil {
			// Link OIDC identity to existing account.
			_, err = s.client.User.UpdateOneID(int(u.ID)).
				SetOidcSubject(claims.Sub).
				SetOidcProvider(providerName).
				Save(ctx)
			if err != nil {
				return nil, fmt.Errorf("link oidc identity: %w", err)
			}
			u.OIDCSubject = claims.Sub
			u.OIDCProvider = providerName
			return u, nil
		}
		if !ent.IsNotFound(err) {
			return nil, fmt.Errorf("query by email: %w", err)
		}
	}

	// No existing account — create a new one.
	return s.createUserFromOIDC(ctx, providerName, claims)
}

// findByOIDCSubject finds a user by their OIDC subject and provider.
func (s *OIDCService) findByOIDCSubject(ctx context.Context, subject, provider string) (*model.User, error) {
	if subject == "" {
		return nil, &ent.NotFoundError{}
	}
	u, err := s.client.User.Query().
		Where(
			entUser.OidcSubject(subject),
			entUser.OidcProvider(provider),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return entUserToModel(u), nil
}

// findByEmail finds a user by email stored in username field (convention).
func (s *OIDCService) findByEmail(ctx context.Context, email string) (*model.User, error) {
	u, err := s.client.User.Query().
		Where(entUser.Username(email)).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return entUserToModel(u), nil
}

// createUserFromOIDC creates a new user from OIDC claims.
func (s *OIDCService) createUserFromOIDC(ctx context.Context, providerName string, claims *OIDCClaims) (*model.User, error) {
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		username = fmt.Sprintf("oidc_%s_%s", providerName, truncateString(claims.Sub, 12))
	}

	// Ensure uniqueness
	username = s.ensureUniqueUsername(ctx, username)

	// Random password (user authenticates via OIDC, not password)
	randomPassword := generateRandomPassword(32)
	hash, err := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	_, err = s.client.User.Create().
		SetUsername(username).
		SetPasswordHash(string(hash)).
		SetRole("developer").
		SetOidcSubject(claims.Sub).
		SetOidcProvider(providerName).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	// Re-fetch to get all columns
	return s.findByOIDCSubject(ctx, claims.Sub, providerName)
}

// ensureUniqueUsername appends a numeric suffix if the username already exists.
func (s *OIDCService) ensureUniqueUsername(ctx context.Context, username string) string {
	base := username
	for i := 1; i <= 100; i++ {
		count, err := s.client.User.Query().
			Where(entUser.Username(username)).
			Count(ctx)
		if err != nil || count == 0 {
			return username
		}
		username = fmt.Sprintf("%s_%d", base, i)
	}
	return base + "_overflow"
}

// InvalidateDiscovery clears cached discovery documents for a provider.
func (s *OIDCService) InvalidateDiscovery(providerName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.discovery, providerName)
}

// generateState creates a random state parameter for OAuth2.
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

// truncateString truncates a string to maxLen.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// entOIDCProviderToModel converts an ent OIDCProvider entity to a model.OIDCProvider.
func entOIDCProviderToModel(p *ent.OIDCProvider) *model.OIDCProvider {
	return &model.OIDCProvider{
		ID:           int64(p.ID),
		Name:         p.Name,
		Issuer:       p.Issuer,
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Scopes:       p.Scopes,
		Enabled:      p.Enabled,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}
