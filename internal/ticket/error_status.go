package ticket

import (
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/resp"
)

// The error map, as data.
//
// It was eleven `switch err` statements in one handler file, plus more in two
// others. Two problems compounded:
//
//   - Eight of the package's twenty-four exported Err* had no case anywhere, so
//     they fell to default and became a generic 500. The Service composes a
//     precise, user-facing Chinese message and the handler threw it away —
//     ErrTicketDatasourceDisabled, ErrTicketInternalDatasource,
//     ErrTooManyStatements and the rest all read as "创建工单失败".
//   - `switch err` compares with ==, so a wrapped error could not be matched at
//     all. ErrTicketSQLUnanalyzable is returned as fmt.Errorf("%w: %v", …), so
//     adding a case for it would not have worked either; only errors.Is does.
//     One call site had already discovered this and worked around it with a
//     hand-written errors.Is before its switch.
//
// One table, consulted with errors.Is, and a test that fails on any exported
// Err* missing from it. Adding a domain error and forgetting to map it is the
// mistake that keeps happening, so it is the one made impossible.

// errorStatuses maps a domain error to the HTTP status it means.
//
// The message shown is always the error's own text: the Service already wrote
// it for a person to read, and paraphrasing it in the handler is how the two
// drifted apart.
var errorStatuses = []struct {
	err    error
	status int
}{
	// Bad input from the submitter.
	{ErrTicketSQLRequired, http.StatusBadRequest},
	{ErrTicketDatasourceRequired, http.StatusBadRequest},
	{ErrTicketDatasourceNotFound, http.StatusBadRequest},
	{ErrTicketDatasourceDisabled, http.StatusBadRequest},
	{ErrTicketSQLUnanalyzable, http.StatusBadRequest},
	{ErrTooManyStatements, http.StatusBadRequest},
	{ErrRejectReasonRequired, http.StatusBadRequest},
	{ErrCancelReasonRequired, http.StatusBadRequest},
	{ErrCommentContentEmpty, http.StatusBadRequest},
	{ErrScheduleTimeInPast, http.StatusBadRequest},
	{ErrScheduleTimeRequired, http.StatusBadRequest},
	{datasource.ErrDatabaseScopeMismatch, http.StatusBadRequest},

	// The actor may not do this.
	{ErrNoPermission, http.StatusForbidden},
	{ErrSelfApproval, http.StatusForbidden},
	{ErrTicketInternalDatasource, http.StatusForbidden},
	{ErrCommentNotOwner, http.StatusForbidden},
	// Tampering with post-approval SQL is an integrity refusal, not a fault.
	{ErrSQLHashMismatch, http.StatusForbidden},

	{ErrTicketNotFound, http.StatusNotFound},
	{ErrOrderNotFound, http.StatusNotFound},
	{ErrCommentNotFound, http.StatusNotFound},

	// The ticket is not in a state where this makes sense.
	//
	// 400 rather than 409, because that is what these already answered and this
	// change is about the eight errors that answered nothing. Reclassifying the
	// ones that already had a status would be a separate decision, and one the
	// frontend would have to be told about.
	{ErrInvalidStatusTransition, http.StatusBadRequest},
	{ErrTicketAlreadyProcessed, http.StatusBadRequest},
	{ErrApprovalStageConflict, http.StatusBadRequest},
	{ErrTicketExecutionConflict, http.StatusBadRequest},
	{ErrTicketNotExecutable, http.StatusBadRequest},
	{ErrTicketNotCancellable, http.StatusBadRequest},
	{ErrTicketNotResubmittable, http.StatusBadRequest},
	{ErrTicketNotSchedulable, http.StatusBadRequest},
	{ErrTicketNotScheduled, http.StatusBadRequest},
	{ErrNoApprovalPolicy, http.StatusBadRequest},

	// The platform cannot serve this request as configured. These are the ones
	// that used to reach the caller as a generic 500 with the Service's message
	// discarded; they keep the 500 and regain the message.
	{ErrTicketExecNotSupported, http.StatusBadRequest},
	{ErrTicketExecUnavailable, http.StatusInternalServerError},
	{ErrApprovalEngineUnavailable, http.StatusInternalServerError},
	{ErrTransitionNotDeclared, http.StatusInternalServerError},
}

// respondError maps a domain error to its HTTP answer.
//
// fallback is what an unmapped error becomes — a 500 with a generic message,
// logged, exactly as before. The difference is that the set of errors reaching
// it is now checked by a test rather than by whoever last edited a switch.
func respondError(c echo.Context, op string, err error, fallback string) error {
	for _, m := range errorStatuses {
		if errors.Is(err, m.err) {
			switch m.status {
			case http.StatusBadRequest:
				return resp.BadRequest(c, err.Error())
			case http.StatusForbidden:
				return resp.Forbidden(c, err.Error())
			case http.StatusNotFound:
				return resp.NotFound(c, err.Error())
			case http.StatusConflict:
				return resp.Conflict(c, err.Error())
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
