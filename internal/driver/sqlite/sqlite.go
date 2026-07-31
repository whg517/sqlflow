// Package sqlite implements read-only access to local SQLite data sources.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
	_ "modernc.org/sqlite"
)

func init() {
	driver.Register("sqlite", func() driver.Driver { return &Driver{} })
}

// Driver implements the SQLFlow driver contract for SQLite.
// Connections are always opened in read-only/query-only mode.
type Driver struct {
	db *sql.DB
}

func (d *Driver) Type() string { return "sqlite" }

func (d *Driver) Capabilities() driver.CapabilitySet {
	return driver.CapabilitySet(
		driver.CapQuery |
			driver.CapMetadata |
			driver.CapTableLevelPermission |
			driver.CapFieldMasking |
			driver.CapSQLParse |
			driver.CapExport,
	)
}

func (d *Driver) Connect(ctx context.Context, cfg *driver.Config) error {
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("sqlite database path is required")
	}

	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)", cfg.Database)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping sqlite: %w", err)
	}
	d.db = db
	return nil
}

func (d *Driver) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *Driver) Ping(ctx context.Context) error {
	if d.db == nil {
		return fmt.Errorf("sqlite: not connected")
	}
	return d.db.PingContext(ctx)
}

func (d *Driver) ListDatabases(context.Context) ([]string, error) {
	if d.db == nil {
		return nil, fmt.Errorf("sqlite: not connected")
	}
	return []string{"main"}, nil
}

func (d *Driver) ListTables(ctx context.Context, _ string) ([]driver.TableInfo, error) {
	if d.db == nil {
		return nil, fmt.Errorf("sqlite: not connected")
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT name
		FROM sqlite_schema
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list sqlite tables: %w", err)
	}
	defer rows.Close()

	tables := make([]driver.TableInfo, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan sqlite table: %w", err)
		}
		tables = append(tables, driver.TableInfo{Name: name})
	}
	return tables, rows.Err()
}

func (d *Driver) GetColumns(ctx context.Context, _, table string) ([]driver.ColumnInfo, error) {
	if d.db == nil {
		return nil, fmt.Errorf("sqlite: not connected")
	}
	quotedTable := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
	rows, err := d.db.QueryContext(ctx, "PRAGMA table_info("+quotedTable+")")
	if err != nil {
		return nil, fmt.Errorf("get sqlite columns: %w", err)
	}
	defer rows.Close()

	columns := make([]driver.ColumnInfo, 0)
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal interface{}
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan sqlite column: %w", err)
		}
		columns = append(columns, driver.ColumnInfo{Name: name, Type: columnType})
	}
	return columns, rows.Err()
}

func (d *Driver) ExecuteQuery(ctx context.Context, _ string, query string, limit int) (*driver.QueryResult, error) {
	return d.executeQuery(ctx, query, nil, limit)
}

func (d *Driver) ExecuteQueryWithArgs(ctx context.Context, _ string, query string, args []interface{}, limit int) (*driver.QueryResult, error) {
	return d.executeQuery(ctx, query, args, limit)
}

func (d *Driver) executeQuery(ctx context.Context, query string, args []interface{}, limit int) (*driver.QueryResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("sqlite: not connected")
	}
	if limit <= 0 {
		limit = 1000
	}

	start := time.Now()
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("执行 SQLite 查询失败: %w", err)
	}
	defer rows.Close()

	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("获取 SQLite 列信息失败: %w", err)
	}
	resultRows := make([]map[string]interface{}, 0, limit)
	for rows.Next() && len(resultRows) < limit {
		values := make([]interface{}, len(columnNames))
		pointers := make([]interface{}, len(columnNames))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, fmt.Errorf("读取 SQLite 数据失败: %w", err)
		}
		row := make(map[string]interface{}, len(columnNames))
		for i, name := range columnNames {
			if bytes, ok := values[i].([]byte); ok {
				row[name] = string(bytes)
			} else {
				row[name] = values[i]
			}
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 SQLite 结果失败: %w", err)
	}
	return &driver.QueryResult{
		Columns:       columnNames,
		Rows:          resultRows,
		Total:         int64(len(resultRows)),
		ExecutionTime: time.Since(start).Milliseconds(),
	}, nil
}

func (d *Driver) ExecuteStatement(context.Context, string, string) (*driver.StatementResult, error) {
	return nil, fmt.Errorf("SQLite 数据源仅支持只读查询")
}

func (d *Driver) ExecuteStatements(context.Context, string, []string) ([]driver.StatementResult, error) {
	return nil, fmt.Errorf("SQLite 数据源仅支持只读查询")
}

func (d *Driver) Parse(query string) (*driver.ParseResult, error) {
	result, err := sqlparser.ParseSQL(query, "sqlite")
	if err != nil {
		return nil, err
	}
	return &driver.ParseResult{
		Operation:   string(result.Operation),
		Targets:     result.Tables,
		RiskLevel:   string(result.RiskLevel),
		IsBlocked:   result.IsBlocked,
		BlockReason: result.BlockReason,
		Warnings:    result.Warnings,
	}, nil
}
