package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/service"
)

func TestDingTalkHandler_Login_Enabled(t *testing.T) {
	// Create a disabled service (no real DB needed for this test)
	svc := &service.DingTalkOAuthService{}
	h := NewDingTalkHandler(svc)
	e := echo.New()

	// Login with disabled service
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/dingtalk/login", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestDingTalkHandler_Callback_MissingCode(t *testing.T) {
	svc := &service.DingTalkOAuthService{}
	h := NewDingTalkHandler(svc)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/dingtalk/callback", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Callback(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestDingTalkHandler_Callback_Disabled(t *testing.T) {
	svc := &service.DingTalkOAuthService{}
	h := NewDingTalkHandler(svc)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/dingtalk/callback?code=testcode", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Callback(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestDingTalkHandler_Enabled_Disabled(t *testing.T) {
	svc := &service.DingTalkOAuthService{}
	h := NewDingTalkHandler(svc)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/dingtalk/enabled", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Enabled(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["enabled"] {
		t.Error("expected enabled=false")
	}
}

func TestDingTalkHandler_NewDingTalkHandler(t *testing.T) {
	svc := &service.DingTalkOAuthService{}
	h := NewDingTalkHandler(svc)

	if h == nil {
		t.Fatal("NewDingTalkHandler returned nil")
	}
	if h.dingSvc != svc {
		t.Error("dingSvc not set correctly")
	}
}
