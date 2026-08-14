package query

import (
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/platform/sqlparser"
	"github.com/whg517/sqlflow/internal/resp"
)

// The error map, as data — the same shape ticket.respondError uses.
//
// Query and Explain each had a switch statement with a default that returned
// 500 "查询执行失败". Everything not explicitly listed fell through to that
// default: a permission denial, a mask refusal, an SQL syntax error from the
// target database. The user saw "查询执行失败"; the operator saw a 500 and
// could not distinguish "you lack permission" from "the platform is broken".
//
// One table consulted with errors.Is fixes both: the error's own text is
// shown (the Service wrote it for a person), and the status code tells the
// caller which class of problem it is.

var queryErrorStatuses = []struct {
	err    error
	status int
}{
	// Bad input — the user can fix this.
	{ErrEmptySQL, http.StatusBadRequest},
	{ErrSQLBlocked, http.StatusBadRequest},
	{ErrSQLTimeout, http.StatusBadRequest},
	{ErrQueryParamsUnsupported, http.StatusBadRequest},
	{ErrExportRowLimit, http.StatusBadRequest},
	{datasource.ErrDatasourceNotFound, http.StatusBadRequest},
	{datasource.ErrDatasourceDisabled, http.StatusBadRequest},
	{datasource.ErrDatabaseScopeMismatch, http.StatusBadRequest},
	{ErrExplainNotSupported, http.StatusBadRequest},
	{ErrExplainNonSelect, http.StatusBadRequest},
	// A multi-statement body is an injection-shaped client input, not a
	// platform fault — 400, not 500.
	{sqlparser.ErrMultipleStatements, http.StatusBadRequest},

	// The actor may not do this. 403, not 500.
	{ErrSQLOperationForbidden, http.StatusForbidden},
	{ErrSQLHighRisk, http.StatusForbidden},
	{ErrInternalDatasourceOnly, http.StatusForbidden},

	// The mask rules could not be read, so the result was refused. This is
	// the platform's most safety-critical refusal and it used to read
	// "查询执行失败" — indistinguishable from a broken query.
	{ErrMaskRulesUnavailable, http.StatusForbidden},
	{ErrAggregationMaskingUnsupported, http.StatusForbidden},

	// The platform is not correctly configured or a target is unreachable.
	{ErrQueryUnavailable, http.StatusServiceUnavailable},
}

// respondQueryError maps a query-domain error to its HTTP answer.
//
// An unmapped error falls to 500, logged — the same behavior as before, but
// now the set reaching it is audited: TestQueryDomainErrorsAllHaveStatusMapping
// fails on any exported Err* that is not in the table.
func respondQueryError(c echo.Context, op string, err error, fallback string) error {
	for _, m := range queryErrorStatuses {
		if errors.Is(err, m.err) {
			switch m.status {
			case http.StatusBadRequest:
				return resp.BadRequest(c, err.Error())
			case http.StatusForbidden:
				return resp.Forbidden(c, err.Error())
			case http.StatusNotFound:
				return resp.NotFound(c, err.Error())
			case http.StatusServiceUnavailable:
				return resp.ServiceUnavailable(c, err.Error())
			default:
				log.Printf("%s: %v", op, err)
				return resp.InternalError(c, err.Error())
			}
		}
	}

	log.Printf("%s failed: %v", op, err)
	return resp.InternalError(c, fallback)
}
