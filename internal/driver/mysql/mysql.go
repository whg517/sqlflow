// Package mysql implements the Driver interface for MySQL data sources.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/driver/sqlrows"
	"github.com/whg517/sqlflow/internal/platform/sqlparser"
)

func init() {
	driver.Register("mysql", func() driver.Driver { return &MySQLDriver{} })
}

// MySQLDriver implements driver.Driver for MySQL.
type MySQLDriver struct {
	db *sql.DB
}

// Compile-time proof of the contracts this driver claims.
//
// The optional interfaces are satisfied structurally, so a method that is
// renamed or never lands would otherwise only surface as a capability that
// silently reports false. These assertions turn that into a build failure.
var (
	_ driver.StatementSplitter          = (*MySQLDriver)(nil)
	_ driver.Driver                     = (*MySQLDriver)(nil)
	_ driver.ConfigValidator            = (*MySQLDriver)(nil)
	_ driver.MetadataBrowser            = (*MySQLDriver)(nil)
	_ driver.StatementExecutor          = (*MySQLDriver)(nil)
	_ driver.ParameterizedQueryExecutor = (*MySQLDriver)(nil)
	_ driver.ParameterBinder            = (*MySQLDriver)(nil)
	_ driver.QueryExplainer             = (*MySQLDriver)(nil)
)

// Type returns "mysql".
func (d *MySQLDriver) Type() string { return "mysql" }

// ValidateConfig checks the shape of a saved configuration.
//
// It never connects: a datasource can be saved before the target is reachable,
// so this covers only what is decidable from the form. It deliberately says
// nothing about credentials — a password is either accepted or not, and only
// the target knows which, so demanding one here would reject the setups that
// legitimately have none.
func (d *MySQLDriver) ValidateConfig(cfg *driver.Config) error {
	if cfg.Host == "" {
		return fmt.Errorf("mysql: 主机地址不能为空")
	}
	if cfg.Port == 0 {
		return fmt.Errorf("mysql: 端口不能为空")
	}
	return nil
}

// SplitStatements returns the units this driver will execute for a body.
func (d *MySQLDriver) SplitStatements(body string) ([]string, error) {
	return sqlparser.SplitMySQLDialect(body)
}

// QueryForm declares how read queries are composed for this data source.
func (d *MySQLDriver) QueryForm() driver.QueryForm { return driver.QueryFormSQL }

// Placeholder reports MySQL's positional parameter token.
func (d *MySQLDriver) Placeholder(int) string { return "?" }

// explainRowLimit caps plan rows; a plan is per access step, never large.
const explainRowLimit = 200

// Connect establishes a connection pool to the MySQL server.
func (d *MySQLDriver) Connect(ctx context.Context, cfg *driver.Config) error {
	dsn, err := buildDSN(cfg)
	if err != nil {
		return err
	}

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

	// An empty database means "whatever the connection is bound to", which is
	// what SHOW TABLES reports. Callers must not substitute a default such as
	// information_schema — that would list the server's own catalog instead of
	// the user's tables.
	query := "SHOW TABLES"
	var args []interface{}
	if database != "" {
		query = "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ?"
		args = append(args, database)
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
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
func (d *MySQLDriver) ExecuteQuery(ctx context.Context, query string, limit int) (*driver.QueryResult, error) {
	return d.executeQuery(ctx, query, nil, limit)
}

// ExecuteQueryWithArgs executes a read-only SQL query with bound parameters.
func (d *MySQLDriver) ExecuteQueryWithArgs(ctx context.Context, query string, args []interface{}, limit int) (*driver.QueryResult, error) {
	return d.executeQuery(ctx, query, args, limit)
}

func (d *MySQLDriver) executeQuery(ctx context.Context, query string, args []interface{}, limit int) (*driver.QueryResult, error) {
	return sqlrows.Query(ctx, d.db, "mysql", query, args, limit)
}

// ExecuteStatement executes a single DML/DDL statement.
func (d *MySQLDriver) ExecuteStatement(ctx context.Context, stmt string) (*driver.StatementResult, error) {
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

// ExecuteStatements 逐条 auto-commit 执行多条语句（MySQL DDL 非事务性，无法回滚）。
// 任一语句失败后继续执行剩余语句（与 service.executeSQL 的 MySQL 路径一致），
// 首个错误通过返回值 error 传递，但所有语句的结果都会收集到 results 中。
func (d *MySQLDriver) ExecuteStatements(ctx context.Context, statements []string) ([]driver.StatementResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("mysql: not connected")
	}

	results := make([]driver.StatementResult, 0, len(statements))
	var firstErr error

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		start := time.Now()
		sqlResult, execErr := d.db.ExecContext(ctx, stmt)
		duration := time.Since(start).Milliseconds()

		r := driver.StatementResult{
			Statement:  stmt,
			DurationMs: duration,
		}

		if execErr != nil {
			r.Status = "error"
			r.Error = execErr.Error()
			if firstErr == nil {
				firstErr = fmt.Errorf("statement failed: %s", r.Error)
			}
		} else {
			r.Status = "success"
			if sqlResult != nil {
				r.RowsAffected, _ = sqlResult.RowsAffected()
			}
		}
		results = append(results, r)
	}

	return results, firstErr
}

// ExplainQuery returns MySQL's tabular query plan.
//
// The driver owns the EXPLAIN dialect: MySQL prefixes the statement and returns
// one row per access step, which is not how every SQL engine reports a plan.
func (d *MySQLDriver) ExplainQuery(ctx context.Context, query string, args []interface{}) (*driver.QueryResult, error) {
	return d.executeQuery(ctx, "EXPLAIN "+query, args, explainRowLimit)
}

func (d *MySQLDriver) Parse(query string) (*driver.ParseResult, error) {
	result, err := sqlparser.ParseMySQLDialect(query)
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

// tlsParamForSSLMode maps the platform's sslmode vocabulary onto the MySQL
// driver's tls parameter.
//
// The two use different words for the same three decisions: do not encrypt,
// encrypt without checking who answered, encrypt and verify. An unrecognized
// value is an error rather than a default, because every plausible default is
// wrong — falling back to "off" downgrades a user who asked for TLS, and
// falling back to "on" breaks a datasource that was working.
func tlsParamForSSLMode(sslMode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(sslMode)) {
	case "", "disable":
		return "false", nil
	case "prefer":
		return "preferred", nil
	case "require":
		return "skip-verify", nil
	case "verify-ca", "verify-full":
		return "true", nil
	default:
		return "", fmt.Errorf("mysql: 不支持的 sslmode: %s", sslMode)
	}
}

// buildDSN assembles the connection string.
//
// It goes through the driver's own Config rather than fmt.Sprintf, which both
// applies sslmode — previously accepted by the API, stored, and then ignored
// here — and escapes the credentials. A password is arbitrary bytes, and the
// DSN grammar gives meaning to @ : / and ?, so concatenation made the driver
// read a different user, host or database than the one configured.
func buildDSN(cfg *driver.Config) (string, error) {
	dbName := cfg.Database
	if dbName == "" {
		dbName = "information_schema"
	}

	tlsParam, err := tlsParamForSSLMode(cfg.SSLMode)
	if err != nil {
		return "", err
	}

	c := gomysql.NewConfig()
	c.User = cfg.Username
	c.Passwd = cfg.Password
	c.Net = "tcp"
	c.Addr = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	c.DBName = dbName
	c.Timeout = 30 * time.Second
	c.ParseTime = true
	c.TLSConfig = tlsParam
	return c.FormatDSN(), nil
}
