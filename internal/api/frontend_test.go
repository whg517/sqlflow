package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestServeFrontendUsesCurrentIndexAndRejectsMissingAssets(t *testing.T) {
	distDir := t.TempDir()
	assetsDir := filepath.Join(distDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("create assets directory: %v", err)
	}

	indexPath := filepath.Join(distDir, "index.html")
	oldAssetPath := filepath.Join(assetsDir, "index-old.js")
	if err := os.WriteFile(indexPath, []byte(`<script src="/assets/index-old.js"></script>`), 0o644); err != nil {
		t.Fatalf("write initial index: %v", err)
	}
	if err := os.WriteFile(oldAssetPath, []byte("old bundle"), 0o644); err != nil {
		t.Fatalf("write initial asset: %v", err)
	}

	e := echo.New()
	serveFrontendFromDir(e, distDir)

	assertResponseContains(t, e, "/", http.StatusOK, "index-old.js")

	if err := os.WriteFile(indexPath, []byte(`<script src="/assets/index-new.js"></script>`), 0o644); err != nil {
		t.Fatalf("replace index: %v", err)
	}
	if err := os.Remove(oldAssetPath); err != nil {
		t.Fatalf("remove old asset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "index-new.js"), []byte("new bundle"), 0o644); err != nil {
		t.Fatalf("write new asset: %v", err)
	}

	rec := assertResponseContains(t, e, "/", http.StatusOK, "index-new.js")
	if got := rec.Header().Get(echo.HeaderCacheControl); !strings.Contains(got, "no-store") {
		t.Fatalf("index cache-control = %q, want no-store", got)
	}

	assertResponseContains(t, e, "/assets/index-old.js", http.StatusNotFound, "")

	rec = assertResponseContains(t, e, "/assets/index-new.js", http.StatusOK, "new bundle")
	if got := rec.Header().Get(echo.HeaderCacheControl); !strings.Contains(got, "immutable") {
		t.Fatalf("asset cache-control = %q, want immutable", got)
	}
}

func TestServeFrontendPreservesBackendAndSharedResultRoutes(t *testing.T) {
	distDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("spa index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	e := echo.New()
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/readyz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})
	e.GET("/s/:token", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"token": c.Param("token")})
	})
	e.POST("/s/:token/verify", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]bool{"verified": true})
	})
	serveFrontendFromDir(e, distDir)

	assertResponseContains(t, e, "/healthz", http.StatusOK, `"status":"ok"`)
	assertResponseContains(t, e, "/readyz", http.StatusOK, `"status":"ready"`)
	assertResponseContains(t, e, "/s/test-token", http.StatusOK, `"token":"test-token"`)

	navigationReq := httptest.NewRequest(http.MethodGet, "/s/test-token", nil)
	navigationReq.Header.Set(echo.HeaderAccept, echo.MIMETextHTML)
	navigationRec := httptest.NewRecorder()
	e.ServeHTTP(navigationRec, navigationReq)
	if navigationRec.Code != http.StatusOK || !strings.Contains(navigationRec.Body.String(), "spa index") {
		t.Fatalf("shared page response = %d %q", navigationRec.Code, navigationRec.Body.String())
	}

	verifyReq := httptest.NewRequest(http.MethodPost, "/s/test-token/verify", strings.NewReader(`{"password":"secret"}`))
	verifyRec := httptest.NewRecorder()
	e.ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK || !strings.Contains(verifyRec.Body.String(), `"verified":true`) {
		t.Fatalf("verify response = %d %q", verifyRec.Code, verifyRec.Body.String())
	}
}

func assertResponseContains(
	t *testing.T,
	e *echo.Echo,
	path string,
	wantStatus int,
	wantBody string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != wantStatus {
		t.Fatalf("%s status = %d, want %d", path, rec.Code, wantStatus)
	}
	if wantBody != "" && !strings.Contains(rec.Body.String(), wantBody) {
		t.Fatalf("%s body = %q, want substring %q", path, rec.Body.String(), wantBody)
	}
	return rec
}
