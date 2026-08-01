// Package auditlog defines the audit record every domain produces and the
// narrow interface it writes through.
//
// It exists so that domains can emit audit evidence without importing the audit
// implementation: a ticket package that depended on the audit service would drag
// reporting, full-text search and analytics along with it. Domains accept a
// Writer; only the composition root knows which implementation satisfies it.
//
// This package must stay dependency-free.
package auditlog

import (
	"context"
	"unicode/utf8"
)

// Record is one audit entry.
//
// Fields are optional by design — a login attempt has no datasource, a failed
// query has no row count. Every operation fills what it knows.
type Record struct {
	UserID             int64
	Action             string
	DatasourceID       int64
	Database           string
	SQLContent         string
	SQLSummary         string
	ResultRows         int64
	AffectedRows       int64
	ExecutionTimeMs    int64
	ErrorMessage       string
	DesensitizedFields string
	IPAddress          string
	AIReviewResult     string
	TicketID           int64
}

// Writer persists audit records.
//
// Write returns nothing: audit is evidence, not control flow. A failure to
// record must never roll back the operation that succeeded, so implementations
// log and move on. Callers therefore cannot — and must not — branch on it.
type Writer interface {
	Write(ctx context.Context, rec Record)
}

// Discard drops every record.
//
// It exists so that a caller who genuinely does not need audit evidence — a test
// asserting on something else — says so by name instead of passing nil. A nil
// Writer is indistinguishable from a wiring mistake, and this system's whole
// premise is that operations leave a trace; losing that silently is the one
// failure mode worth making impossible to write by accident.
var Discard Writer = discard{}

type discard struct{}

func (discard) Write(context.Context, Record) {}

// OrDiscard returns w, or Discard when w is nil.
//
// Constructors call it so that a missed wiring degrades to a lost audit line
// rather than a nil dereference in the middle of a request.
func OrDiscard(w Writer) Writer {
	if w == nil {
		return Discard
	}
	return w
}

// SummaryMaxRunes bounds the length of Record.SQLSummary.
const SummaryMaxRunes = 100

// Summarize shortens a statement for Record.SQLSummary.
//
// It truncates on rune boundaries, not bytes: a statement containing Chinese
// text — a comment, or a string literal in a WHERE clause — would otherwise be
// cut mid-character and stored as invalid UTF-8 in the audit trail.
func Summarize(sql string) string {
	if utf8.RuneCountInString(sql) <= SummaryMaxRunes {
		return sql
	}
	count := 0
	for i := range sql {
		if count == SummaryMaxRunes {
			return sql[:i]
		}
		count++
	}
	return sql
}
