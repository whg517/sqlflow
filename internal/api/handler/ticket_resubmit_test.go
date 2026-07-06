package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// Tests for ticket_resubmit.go methods on TicketHandler.
// Reuses setupTicketHandlerTest / setTicketAuthContext defined in ticket_test.go.

func TestTicketHandler_ResubmitTicket_InvalidID(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/tickets/abc/resubmit",
		strings.NewReader(`{"sql_content":"SELECT 1"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	setTicketAuthContext(c, 1, "dev", "developer")

	if err := h.ResubmitTicket(c); err != nil {
		t.Fatalf("ResubmitTicket: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid id); body=%s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestTicketHandler_ResubmitTicket_EmptySQL(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/tickets/1/resubmit",
		strings.NewReader(`{"sql_content":""}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setTicketAuthContext(c, 1, "dev", "developer")

	if err := h.ResubmitTicket(c); err != nil {
		t.Fatalf("ResubmitTicket: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (empty sql); body=%s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestTicketHandler_ResubmitTicket_NotFound(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/tickets/99999/resubmit",
		strings.NewReader(`{"sql_content":"SELECT 1","change_reason":"retry"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setTicketAuthContext(c, 1, "dev", "developer")

	if err := h.ResubmitTicket(c); err != nil {
		t.Fatalf("ResubmitTicket: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not found); body=%s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestTicketHandler_ResubmitTicket_MalformedJSON(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/tickets/1/resubmit",
		strings.NewReader(`{not-json`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setTicketAuthContext(c, 1, "dev", "developer")

	if err := h.ResubmitTicket(c); err != nil {
		t.Fatalf("ResubmitTicket: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (malformed); body=%s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestTicketHandler_ListRevisions_InvalidID(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/abc/revisions", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	setTicketAuthContext(c, 1, "dev", "developer")

	if err := h.ListRevisions(c); err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid id)", rec.Code, http.StatusBadRequest)
	}
}

func TestTicketHandler_ListRevisions_NotFound(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/99999/revisions", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setTicketAuthContext(c, 1, "dev", "developer")

	if err := h.ListRevisions(c); err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not found); body=%s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
