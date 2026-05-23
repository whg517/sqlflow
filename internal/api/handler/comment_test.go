package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/service"
)

func setupCommentHandlerTest(t *testing.T) (*echo.Echo, *service.CommentService, *CommentHandler, int64) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert a test user
	userRes, err := database.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)", "commenter", "hash", "developer")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, _ := userRes.LastInsertId()

	// Insert a test datasource
	dsRes, err := database.Exec("INSERT INTO datasources (name, type, host, port, username, password_encrypted, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"test-ds", "mysql", "10.0.0.1", 3306, "root", "enc", "active")
	if err != nil {
		t.Fatalf("insert datasource: %v", err)
	}
	dsID, _ := dsRes.LastInsertId()

	// Insert a test ticket
	ticketRes, err := database.Exec(
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, risk_level, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, dsID, "testdb", "SELECT 1", "SELECT 1", "mysql", "test", "low", model.TicketStatusDone,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	ticketID, _ := ticketRes.LastInsertId()

	commentSvc := service.NewCommentService(database.DB)
	handler := NewCommentHandler(commentSvc)
	e := echo.New()

	return e, commentSvc, handler, ticketID
}

func TestCommentHandler_ListComments(t *testing.T) {
	e, commentSvc, h, ticketID := setupCommentHandlerTest(t)
	ctx := contextWithTimeout(t)

	// Create some comments
	commentSvc.CreateComment(ctx, ticketID, 1, "First comment", 0)
	commentSvc.CreateComment(ctx, ticketID, 1, "Second comment", 0)

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/"+intToStr(ticketID)+"/comments", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(intToStr(ticketID))

	if err := h.ListComments(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array; body=%s", rec.Body.String())
	}
	if len(data) != 2 {
		t.Errorf("expected 2 comments, got %d", len(data))
	}
}

func TestCommentHandler_ListComments_InvalidTicketID(t *testing.T) {
	e, _, h, _ := setupCommentHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/abc/comments", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")

	if err := h.ListComments(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCommentHandler_CreateComment(t *testing.T) {
	e, _, h, ticketID := setupCommentHandlerTest(t)

	body := `{"content": "Great work!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/"+intToStr(ticketID)+"/comments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(intToStr(ticketID))
	c.Set(middleware.ContextKeyUserID, int64(1))
	c.Set(middleware.ContextKeyRole, "developer")

	if err := h.CreateComment(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestCommentHandler_CreateComment_EmptyContent(t *testing.T) {
	e, _, h, ticketID := setupCommentHandlerTest(t)

	body := `{"content": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/"+intToStr(ticketID)+"/comments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(intToStr(ticketID))
	c.Set(middleware.ContextKeyUserID, int64(1))
	c.Set(middleware.ContextKeyRole, "developer")

	if err := h.CreateComment(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCommentHandler_CreateComment_InvalidJSON(t *testing.T) {
	e, _, h, ticketID := setupCommentHandlerTest(t)

	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/"+intToStr(ticketID)+"/comments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(intToStr(ticketID))
	c.Set(middleware.ContextKeyUserID, int64(1))
	c.Set(middleware.ContextKeyRole, "developer")

	if err := h.CreateComment(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCommentHandler_CreateComment_InvalidTicketID(t *testing.T) {
	e, _, h, _ := setupCommentHandlerTest(t)

	body := `{"content": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/abc/comments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	c.Set(middleware.ContextKeyUserID, int64(1))
	c.Set(middleware.ContextKeyRole, "developer")

	if err := h.CreateComment(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCommentHandler_CreateComment_WithParentID(t *testing.T) {
	e, commentSvc, h, ticketID := setupCommentHandlerTest(t)
	ctx := contextWithTimeout(t)

	// Create parent comment first
	parent, err := commentSvc.CreateComment(ctx, ticketID, 1, "Parent", 0)
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	body := `{"content": "Reply", "parent_id": ` + intToStr(parent.ID) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/"+intToStr(ticketID)+"/comments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(intToStr(ticketID))
	c.Set(middleware.ContextKeyUserID, int64(1))
	c.Set(middleware.ContextKeyRole, "developer")

	if err := h.CreateComment(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestCommentHandler_DeleteComment(t *testing.T) {
	e, commentSvc, h, ticketID := setupCommentHandlerTest(t)
	ctx := contextWithTimeout(t)

	// Create a comment to delete
	comment, err := commentSvc.CreateComment(ctx, ticketID, 1, "Delete me", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/comments/"+intToStr(comment.ID), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(intToStr(comment.ID))
	c.Set(middleware.ContextKeyUserID, int64(1))
	c.Set(middleware.ContextKeyRole, "developer")

	if err := h.DeleteComment(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestCommentHandler_DeleteComment_NotFound(t *testing.T) {
	e, _, h, _ := setupCommentHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/comments/99999", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	c.Set(middleware.ContextKeyUserID, int64(1))
	c.Set(middleware.ContextKeyRole, "developer")

	if err := h.DeleteComment(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestCommentHandler_DeleteComment_InvalidID(t *testing.T) {
	e, _, h, _ := setupCommentHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/comments/abc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	c.Set(middleware.ContextKeyUserID, int64(1))
	c.Set(middleware.ContextKeyRole, "developer")

	if err := h.DeleteComment(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCommentHandler_DeleteComment_Forbidden(t *testing.T) {
	e, commentSvc, h, ticketID := setupCommentHandlerTest(t)
	ctx := contextWithTimeout(t)

	// Create a comment as user 1
	comment, err := commentSvc.CreateComment(ctx, ticketID, 1, "Owner comment", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// Try to delete as a different non-admin user
	req := httptest.NewRequest(http.MethodDelete, "/api/comments/"+intToStr(comment.ID), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(intToStr(comment.ID))
	c.Set(middleware.ContextKeyUserID, int64(999)) // different user
	c.Set(middleware.ContextKeyRole, "developer")   // not admin/dba

	if err := h.DeleteComment(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// helper to convert int64 to string
func intToStr(n int64) string {
	return fmt.Sprintf("%d", n)
}
