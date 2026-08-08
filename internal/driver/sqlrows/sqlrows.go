// Package sqlrows holds the row-reading half of a database/sql driver.
//
// It exists because MySQL and PostgreSQL had the same seventy-line function
// twice, differing on exactly two lines: the receiver type and the string in
// "<type>: not connected". Nothing about scanning a *sql.Rows into
// []map[string]interface{} varies between them — not the []byte coercion, not
// the limit break, not the elapsed-ms stamp, not the rows.Err() check — so a
// defect in any of it was a defect in both, to be found and fixed twice.
//
// What genuinely does vary stays with each driver: DSN assembly and Connect,
// statement execution (PostgreSQL wraps a batch in a transaction, MySQL
// auto-commits), EXPLAIN dialect, and the document-shaped drivers' own row
// assembly, which is not a *sql.Rows at all.
package sqlrows

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/driver"
)

const (
	// DefaultLimit bounds a query that did not ask for one.
	DefaultLimit = 1000
	// QueryTimeout bounds a single read.
	QueryTimeout = 30 * time.Second
)

// Query runs a read and reads its rows.
//
// `name` is the driver's own type name and appears only in the not-connected
// error, which is the one message that has to name who was not connected.
func Query(ctx context.Context, db *sql.DB, name, query string, args []interface{}, limit int) (*driver.QueryResult, error) {
	if db == nil {
		return nil, fmt.Errorf("%s: not connected", name)
	}

	if limit <= 0 {
		limit = DefaultLimit
	}

	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("查询超时")
		}
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("获取列信息失败: %w", err)
	}

	resultRows := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		if len(resultRows) >= limit {
			break
		}

		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("读取数据失败: %w", err)
		}

		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			// Both drivers hand back []byte for text columns, which marshals to
			// base64 rather than to a string if it is passed through as-is.
			if b, ok := values[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = values[i]
			}
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果失败: %w", err)
	}

	return &driver.QueryResult{
		Columns:       cols,
		Rows:          resultRows,
		Total:         int64(len(resultRows)),
		ExecutionTime: time.Since(start).Milliseconds(),
	}, nil
}
