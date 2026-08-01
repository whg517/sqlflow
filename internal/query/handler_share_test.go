package query

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/testutil"
)

// setupShareTest builds a fresh Echo, ShareService and ShareHandler backed by a
// per-test migrated SQLite DB. The owner user is seeded directly via raw SQL so
// ShareService (which only reads user_id/username from the request context) has
// a real user row.
func setupShareTest(t *testing.T) (*echo.Echo, *ShareService, *ShareHandler, int64) {
	t.Helper()
	d := testutil.NewDB(t)

	shareSvc := NewShareService(d, "handler-share-test-secret-at-least-32-bytes")
	h := NewShareHandler(shareSvc)
	e := echo.New()

	if _, err := d.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('sharer', 'hash', 'developer')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var userID int64 = 1
	return e, shareSvc, h, userID
}

// newShareContext builds an authenticated echo.Context for the given body.
func newShareContext(e *echo.Echo, method, path, body string, userID int64) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, userID, "sharer", "developer")
	return c, rec
}

// TestShareHandler_CreateShare_Success covers the happy path: a valid request
// returns 200 and a token identifying the new share.
func TestShareHandler_CreateShare_Success(t *testing.T) {
	e, _, h, userID := setupShareTest(t)

	body := `{"columns":["id","name"],"rows":[{"id":1,"name":"a"}],"sql_summary":"SELECT 1","datasource_name":"prod"}`
	c, rec := newShareContext(e, http.MethodPost, "/api/query/share", body, userID)
	c.SetPath("/api/query/share")

	if err := h.CreateShare(c); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v; body=%s", err, rec.Body.String())
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data object: %v", resp)
	}
	if data["token"] == nil || data["token"] == "" {
		t.Error("expected non-empty token in response")
	}
	if got, _ := data["row_count"].(float64); got != 1 {
		t.Errorf("row_count = %v, want 1", data["row_count"])
	}
}

// TestShareHandler_CreateShare_ValidationErrors exercises each 400 path.
func TestShareHandler_CreateShare_ValidationErrors(t *testing.T) {
	e, _, h, userID := setupShareTest(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty columns", `{"columns":[],"rows":[{"id":1}]}`, http.StatusBadRequest},
		{"missing columns", `{"rows":[{"id":1}]}`, http.StatusBadRequest},
		{"malformed JSON", `{not-json`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newShareContext(e, http.MethodPost, "/api/query/share", tc.body, userID)
			c.SetPath("/api/query/share")
			if err := h.CreateShare(c); err != nil {
				t.Fatalf("CreateShare returned error: %v", err)
			}
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestShareHandler_CreateShare_RowLimitTooLong hits the two service-level
// rejection branches that the handler maps to 400.
func TestShareHandler_CreateShare_ServiceLimits(t *testing.T) {
	e, _, h, userID := setupShareTest(t)

	t.Run("expiry too long (>7d)", func(t *testing.T) {
		// expires_in_hours=168 is exactly 7 days (allowed), so use 200 to exceed.
		body := `{"columns":["id"],"rows":[{"id":1}],"expires_in_hours":200}`
		c, rec := newShareContext(e, http.MethodPost, "/api/query/share", body, userID)
		c.SetPath("/api/query/share")
		if err := h.CreateShare(c); err != nil {
			t.Fatalf("CreateShare: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d (expiry too long); body=%s",
				rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("row count over limit", func(t *testing.T) {
		// Build a JSON body with 10001 rows — well over the 10000 cap.
		var b strings.Builder
		b.WriteString(`{"columns":["id"],"rows":[`)
		for i := 0; i < 10001; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`{"id":`)
			b.WriteByte('0' + byte(i%10))
			b.WriteByte('}')
		}
		b.WriteString(`]}`)
		c, rec := newShareContext(e, http.MethodPost, "/api/query/share", b.String(), userID)
		c.SetPath("/api/query/share")
		if err := h.CreateShare(c); err != nil {
			t.Fatalf("CreateShare: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d (row limit); body=%s",
				rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})
}

// TestShareHandler_GetShare_NotFound verifies the public read maps an unknown
// token to 404.
func TestShareHandler_GetShare_NotFound(t *testing.T) {
	e, _, h, _ := setupShareTest(t)

	req := httptest.NewRequest(http.MethodGet, "/s/nope", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/s/:token")
	c.SetParamNames("token")
	c.SetParamValues("nope")

	if err := h.GetShare(c); err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestShareHandler_GetShare_EmptyToken covers the guard that rejects a missing
// path param before the service is called.
func TestShareHandler_GetShare_EmptyToken(t *testing.T) {
	e, _, h, _ := setupShareTest(t)

	req := httptest.NewRequest(http.MethodGet, "/s/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/s/:token")
	c.SetParamNames("token")
	c.SetParamValues("")

	if err := h.GetShare(c); err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestShareHandler_GetShare_Success_RoundTrip creates a share via the handler,
// then reads it back through the public GetShare endpoint and asserts the
// columns/rows are returned.
func TestShareHandler_GetShare_Success_RoundTrip(t *testing.T) {
	e, _, h, userID := setupShareTest(t)

	body := `{"columns":["id","name"],"rows":[{"id":1,"name":"alice"}]}`
	c1, rec1 := newShareContext(e, http.MethodPost, "/api/query/share", body, userID)
	c1.SetPath("/api/query/share")
	if err := h.CreateShare(c1); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	if rec1.Code != http.StatusOK {
		t.Fatalf("seed share: status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	var created map[string]interface{}
	if err := json.Unmarshal(rec1.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	token, _ := created["data"].(map[string]interface{})["token"].(string)
	if token == "" {
		t.Fatal("no token returned from CreateShare")
	}

	// Public read — no auth context.
	req := httptest.NewRequest(http.MethodGet, "/s/"+token, nil)
	rec := httptest.NewRecorder()
	c2 := e.NewContext(req, rec)
	c2.SetPath("/s/:token")
	c2.SetParamNames("token")
	c2.SetParamValues(token)
	if err := h.GetShare(c2); err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get: %v; body=%s", err, rec.Body.String())
	}
	data, _ := got["data"].(map[string]interface{})
	cols, _ := data["columns"].([]interface{})
	if len(cols) != 2 {
		t.Errorf("columns len = %d, want 2", len(cols))
	}
	if got["code"].(float64) != 0 {
		t.Errorf("code = %v, want 0", got["code"])
	}
}

// TestShareHandler_VerifyPassword_WrongPassword creates a password-protected
// share and asserts a wrong password yields 401 while the right one yields 200.
func TestShareHandler_VerifyPassword(t *testing.T) {
	e, _, h, userID := setupShareTest(t)

	body := `{"columns":["id"],"rows":[{"id":1}],"password":"s3cret","sql_summary":"SELECT secret","datasource_name":"prod"}`
	c1, rec1 := newShareContext(e, http.MethodPost, "/api/query/share", body, userID)
	c1.SetPath("/api/query/share")
	if err := h.CreateShare(c1); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	if rec1.Code != http.StatusOK {
		t.Fatalf("seed share: %d %s", rec1.Code, rec1.Body.String())
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(rec1.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("decode share: %v", err)
	}
	tok, _ := parsed["data"].(map[string]interface{})["token"].(string)

	t.Run("direct read returns metadata only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/s/"+tok, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/s/:token")
		c.SetParamNames("token")
		c.SetParamValues(tok)
		if err := h.GetShare(c); err != nil {
			t.Fatalf("GetShare: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		data, _ := response["data"].(map[string]interface{})
		if granted, _ := data["access_granted"].(bool); granted {
			t.Fatal("direct read unexpectedly granted access")
		}
		if rows, _ := data["rows"].([]interface{}); len(rows) != 0 {
			t.Fatal("direct read leaked rows")
		}
		if columns, _ := data["columns"].([]interface{}); len(columns) != 0 {
			t.Fatal("direct read leaked columns")
		}
		if data["sql_summary"] != nil || data["datasource_name"] != nil {
			t.Fatal("direct read leaked protected metadata")
		}
		if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", cacheControl)
		}
	})

	t.Run("wrong password -> 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/s/"+tok+"/verify",
			strings.NewReader(`{"password":"wrong"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/s/:token/verify")
		c.SetParamNames("token")
		c.SetParamValues(tok)
		if err := h.VerifySharePassword(c); err != nil {
			t.Fatalf("VerifySharePassword: %v", err)
		}
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
	})

	t.Run("correct password -> 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/s/"+tok+"/verify",
			strings.NewReader(`{"password":"s3cret"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/s/:token/verify")
		c.SetParamNames("token")
		c.SetParamValues(tok)
		if err := h.VerifySharePassword(c); err != nil {
			t.Fatalf("VerifySharePassword: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}

		cookies := rec.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("Set-Cookie count = %d, want 1", len(cookies))
		}
		accessCookie := cookies[0]
		if !accessCookie.HttpOnly || accessCookie.SameSite != http.SameSiteStrictMode {
			t.Fatalf("access cookie flags = HttpOnly:%v SameSite:%v", accessCookie.HttpOnly, accessCookie.SameSite)
		}

		getReq := httptest.NewRequest(http.MethodGet, "/s/"+tok, nil)
		getReq.AddCookie(accessCookie)
		getRec := httptest.NewRecorder()
		getCtx := e.NewContext(getReq, getRec)
		getCtx.SetPath("/s/:token")
		getCtx.SetParamNames("token")
		getCtx.SetParamValues(tok)
		if err := h.GetShare(getCtx); err != nil {
			t.Fatalf("GetShare after verification: %v", err)
		}
		var getResponse map[string]interface{}
		if err := json.Unmarshal(getRec.Body.Bytes(), &getResponse); err != nil {
			t.Fatalf("decode verified response: %v", err)
		}
		data, _ := getResponse["data"].(map[string]interface{})
		if granted, _ := data["access_granted"].(bool); !granted {
			t.Fatal("verified request did not receive access")
		}
		if rows, _ := data["rows"].([]interface{}); len(rows) != 1 {
			t.Fatalf("verified rows len = %d, want 1", len(rows))
		}
	})
}

// TestShareHandler_VerifyPassword_NotFound ensures an unknown token is a 404.
func TestShareHandler_VerifyPassword_NotFound(t *testing.T) {
	e, _, h, _ := setupShareTest(t)

	req := httptest.NewRequest(http.MethodPost, "/s/ghost/verify",
		strings.NewReader(`{"password":"x"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/s/:token/verify")
	c.SetParamNames("token")
	c.SetParamValues("ghost")
	if err := h.VerifySharePassword(c); err != nil {
		t.Fatalf("VerifySharePassword: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestShareHandler_GetShare_ExpiredAndRevoked(t *testing.T) {
	e, shareSvc, h, userID := setupShareTest(t)

	expired, err := shareSvc.CreateShare(t.Context(), &CreateShareRequest{
		UserID:    userID,
		Username:  "sharer",
		Columns:   []string{"id"},
		Rows:      []map[string]interface{}{{"id": 1}},
		ExpiresAt: time.Now().Add(-time.Hour),
		Password:  "secret",
	})
	if err != nil {
		t.Fatalf("create expired share: %v", err)
	}

	revoked, err := shareSvc.CreateShare(t.Context(), &CreateShareRequest{
		UserID:    userID,
		Username:  "sharer",
		Columns:   []string{"id"},
		Rows:      []map[string]interface{}{{"id": 1}},
		ExpiresAt: time.Now().Add(time.Hour),
		Password:  "secret",
	})
	if err != nil {
		t.Fatalf("create revoked share: %v", err)
	}
	if err := shareSvc.RevokeShare(t.Context(), revoked.ID, userID); err != nil {
		t.Fatalf("revoke share: %v", err)
	}

	for _, tc := range []struct {
		name  string
		token string
	}{
		{name: "expired", token: expired.Token},
		{name: "revoked", token: revoked.Token},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/s/"+tc.token, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath("/s/:token")
			c.SetParamNames("token")
			c.SetParamValues(tc.token)
			if err := h.GetShare(c); err != nil {
				t.Fatalf("GetShare: %v", err)
			}
			if rec.Code != http.StatusGone {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusGone, rec.Body.String())
			}
		})
	}
}

// TestShareHandler_ListMyShares_ReturnsCreated confirms List returns the share
// created by the owner.
func TestShareHandler_ListMyShares_ReturnsCreated(t *testing.T) {
	e, _, h, userID := setupShareTest(t)

	body := `{"columns":["id"],"rows":[{"id":1}]}`
	c1, rec1 := newShareContext(e, http.MethodPost, "/api/query/share", body, userID)
	c1.SetPath("/api/query/share")
	if err := h.CreateShare(c1); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	if rec1.Code != http.StatusOK {
		t.Fatalf("seed: %d %s", rec1.Code, rec1.Body.String())
	}

	c2, rec2 := newShareContext(e, http.MethodGet, "/api/query/share", "", userID)
	c2.SetPath("/api/query/share")
	if err := h.ListMyShares(c2); err != nil {
		t.Fatalf("ListMyShares: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec2.Code, http.StatusOK, rec2.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("expected 1 share, got %d", len(data))
	}
}

// TestShareHandler_ListMyShares_EmptyUser returns an empty list (never nil)
// when the owner has no shares.
func TestShareHandler_ListMyShares_EmptyUser(t *testing.T) {
	e, _, h, userID := setupShareTest(t)

	c, rec := newShareContext(e, http.MethodGet, "/api/query/share", "", userID)
	c.SetPath("/api/query/share")
	if err := h.ListMyShares(c); err != nil {
		t.Fatalf("ListMyShares: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected 0 shares, got %d", len(data))
	}
}

// TestShareHandler_RevokeShare covers the not-found, invalid-id and success
// branches.
func TestShareHandler_RevokeShare(t *testing.T) {
	e, _, h, userID := setupShareTest(t)

	t.Run("invalid id", func(t *testing.T) {
		c, rec := newShareContext(e, http.MethodDelete, "/api/query/share/abc", "", userID)
		c.SetPath("/api/query/share/:id")
		c.SetParamNames("id")
		c.SetParamValues("abc")
		if err := h.RevokeShare(c); err != nil {
			t.Fatalf("RevokeShare: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("not found", func(t *testing.T) {
		c, rec := newShareContext(e, http.MethodDelete, "/api/query/share/99999", "", userID)
		c.SetPath("/api/query/share/:id")
		c.SetParamNames("id")
		c.SetParamValues("99999")
		if err := h.RevokeShare(c); err != nil {
			t.Fatalf("RevokeShare: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("success", func(t *testing.T) {
		// Seed a share to revoke.
		body := `{"columns":["id"],"rows":[{"id":1}]}`
		c1, rec1 := newShareContext(e, http.MethodPost, "/api/query/share", body, userID)
		c1.SetPath("/api/query/share")
		if err := h.CreateShare(c1); err != nil {
			t.Fatalf("seed: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(rec1.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("decode: %v", err)
		}
		data := parsed["data"].(map[string]interface{})
		idStr := strconv.FormatInt(int64(data["id"].(float64)), 10)

		c2, rec2 := newShareContext(e, http.MethodDelete, "/api/query/share/"+idStr, "", userID)
		c2.SetPath("/api/query/share/:id")
		c2.SetParamNames("id")
		c2.SetParamValues(idStr)
		if err := h.RevokeShare(c2); err != nil {
			t.Fatalf("RevokeShare: %v", err)
		}
		if rec2.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body=%s", rec2.Code, http.StatusOK, rec2.Body.String())
		}
	})
}
