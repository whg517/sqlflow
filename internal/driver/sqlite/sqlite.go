// Package sqlite implements read-only access to local SQLite data sources.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/driver/sqlrows"
	"github.com/whg517/sqlflow/internal/platform/sqlparser"
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

// Compile-time proof of the contracts this driver claims.
//
// The optional interfaces are satisfied structurally, so a method that is
// renamed or never lands would otherwise only surface as a capability that
// silently reports false. These assertions turn that into a build failure.
var (
	_ driver.StatementSplitter          = (*Driver)(nil)
	_ driver.Driver                     = (*Driver)(nil)
	_ driver.MetadataBrowser            = (*Driver)(nil)
	_ driver.ParameterizedQueryExecutor = (*Driver)(nil)
	_ driver.ParameterBinder            = (*Driver)(nil)
	_ driver.ConfigValidator            = (*Driver)(nil)
)

func (d *Driver) Type() string { return "sqlite" }

// SplitStatements returns the units this driver will execute for a body.
//
// SQLite reuses the MySQL scanner, as its Parse reuses the MySQL analysis:
// the lexical rules this cares about — quotes, comments, bracket identifiers —
// are shared, and the places they diverge are not places a semicolon hides.
func (d *Driver) SplitStatements(body string) ([]string, error) {
	return sqlparser.SplitMySQLDialect(body)
}

// QueryForm declares how read queries are composed for this data source.
func (d *Driver) QueryForm() driver.QueryForm { return driver.QueryFormSQL }

// Placeholder reports SQLite's positional parameter token.
func (d *Driver) Placeholder(int) string { return "?" }

// ValidateConfig checks the parts of a SQLite configuration that can be judged
// without touching the filesystem.
//
// Whether the file exists is deliberately not checked here: ValidateConfig also
// runs before a datasource is saved, and a path that is not populated yet is a
// legitimate configuration. The file is checked when connecting.
func (d *Driver) ValidateConfig(cfg *driver.Config) error {
	if cfg.Database == "" {
		return fmt.Errorf("sqlite: 必须指定数据库文件路径")
	}
	return nil
}

// checkDatabaseFile reports whether the configured path is a usable database
// file. Connect calls it so a missing file produces a message the user can act
// on instead of a driver-level open error.
func checkDatabaseFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("SQLite 文件不存在: %s", path)
		}
		return fmt.Errorf("无法访问 SQLite 文件: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("SQLite 地址必须指向数据库文件")
	}
	return nil
}

func (d *Driver) Connect(ctx context.Context, cfg *driver.Config) error {
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("sqlite database path is required")
	}
	if err := checkDatabaseFile(cfg.Database); err != nil {
		return err
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

func (d *Driver) ExecuteQuery(ctx context.Context, query string, limit int) (*driver.QueryResult, error) {
	return d.executeQuery(ctx, query, nil, limit)
}

func (d *Driver) ExecuteQueryWithArgs(ctx context.Context, query string, args []interface{}, limit int) (*driver.QueryResult, error) {
	return d.executeQuery(ctx, query, args, limit)
}

// executeQuery reads rows through the shared reader.
//
// It used to be fifty-nine lines that reimplemented sqlrows.Query: same nil
// guard, same 1000 default, same 30-second cap, same []byte-to-string coercion,
// same rows.Err() check, same result assembly. The only differences were three
// error strings — which is the definition of a duplicate rather than a
// variation, and it is why the missing timeout had to be fixed here separately
// after the shared reader already had one.
func (d *Driver) executeQuery(ctx context.Context, query string, args []interface{}, limit int) (*driver.QueryResult, error) {
	return sqlrows.Query(ctx, d.db, "sqlite", query, args, limit)
}

// SQLite implements no StatementExecutor: it is read-only here, and the two
// methods that used to sit in this spot existed only to return "仅支持只读查询"
// because Driver demanded them. Callers now find out from the type.

func (d *Driver) Parse(query string) (*driver.ParseResult, error) {
	// SQLite shares the MySQL analysis: the dialects differ in ways the
	// operation, target and risk rules do not look at.
	result, err := sqlparser.ParseMySQLDialect(query)
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

// ConfigSchema declares what a SQLite connection is made of: a file path.
//
// No host, no port, no credentials. The form used to reach that shape by
// excluding SQLite from three other blocks.
func (d *Driver) ConfigSchema() []driver.ConfigField {
	return []driver.ConfigField{{
		Name: "database", Label: "SQLite 文件路径", Kind: driver.FieldText,
		Required: true, Placeholder: "/absolute/path/to/database.db",
		Storage: driver.StorageColumn,
	}}
}
