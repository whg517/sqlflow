// Package sqlutil holds what is left of the hand-written SQL layer.
//
// It used to assemble WHERE clauses, count queries and paginated selects for
// every domain's list endpoint. Those callers moved to ent (ADR-0010), which
// builds the same statements with the column names checked at compile time, so
// the assembly helpers went with them.
//
// What remains is not about building statements:
//
//   - ParsePagination normalizes a page and size from a request, which is a
//     transport concern rather than a SQL one.
//   - EscapeLike and NumberPlaceholders serve the few expressions ent has no
//     builder for — full-text operators and the mask-rule lookup.
package sqlutil

import (
	"strconv"
	"strings"
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
