package ticket

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestEveryDomainErrorHasAStatus is the guard the eleven switch statements
// could not provide.
//
// Eight of the package's exported Err* had no case anywhere, so they fell
// through to a generic 500 — the Service composed a precise message for a
// person to read and the handler replaced it with "创建工单失败". Nothing caught
// that, because "did anyone add a case" is not a question a switch can be asked.
//
// This asks it of the declarations themselves: every exported Err* in the
// package must be reachable through errorStatuses. Adding a domain error and
// forgetting to map it now fails the build gate instead of shipping a 500.
func TestEveryDomainErrorHasAStatus(t *testing.T) {
	declared := declaredDomainErrors(t)
	if len(declared) == 0 {
		t.Fatal("found no exported Err* declarations; this test can no longer see its subject")
	}

	mapped := map[string]bool{}
	for _, m := range errorStatuses {
		mapped[m.err.Error()] = true
	}

	for name, err := range declared {
		if !mapped[err.Error()] {
			t.Errorf("%s has no HTTP status; it will reach the caller as a generic 500", name)
		}
	}
}

// TestRespondErrorMatchesWrappedErrors is the other half of why a switch was
// the wrong shape.
//
// `switch err` compares with ==, so a wrapped error could not be matched even
// if someone added a case for it. ErrTicketSQLUnanalyzable is returned as
// fmt.Errorf("%w: %v", ...), which made it unmappable by construction — one
// call site had already discovered this and worked around it with a
// hand-written errors.Is in front of its switch.
//
// It goes through respondError rather than the table, because the table is not
// the thing that can regress: I sabotaged this by putting == back and the first
// version of this test did not notice.
func TestRespondErrorMatchesWrappedErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"wrapped domain error", fmt.Errorf("%w: unexpected token", ErrTicketSQLUnanalyzable), 400},
		{"bare domain error", ErrNoPermission, 403},
		{"unmapped error falls back", errors.New("something else"), 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)

			if err := respondError(c, "Test", tt.err, "兜底消息"); err != nil {
				t.Fatalf("respondError: %v", err)
			}
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.status, rec.Body.String())
			}
			if tt.status != 500 && !strings.Contains(rec.Body.String(), "兜底") == false {
				t.Error("a mapped error was answered with the fallback message")
			}
		})
	}
}

// declaredDomainErrors reads the package's exported Err* variables by parsing
// the source, so the test sees what is declared rather than what someone
// remembered to list here.
func declaredDomainErrors(t *testing.T) map[string]error {
	t.Helper()

	byName := map[string]error{}
	// The values themselves come from the package; the AST is only used to learn
	// which names exist, so a new error cannot hide from this test.
	known := map[string]error{
		"ErrApprovalEngineUnavailable": ErrApprovalEngineUnavailable,
		"ErrApprovalStageConflict":     ErrApprovalStageConflict,
		"ErrCommentContentEmpty":       ErrCommentContentEmpty,
		"ErrCommentNotFound":           ErrCommentNotFound,
		"ErrCommentNotOwner":           ErrCommentNotOwner,
		"ErrOrderNotFound":             ErrOrderNotFound,
		"ErrCancelReasonRequired":      ErrCancelReasonRequired,
		"ErrInvalidStatusTransition":   ErrInvalidStatusTransition,
		"ErrNoApprovalPolicy":          ErrNoApprovalPolicy,
		"ErrNoPermission":              ErrNoPermission,
		"ErrRejectReasonRequired":      ErrRejectReasonRequired,
		"ErrScheduleTimeInPast":        ErrScheduleTimeInPast,
		"ErrScheduleTimeRequired":      ErrScheduleTimeRequired,
		"ErrSelfApproval":              ErrSelfApproval,
		"ErrSQLHashMismatch":           ErrSQLHashMismatch,
		"ErrTicketAlreadyProcessed":    ErrTicketAlreadyProcessed,
		"ErrTicketDatasourceDisabled":  ErrTicketDatasourceDisabled,
		"ErrTicketDatasourceNotFound":  ErrTicketDatasourceNotFound,
		"ErrTicketDatasourceRequired":  ErrTicketDatasourceRequired,
		"ErrTicketExecNotSupported":    ErrTicketExecNotSupported,
		"ErrTicketExecUnavailable":     ErrTicketExecUnavailable,
		"ErrTicketExecutionConflict":   ErrTicketExecutionConflict,
		"ErrTicketInternalDatasource":  ErrTicketInternalDatasource,
		"ErrTicketNotCancellable":      ErrTicketNotCancellable,
		"ErrTicketNotExecutable":       ErrTicketNotExecutable,
		"ErrTicketNotFound":            ErrTicketNotFound,
		"ErrTicketNotResubmittable":    ErrTicketNotResubmittable,
		"ErrTicketNotSchedulable":      ErrTicketNotSchedulable,
		"ErrTicketNotScheduled":        ErrTicketNotScheduled,
		"ErrTicketSQLRequired":         ErrTicketSQLRequired,
		"ErrTicketSQLUnanalyzable":     ErrTicketSQLUnanalyzable,
		"ErrTooManyStatements":         ErrTooManyStatements,
		"ErrTransitionNotDeclared":     ErrTransitionNotDeclared,
	}

	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", e.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range vs.Names {
					if !strings.HasPrefix(name.Name, "Err") || !ast.IsExported(name.Name) {
						continue
					}
					value, ok := known[name.Name]
					if !ok {
						t.Errorf("%s is declared but this test does not know it; add it to `known` "+
							"and to errorStatuses", name.Name)
						continue
					}
					byName[name.Name] = value
				}
			}
		}
	}
	return byName
}
