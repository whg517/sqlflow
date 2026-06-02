// Package postgresql implements the Driver interface for PostgreSQL data sources.
package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

func init() {
	driver.Register("postgresql", func() driver.Driver { return &PostgreSQLDriver{} })
}

// PostgreSQLDriver implements driver.Driver for PostgreSQL.
type PostgreSQLDriver struct {
	db *sql.DB
}

// Type returns "postgresql".
func (d *PostgreSQLDriver) Type() string { return "postgresql" }

// Capabilities declares PostgreSQL's full capability set.
func (d *PostgreSQLDriver) Capabilities() driver.CapabilitySet {
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

// Connect establishes a connection pool to the PostgreSQL server.
func (d *PostgreSQLDriver) Connect(ctx context.Context, cfg *driver.Config) error {
	dbName := cfg.Database
	if dbName == "" {
		dbName = "postgres"
	}

	sslmode := cfg.SSLMode
	if sslmode == "" {
		sslmode = "prefer"
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s&connect_timeout=30",
		url.QueryEscape(cfg.Username),
		url.QueryEscape(cfg.Password),
		cfg.Host, cfg.Port, dbName, sslmode)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open postgresql: %w", err)
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
		return fmt.Errorf("ping postgresql: %w", err)
	}

	d.db = db
	return nil
}

// Close releases the connection pool.
func (d *PostgreSQLDriver) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Ping verifies the connection is alive.
func (d *PostgreSQLDriver) Ping(ctx context.Context) error {
	if d.db == nil {
		return fmt.Errorf("postgresql: not connected")
	}
	return d.db.PingContext(ctx)
}

// ListDatabases returns all databases on the server.
func (d *PostgreSQLDriver) ListDatabases(ctx context.Context) ([]string, error) {
	if d.db == nil {
		return nil, fmt.Errorf("postgresql: not connected")
	}

	rows, err := d.db.QueryContext(ctx,
		"SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
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

// ListTables returns all tables in the given database/schema.
func (d *PostgreSQLDriver) ListTables(ctx context.Context, database string) ([]driver.TableInfo, error) {
	if d.db == nil {
		return nil, fmt.Errorf("postgresql: not connected")
	}

	schema := "public"
	if cfgSchema := database; cfgSchema != "" {
		// If a specific schema is provided (via Config.SchemaName or Extra["schema"]), use it.
		// The "database" parameter here is overloaded: when called from service layer
		// with a schema context, it represents the schema name.
		schema = cfgSchema
	}

	query := "SELECT table_name FROM information_schema.tables WHERE table_schema = $1 AND table_type = 'BASE TABLE' ORDER BY table_name"
	rows, err := d.db.QueryContext(ctx, query, schema)
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
func (d *PostgreSQLDriver) GetColumns(ctx context.Context, database, table string) ([]driver.ColumnInfo, error) {
	if d.db == nil {
		return nil, fmt.Errorf("postgresql: not connected")
	}

	schema := "public"
	if database != "" {
		schema = database
	}

	query := `
		SELECT column_name, data_type, 
		       COALESCE(col_description(
		         (quote_ident(table_schema)||'.'||quote_ident(table_name))::regclass,
		         ordinal_position
		       ), '') AS column_comment
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`

	rows, err := d.db.QueryContext(ctx, query, schema, table)
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
func (d *PostgreSQLDriver) ExecuteQuery(ctx context.Context, database string, query string, limit int) (*driver.QueryResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("postgresql: not connected")
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
func (d *PostgreSQLDriver) ExecuteStatement(ctx context.Context, database string, stmt string) (*driver.StatementResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("postgresql: not connected")
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
func (d *PostgreSQLDriver) Parse(query string) (*driver.ParseResult, error) {
	result, err := sqlparser.ParseSQL(query, "postgresql")
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

	pr.RiskLevel = string(result.RiskLevel)

	return pr, nil
}
