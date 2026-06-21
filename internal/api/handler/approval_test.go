package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestValidateApprovalChainJSON_Empty(t *testing.T) {
	cases := []string{"", "  ", "[]"}
	for _, c := range cases {
		if err := validateApprovalChainJSON(c); err != nil {
			t.Errorf("validateApprovalChainJSON(%q) = %v, want nil", c, err)
		}
	}
}

func TestValidateApprovalChainJSON_Valid(t *testing.T) {
	chain := `[{"role":"dba","auto_skip_same_submitter":true}]`
	if err := validateApprovalChainJSON(chain); err != nil {
		t.Errorf("valid chain failed: %v", err)
	}

	chain = `[{"role":"team_lead","auto_skip_same_submitter":false},{"role":"dba","auto_skip_same_submitter":true}]`
	if err := validateApprovalChainJSON(chain); err != nil {
		t.Errorf("multi-stage chain failed: %v", err)
	}
}

func TestValidateApprovalChainJSON_InvalidRole(t *testing.T) {
	chain := `[{"role":"superuser","auto_skip_same_submitter":true}]`
	if err := validateApprovalChainJSON(chain); err == nil {
		t.Error("expected error for invalid role")
	}
}

func TestValidateApprovalChainJSON_EmptyRole(t *testing.T) {
	chain := `[{"role":"","auto_skip_same_submitter":true}]`
	if err := validateApprovalChainJSON(chain); err == nil {
		t.Error("expected error for empty role")
	}
}

func TestValidateApprovalChainJSON_InvalidJSON(t *testing.T) {
	chain := `not json`
	if err := validateApprovalChainJSON(chain); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestTrimSpace(t *testing.T) {
	if trimSpace("  hello  ") != "hello" {
		t.Error("trimSpace failed")
	}
	if trimSpace("\t\nhello\r\n") != "hello" {
		t.Error("trimSpace failed for tabs/newlines")
	}
	if trimSpace("") != "" {
		t.Error("trimSpace empty string failed")
	}
}

// TestReorderRequest_Bind tests the reorder request binding.
func TestReorderRequest_Bind(t *testing.T) {
	e := echo.New()
	body := `{"priorities":{"1":10,"2":20,"3":30}}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/approval-policies/reorder", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var reqBody reorderRequest
	if err := c.Bind(&reqBody); err != nil {
		t.Fatalf("bind failed: %v", err)
	}
	if len(reqBody.Priorities) != 3 {
		t.Errorf("expected 3 priorities, got %d", len(reqBody.Priorities))
	}
}

// TestCreatePolicyRequest_Bind tests the create policy request binding.
func TestCreatePolicyRequest_Bind(t *testing.T) {
	e := echo.New()
	body := `{"name":"test-policy","description":"test","conditions":"{}","approval_chain":"[]","priority":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/approval-policies", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var reqBody createPolicyRequest
	if err := c.Bind(&reqBody); err != nil {
		t.Fatalf("bind failed: %v", err)
	}
	if reqBody.Name != "test-policy" {
		t.Errorf("name = %q", reqBody.Name)
	}
	if reqBody.Priority != 5 {
		t.Errorf("priority = %d", reqBody.Priority)
	}
}
