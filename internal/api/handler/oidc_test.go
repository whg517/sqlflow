package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/service"
)

func TestOIDCHandler_Providers(t *testing.T) {
	svc := &service.OIDCService{}
	h := NewOIDCHandler(svc)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/providers", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Providers(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestOIDCHandler_Login_MissingProvider(t *testing.T) {
	svc := &service.OIDCService{}
	h := NewOIDCHandler(svc)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestOIDCHandler_Login_MissingRedirectURI(t *testing.T) {
	svc := &service.OIDCService{}
	h := NewOIDCHandler(svc)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/keycloak", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("provider")
	c.SetParamValues("keycloak")

	if err := h.Login(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestOIDCHandler_Callback_MissingCode(t *testing.T) {
	svc := &service.OIDCService{}
	h := NewOIDCHandler(svc)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/keycloak/callback", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("provider")
	c.SetParamValues("keycloak")

	if err := h.Callback(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestOIDCHandler_Callback_MissingCodeVerifier(t *testing.T) {
	svc := &service.OIDCService{}
	h := NewOIDCHandler(svc)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/keycloak/callback?code=testcode&redirect_uri=http://localhost/cb", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("provider")
	c.SetParamValues("keycloak")

	if err := h.Callback(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestNewOIDCHandler(t *testing.T) {
	svc := &service.OIDCService{}
	h := NewOIDCHandler(svc)

	if h == nil {
		t.Fatal("NewOIDCHandler returned nil")
	}
	if h.oidcSvc != svc {
		t.Error("oidcSvc not set correctly")
	}
}
