package service

import (
	"fmt"
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
	return fmt.Sprintf("SELECT COUNT(*) FROM %s %s", table, whereClause)
}

// PaginatedQuerySQL returns a SELECT query with ORDER BY, LIMIT and OFFSET.
func PaginatedQuerySQL(selectCols, table, whereClause, orderBy string, p Pagination) string {
	return fmt.Sprintf(
		"%s FROM %s %s ORDER BY %s LIMIT ? OFFSET ?",
		selectCols, table, whereClause, orderBy,
	)
}

// AppendLimitArgs appends pageSize and offset to the args slice and returns it.
func AppendLimitArgs(args []interface{}, p Pagination) []interface{} {
	return append(args, p.PageSize, p.Offset)
}
