// Package sqlutil builds the pagination and WHERE fragments shared by every
// domain's list query.
//
// It is deliberately domain-free: it knows about SQL shape, never about
// tickets, datasources or audit records. Keeping it here is what lets the
// domain packages stay independent of one another.
package sqlutil

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// Pagination holds normalized pagination parameters.
type Pagination struct {
	Page     int
	PageSize int
	Offset   int
}

// ParsePagination normalizes page and pageSize values.
// Defaults: page=1, pageSize=50, max pageSize=100.
func ParsePagination(page, pageSize int) Pagination {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return Pagination{
		Page:     page,
		PageSize: pageSize,
		Offset:   (page - 1) * pageSize,
	}
}

// FilterClause represents a single SQL WHERE condition with its arguments.
type FilterClause struct {
	Condition string
	Args      []interface{}
}

// BuildWhereClause constructs a WHERE clause from a slice of FilterClauses.
// Returns the WHERE clause (including "WHERE" prefix if any filters exist) and the combined args.
func BuildWhereClause(filters []FilterClause) (string, []interface{}) {
	if len(filters) == 0 {
		return "", nil
	}
	conds := make([]string, 0, len(filters))
	args := make([]interface{}, 0)
	for _, f := range filters {
		conds = append(conds, f.Condition)
		args = append(args, f.Args...)
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// PaginatedCountSQL returns a COUNT query for the given table with the WHERE clause.
func PaginatedCountSQL(table, whereClause string) string {
	return NumberPlaceholders(fmt.Sprintf("SELECT COUNT(*) FROM %s %s", table, whereClause))
}

// PaginatedQuerySQL returns a SELECT query with ORDER BY, LIMIT and OFFSET.
// The whole statement is numbered here rather than by the caller: the LIMIT and
// OFFSET placeholders come after however many the WHERE clause contributed, and
// only the assembled statement knows that count.
func PaginatedQuerySQL(selectCols, table, whereClause, orderBy string, p Pagination) string {
	return NumberPlaceholders(fmt.Sprintf(
		"%s FROM %s %s ORDER BY %s LIMIT ? OFFSET ?",
		selectCols, table, whereClause, orderBy,
	))
}

// AppendLimitArgs appends pageSize and offset to the args slice and returns it.
func AppendLimitArgs(args []interface{}, p Pagination) []interface{} {
	return append(args, p.PageSize, p.Offset)
}

// EscapeLike escapes the LIKE wildcards (%, _) and the escape character itself,
// so caller-supplied text matches literally.
//
// PostgreSQL uses backslash as the LIKE escape character by default, so callers
// need no explicit ESCAPE clause. On engines where that is not the default, one
// must be supplied or the backslashes are matched as ordinary characters.
func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// NumberPlaceholders rewrites the ? placeholders in a statement to PostgreSQL's
// positional $1, $2, … form, in order of appearance.
//
// Dynamic UPDATE and WHERE clauses are assembled fragment by fragment, and the
// number a fragment's placeholder should carry depends on how many came before
// it — which the fragment cannot know. Writing ? while building and numbering
// once at the end keeps that knowledge in one place; hand-numbering is how a
// SET clause ends up colliding with the WHERE that follows it.
//
// Placeholders inside single-quoted string literals are left alone.
func NumberPlaceholders(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	inString := false
	for _, r := range query {
		switch {
		case r == '\'':
			inString = !inString
			b.WriteRune(r)
		case r == '?' && !inString:
			n++
			b.WriteString("$")
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation.
//
// It matches on SQLSTATE 23505 rather than the message text. The previous check
// looked for SQLite's "UNIQUE constraint failed", which PostgreSQL never emits —
// a duplicate name stopped being reported as a conflict and surfaced as an
// opaque 500 instead.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
