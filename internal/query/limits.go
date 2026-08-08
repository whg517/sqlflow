package query

import "time"

// Limits are the bounds the platform puts on a single query.
//
// They live here, on the service, rather than in each driver. Leaving them to
// the drivers is what the code used to do — a comment claimed "the driver
// bounds the execution itself" — and the result was five different answers:
// SQLite and Elasticsearch imposed 30 seconds, PostgreSQL imposed nothing at
// all, and MySQL's Timeout is the dial timeout, which does not bound a query
// that has already started. How long a user may hold a connection is a policy
// the operator sets, not a property of the data source, so it belongs to
// whichever layer serves every data source alike.
//
// Gathered into a struct rather than added as four more constructor arguments
// because NewService already takes seven, and a call site with eleven
// positional parameters is one where a transposed pair compiles.
type Limits struct {
	// Timeout bounds connect plus execution together, because that sum is what
	// the user actually waits and what holds a pooled connection.
	Timeout time.Duration

	// MaxRows caps an interactive query. Export and share have their own caps
	// because they are answering a different question: a grid a human reads
	// versus a file they take away.
	MaxRows       int
	ExportMaxRows int
	ShareMaxRows  int
}

// Default limits, applied to any field left at zero.
//
// These are the values that were previously compiled in, so an existing
// deployment that configures nothing keeps the behavior it had.
const (
	defaultQueryTimeout  = 30 * time.Second
	defaultRowLimit      = 1000
	defaultExportMaxRows = 10000
	defaultShareMaxRows  = 10000
)

// withDefaults fills in whatever the operator left unset.
//
// Zero is what a config loader produces for a key nobody wrote, so reading it
// literally would turn "I did not configure a timeout" into "this query may run
// forever" — an unbounded query is the one outcome an operator who skipped the
// section certainly did not ask for.
func (l Limits) withDefaults() Limits {
	if l.Timeout <= 0 {
		l.Timeout = defaultQueryTimeout
	}
	if l.MaxRows <= 0 {
		l.MaxRows = defaultRowLimit
	}
	if l.ExportMaxRows <= 0 {
		l.ExportMaxRows = defaultExportMaxRows
	}
	if l.ShareMaxRows <= 0 {
		l.ShareMaxRows = defaultShareMaxRows
	}
	return l
}
