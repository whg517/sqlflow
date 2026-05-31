// Package mysql implements the Driver interface for MySQL data sources.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

func init() {
	driver.Register("mysql", func() driver.Driver { return &MySQLDriver{} })
}

// MySQLDriver implements driver.Driver for MySQL.
type MySQLDriver struct {
	db *sql.DB
}

// Type returns "mysql".
func (d *MySQLDriver) Type() string { return "mysql" }

// Capabilities declares MySQL's full capability set.
func (d *MySQLDriver) Capabilities() driver.CapabilitySet {
	return driver.CapabilitySet(
		driver.CapQuery |
			driver.CapTicketExec |
			driver.CapMetadata |
			driver.CapTableLevelPermission |
			driver.CapFieldMasking |
			driver.CapSQLParse |
			driver.CapExport,
	)
}

// Connect establishes a connection pool to the MySQL server.
func (d *MySQLDriver) Connect(ctx context.Context, cfg *driver.Config) error {
	dbName := cfg.Database
	if dbName == "" {
		dbName = "information_schema"
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=30s&parseTime=true",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpen)
	db.SetMaxIdleConns(cfg.MaxIdle)
	if cfg.MaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.MaxLifetime)
	}
	if cfg.MaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.MaxIdleTime)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping mysql: %w", err)
	}

	d.db = db
	return nil
}

// Close releases the connection pool.
func (d *MySQLDriver) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Ping verifies the connection is alive.
func (d *MySQLDriver) Ping(ctx context.Context) error {
	if d.db == nil {
		return fmt.Errorf("mysql: not connected")
	}
	return d.db.PingContext(ctx)
}

// ListDatabases returns all databases on the server.
func (d *MySQLDriver) ListDatabases(ctx context.Context) ([]string, error) {
	if d.db == nil {
		return nil, fmt.Errorf("mysql: not connected")
	}
	rows, err := d.db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, fmt.Errorf("show databases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var dbs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan database name: %w", err)
		}
		dbs = append(dbs, name)
	}
	return dbs, rows.Err()
}

// ListTables returns all tables in the given database.
func (d *MySQLDriver) ListTables(ctx context.Context, database string) ([]driver.TableInfo, error) {
	if d.db == nil {
		return nil, fmt.Errorf("mysql: not connected")
	}

	// Switch database context if needed
	query := "SHOW TABLES"
	if database != "" {
		// Use table_schema filter for explicit database
		query = fmt.Sprintf("SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = '%s'", database)
	}

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []driver.TableInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, driver.TableInfo{Name: name})
	}
	return tables, rows.Err()
}

// GetColumns returns column metadata for a specific table.
func (d *MySQLDriver) GetColumns(ctx context.Context, database, table string) ([]driver.ColumnInfo, error) {
	if d.db == nil {
		return nil, fmt.Errorf("mysql: not connected")
	}

	query := "SELECT COLUMN_NAME, DATA_TYPE, COLUMN_COMMENT FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = ?"
	args := []interface{}{table}
	if database != "" {
		query += " AND TABLE_SCHEMA = ?"
		args = append(args, database)
	}
	query += " ORDER BY ORDINAL_POSITION"

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var columns []driver.ColumnInfo
	for rows.Next() {
		var col driver.ColumnInfo
		if err := rows.Scan(&col.Name, &col.Type, &col.Comment); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		columns = append(columns, col)
	}
	return columns, rows.Err()
}

// ExecuteQuery executes a read-only SQL query.
func (d *MySQLDriver) ExecuteQuery(ctx context.Context, database string, query string, limit int) (*driver.QueryResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("mysql: not connected")
	}

	if limit <= 0 {
		limit = 1000
	}

	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, err := d.db.QueryContext(ctx, query)
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
	rowCount := 0
	for rows.Next() {
		if rowCount >= limit {
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

		row := make(map[string]interface{})
		for i, col := range cols {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		resultRows = append(resultRows, row)
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果失败: %w", err)
	}

	elapsed := time.Since(start).Milliseconds()

	return &driver.QueryResult{
		Columns:       cols,
		Rows:          resultRows,
		Total:         int64(len(resultRows)),
		ExecutionTime: elapsed,
	}, nil
}

// ExecuteStatement executes a single DML/DDL statement.
func (d *MySQLDriver) ExecuteStatement(ctx context.Context, database string, stmt string) (*driver.StatementResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("mysql: not connected")
	}

	start := time.Now()
	sqlResult, err := d.db.ExecContext(ctx, stmt)
	duration := time.Since(start).Milliseconds()

	r := &driver.StatementResult{
		Statement:  stmt,
		DurationMs: duration,
	}

	if err != nil {
		r.Status = "error"
		r.Error = err.Error()
		return r, nil
	}

	r.Status = "success"
	r.RowsAffected, _ = sqlResult.RowsAffected()
	return r, nil
}

// Parse analyzes a SQL string using the unified parser.
func (d *MySQLDriver) Parse(query string) (*driver.ParseResult, error) {
	result, err := sqlparser.ParseSQL(query, "mysql")
	if err != nil {
		return nil, err
	}

	pr := &driver.ParseResult{
		Operation: string(result.Operation),
		Targets:   result.Tables,
		Warnings:  result.Warnings,
	}

	if result.IsBlocked {
		pr.IsBlocked = true
		pr.BlockReason = result.BlockReason
	}

	// Map risk level from parser's result
	pr.RiskLevel = string(result.RiskLevel)

	return pr, nil
}
