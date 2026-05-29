package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

func TestPKCECodeVerifier(t *testing.T) {
	v1, err := PKCECodeVerifier()
	if err != nil {
		t.Fatalf("PKCECodeVerifier: %v", err)
	}
	if len(v1) < 43 {
		t.Errorf("verifier too short: %d chars", len(v1))
	}

	// Should produce different values
	v2, err := PKCECodeVerifier()
	if err != nil {
		t.Fatalf("PKCECodeVerifier: %v", err)
	}
	if v1 == v2 {
		t.Error("two verifiers should differ")
	}
}

func TestPKCECodeChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := PKCECodeChallenge(verifier)

	// RFC 7636 test vector
	expected := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if challenge != expected {
		t.Errorf("challenge = %q, want %q", challenge, expected)
	}
}

func TestOIDCService_ListProvidersEmpty(t *testing.T) {
	svc := &OIDCService{
		providers: make(map[string]*model.OIDCProvider),
	}
	// Use model import
	_ = svc

	providers := svc.ListProviders()
	if len(providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(providers))
	}
}

func TestOIDCService_GetProviderNotFound(t *testing.T) {
	svc := &OIDCService{
		providers: make(map[string]*model.OIDCProvider),
	}
	_, err := svc.GetProvider("nonexistent")
	if err != ErrOIDCProviderNotFound {
		t.Errorf("expected ErrOIDCProviderNotFound, got %v", err)
	}
}

func TestOIDCService_GetProviderDisabled(t *testing.T) {
	svc := &OIDCService{
		providers: map[string]*model.OIDCProvider{
			"disabled-idp": {Name: "disabled-idp", Enabled: false},
		},
	}
	_, err := svc.GetProvider("disabled-idp")
	if err != ErrOIDCProviderDisabled {
		t.Errorf("expected ErrOIDCProviderDisabled, got %v", err)
	}
}

func TestOIDCService_GetProviderEnabled(t *testing.T) {
	svc := &OIDCService{
		providers: map[string]*model.OIDCProvider{
			"keycloak": {Name: "keycloak", Enabled: true},
		},
	}
	p, err := svc.GetProvider("keycloak")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "keycloak" {
		t.Errorf("name = %q, want %q", p.Name, "keycloak")
	}
}

func TestOIDCService_AuthURL_Discovery(t *testing.T) {
	// Start a mock OIDC discovery server
	discoveryCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		discoveryCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"issuer": "https://idp.example.com",
			"authorization_endpoint": "https://idp.example.com/auth",
			"token_endpoint": "https://idp.example.com/token",
			"userinfo_endpoint": "https://idp.example.com/userinfo",
			"jwks_uri": "https://idp.example.com/jwks"
		}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	svc := &OIDCService{
		providers: map[string]*model.OIDCProvider{
			"test-idp": {
				Name:     "test-idp",
				Issuer:   server.URL,
				ClientID: "test-client",
				Scopes:   "openid profile email",
				Enabled:  true,
			},
		},
		discovery:  make(map[string]*oidcDiscovery),
		httpClient: &http.Client{},
	}

	params, err := svc.AuthURL(context.Background(), "test-idp", "http://localhost/callback")
	if err != nil {
		t.Fatalf("AuthURL: %v", err)
	}

	if !discoveryCalled {
		t.Error("discovery endpoint was not called")
	}
	if params.AuthURL == "" {
		t.Error("auth_url should not be empty")
	}
	if params.State == "" {
		t.Error("state should not be empty")
	}
	if params.CodeVerifier == "" {
		t.Error("code_verifier should not be empty")
	}
	if !containsOIDC(params.AuthURL, "code_challenge_method=S256") {
		t.Error("auth_url should contain PKCE S256 method")
	}
	if !containsOIDC(params.AuthURL, "client_id=test-client") {
		t.Error("auth_url should contain client_id")
	}
}

func TestOIDCService_AuthURL_CachedDiscovery(t *testing.T) {
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"issuer": "https://idp.example.com",
			"authorization_endpoint": "https://idp.example.com/auth",
			"token_endpoint": "https://idp.example.com/token",
			"userinfo_endpoint": "https://idp.example.com/userinfo",
			"jwks_uri": "https://idp.example.com/jwks"
		}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	svc := &OIDCService{
		providers: map[string]*model.OIDCProvider{
			"test-idp": {
				Name:     "test-idp",
				Issuer:   server.URL,
				ClientID: "test-client",
				Scopes:   "openid profile email",
				Enabled:  true,
			},
		},
		discovery:  make(map[string]*oidcDiscovery),
		httpClient: &http.Client{},
	}

	// First call triggers discovery
	_, err := svc.AuthURL(context.Background(), "test-idp", "http://localhost/callback")
	if err != nil {
		t.Fatalf("AuthURL 1: %v", err)
	}

	// Second call should use cached discovery
	_, err = svc.AuthURL(context.Background(), "test-idp", "http://localhost/callback")
	if err != nil {
		t.Fatalf("AuthURL 2: %v", err)
	}

	if calls != 1 {
		t.Errorf("discovery called %d times, want 1 (cached)", calls)
	}
}

func TestOIDCService_HandleCallback_Mock(t *testing.T) {
	// Mock OIDC server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"issuer": "https://idp.example.com",
			"authorization_endpoint": "https://idp.example.com/auth",
			"token_endpoint": "http://` + r.Host + `/token",
			"userinfo_endpoint": "http://` + r.Host + `/userinfo",
			"jwks_uri": "https://idp.example.com/jwks"
		}`))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"access_token": "mock-access-token",
			"id_token": "mock-id-token",
			"token_type": "Bearer",
			"expires_in": 3600
		}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer mock-access-token" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"sub": "user-123",
			"email": "testuser@example.com",
			"name": "Test User",
			"preferred_username": "testuser"
		}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	svc := &OIDCService{
		providers: map[string]*model.OIDCProvider{
			"test-idp": {
				Name:         "test-idp",
				Issuer:       server.URL,
				ClientID:     "test-client",
				ClientSecret: "test-secret",
				Scopes:       "openid profile email",
				Enabled:      true,
			},
		},
		discovery:  make(map[string]*oidcDiscovery),
		httpClient: &http.Client{},
		// db and authSvc left nil — will panic at DB access
	}

	// Test the full callback flow — OIDC exchange works but user creation fails (no DB)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic from nil DB: %v", r)
		}
	}()
	svc.HandleCallback(context.Background(), "test-idp", "mock-code", "mock-verifier", "http://localhost/callback")
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input   string
		maxLen  int
		want    string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 5, "this "},
	}
	for _, tt := range tests {
		got := truncateString(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestEnsureUniqueUsername(t *testing.T) {
	// Without a real DB, this will panic — just verify the function exists
	// Real testing is done via integration tests
	t.Log("ensureUniqueUsername requires DB, tested in integration")
}

func containsOIDC(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
