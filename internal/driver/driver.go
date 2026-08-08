// Package driver defines the data source driver abstraction layer.
// New data sources only need to implement the Driver interface and call Register().
package driver

import (
	"context"
	"encoding/json"
	"time"
)

// QueryForm describes how a read query is composed for a data source.
//
// It is a separate axis from what a driver can do: Elasticsearch and MySQL are
// both queryable, but a user composes those queries in fundamentally different
// ways, so the UI must pick a different editor and request payload for each.
type QueryForm string

const (
	// QueryFormSQL is free-form SQL text.
	QueryFormSQL QueryForm = "sql"
	// QueryFormDocument is a MongoDB-style collection + operation + filter document.
	QueryFormDocument QueryForm = "document"
	// QueryFormDSL is an Elasticsearch index pattern + JSON query DSL body.
	QueryFormDSL QueryForm = "dsl"
)

// ResultShape tells the caller how to interpret a QueryResult.
//
// Not every data source returns a table. Forcing Elasticsearch aggregations or
// nested documents into Columns/Rows loses information the UI needs to render
// them, so the result declares its own shape instead.
type ResultShape string

const (
	// ShapeTable is a fixed column set with scalar cells — relational results.
	ShapeTable ResultShape = "table"
	// ShapeDocuments is a list of documents whose values may be nested.
	// Columns is still populated as a best-effort flattening for table view.
	ShapeDocuments ResultShape = "documents"
	// ShapeAggregation is a driver-native aggregation payload carried verbatim
	// in Aggregations; Rows is empty.
	ShapeAggregation ResultShape = "aggregation"
)

// QueryResult is the unified query result type.
type QueryResult struct {
	// Shape defaults to ShapeTable when empty, so drivers that only ever return
	// tables need no change.
	Shape         ResultShape              `json:"shape,omitempty"`
	Columns       []string                 `json:"columns"`
	Rows          []map[string]interface{} `json:"rows"`
	Total         int64                    `json:"total"`
	ExecutionTime int64                    `json:"execution_time_ms"`
	AffectedRows  int64                    `json:"affected_rows"`

	// Aggregations carries the driver-native aggregation payload for
	// ShapeAggregation results. It is passed through untouched because its
	// structure is driver-defined and arbitrarily nested.
	Aggregations json.RawMessage `json:"aggregations,omitempty"`
}

// StatementResult is the result of a single statement execution (ticket workflow).
type StatementResult struct {
	Statement    string `json:"statement"`
	Status       string `json:"status"` // "success" or "error"
	RowsAffected int64  `json:"rows_affected"`
	Error        string `json:"error,omitempty"`
	DurationMs   int64  `json:"duration_ms"`
}

// TableInfo represents table metadata.
type TableInfo struct {
	Name    string       `json:"name"`
	Columns []ColumnInfo `json:"columns,omitempty"`
}

// ColumnInfo represents column metadata.
type ColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
}

// Operation values returned by Driver.Parse. Callers compare against these
// constants rather than re-deriving intent from the query text.
const (
	OpSelect    = "select"
	OpInsert    = "insert"
	OpUpdate    = "update"
	OpDelete    = "delete"
	OpDML       = "dml"
	OpDDL       = "ddl"
	OpAggregate = "aggregate"
	OpUnknown   = "unknown"
)

// Risk levels returned by Driver.Parse.
const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

// ParseResult is the output of SQL/query syntax parsing.
type ParseResult struct {
	Operation   string   // one of the Op* constants
	Targets     []string // involved tables/collections/indices
	RiskLevel   string   // one of the Risk* constants
	IsBlocked   bool
	BlockReason string
	Warnings    []string
}

// Driver is the interface that all data source drivers must implement.
// Each data source only needs to implement the methods relevant to its declared Capabilities.
type Driver interface {
	// Type returns the data source type identifier, e.g. "mysql", "postgresql".
	Type() string

	// QueryForm declares how read queries are composed for this data source.
	QueryForm() QueryForm

	// Connect establishes a connection using the provided config.
	Connect(ctx context.Context, cfg *Config) error

	// Close releases all resources held by this driver.
	Close() error

	// Ping verifies the connection is alive.
	Ping(ctx context.Context) error

	// ExecuteQuery executes a read-only query and returns results.
	//
	// There is no database parameter, because there is nothing a driver could
	// do with one. The pool keys on datasource ID alone, the DSN pins the
	// database at Connect, and nothing in the platform issues USE or SET
	// search_path — so a driver's scope is settled before the first query and
	// cannot be moved by a caller. Three of the five drivers took the argument
	// and discarded it; a fourth never read it.
	//
	// It was not merely useless. The same caller-supplied string narrowed the
	// mask-rule lookup in internal/query, so naming a database no rule was
	// scoped to dropped every rule protecting the one the rows actually came
	// from. A parameter drivers cannot honor is worse than no parameter: it
	// invites callers to treat an unvalidated string as the query's scope.
	// Callers that need to know the scope read it off the datasource row.
	ExecuteQuery(ctx context.Context, query string, limit int) (*QueryResult, error)

	// Parse analyzes a query string and returns operation metadata.
	Parse(query string) (*ParseResult, error)
}

// StatementSplitter is implemented by drivers whose bodies may carry more than
// one statement.
//
// A driver that does not implement it carries exactly one statement per body,
// which is the whole answer for a JSON command document — that is why absence
// needs no method rather than a method that returns the body unchanged.
//
// Splitting is grammar knowledge, and grammar knowledge belongs to the driver
// for the same reason Parse does. It also has to be the same knowledge: the
// statements this returns are what the ticket is graded on and what the
// executor runs, so a driver that split differently from how it parses would
// reopen the gap between the analyzed object and the executed one.
type StatementSplitter interface {
	// SplitStatements returns the units this driver will execute for a body.
	// It needs no connection: statement boundaries are a property of the text.
	SplitStatements(body string) ([]string, error)
}

// MetadataBrowser is implemented by drivers that can enumerate their schema.
//
// It is an optional interface for the same reason as QueryExplainer: the
// ability is structural, so the type system can check it. It was a CapMetadata
// bit, which every driver declared — the check that read it could never fire,
// so the guard existed but gated nothing.
type MetadataBrowser interface {
	// ListDatabases returns available databases.
	ListDatabases(ctx context.Context) ([]string, error)

	// ListTables returns tables for the given database. If the driver cannot
	// provide column info, Columns will be empty.
	ListTables(ctx context.Context, database string) ([]TableInfo, error)

	// GetColumns returns column metadata for a specific table.
	GetColumns(ctx context.Context, database, table string) ([]ColumnInfo, error)
}

// StatementExecutor is implemented by drivers that can run DML/DDL through the
// ticket workflow.
//
// These were mandatory methods on Driver, which forced SQLite and Elasticsearch
// to supply bodies that only ever return "not supported" — a value that cannot
// honor part of the contract it satisfies is not a substitute for it, and only
// a hand-written capability check stood between a caller and those stubs. As an
// optional interface the compiler decides, and the stubs are gone.
type StatementExecutor interface {
	// ExecuteStatement executes a single DML/DDL statement.
	//
	// Like ExecuteQuery it takes no database: the scope came from the ticket's
	// free-text field, which the SQL drivers discarded, so an approver's record
	// could state a scope the executor was never able to apply.
	ExecuteStatement(ctx context.Context, stmt string) (*StatementResult, error)

	// ExecuteStatements executes multiple DML/DDL statements in a batch.
	//
	// 事务语义因驱动而异：
	//   - PostgreSQL: 所有语句包在单个事务中，任一语句失败立即停止并回滚已执行的语句
	//     （成功执行的语句在结果中标记为 "rolled_back"）
	//   - MySQL: 逐条 auto-commit 执行（DDL 无法回滚），任一语句失败后继续执行剩余语句
	//     （收集所有语句的结果，首错通过 error 返回但不中断）
	//   - MongoDB: 不支持批量（仅单条），实现降级为循环调用 ExecuteStatement
	//
	// 工单执行路径完全依赖此方法：事务语义归驱动所有，Service 不再按类型区分。
	ExecuteStatements(ctx context.Context, statements []string) ([]StatementResult, error)
}

// ParameterBinder is implemented by drivers that bind parameter values instead
// of interpolating them into the query text.
//
// It is separate from ParameterizedQueryExecutor because the two are needed at
// different moments: rendering a SQL template happens with no connection, so the
// caller must learn the placeholder syntax before it has anywhere to send the
// query. A driver that binds implements both.
type ParameterBinder interface {
	// Placeholder returns the token a query uses for the parameter at the given
	// 1-based position — "?" for MySQL, "$1" for PostgreSQL.
	Placeholder(position int) string
}

// TemplateDialect describes how a data source consumes a parameterised template.
//
// It exists so the template renderer never has to name a data source type: the
// placeholder syntax and whether values bind at all are the driver's to state.
type TemplateDialect struct {
	// Binds reports whether values travel as bound parameters. When false the
	// renderer must inline them, because there is nothing to bind them to.
	Binds bool
	// Placeholder returns the token for a 1-based position. Nil when Binds is
	// false.
	Placeholder func(position int) string
	// QueryForm mirrors Driver.QueryForm, which decides the payload shape a
	// rendered template travels in.
	QueryForm QueryForm
}

// TemplateDialectFor reports the template dialect of a registered data source
// type. It needs no connection.
func TemplateDialectFor(typeName string) (*TemplateDialect, error) {
	d, err := NewDriver(typeName)
	if err != nil {
		return nil, err
	}
	dialect := &TemplateDialect{QueryForm: d.QueryForm()}
	if binder, ok := d.(ParameterBinder); ok {
		dialect.Binds = true
		dialect.Placeholder = binder.Placeholder
	}
	return dialect, nil
}

// ConfigValidator is implemented by drivers whose configuration has rules the
// generic Config shape cannot express — required extras, transport
// restrictions, local file preconditions.
//
// Validation belongs to the driver because only it knows what its fields mean.
// A service that validated on the driver's behalf would need a branch per type
// and would go stale the moment a driver changed its requirements.
//
// ValidateConfig must not connect; it is a check on the configuration alone and
// is called before persisting a datasource as well as before connecting.
type ConfigValidator interface {
	ValidateConfig(cfg *Config) error
}

// ValidateConfigFor checks a configuration against the rules of the driver
// registered for typeName. Drivers with no extra rules pass.
func ValidateConfigFor(typeName string, cfg *Config) error {
	d, err := NewDriver(typeName)
	if err != nil {
		return err
	}
	validator, ok := d.(ConfigValidator)
	if !ok {
		return nil
	}
	return validator.ValidateConfig(cfg)
}

// QueryExplainer is implemented by drivers that can produce a query plan.
//
// It is an optional interface rather than a Capability bit because the ability
// is structural: a driver either has the method or it does not, and the type
// system can check it. Describe() reports it via Descriptor.Explain.
type QueryExplainer interface {
	// ExplainQuery returns the driver-native query plan rows for a read query.
	// The scope is the connection's, as it is for ExecuteQuery.
	ExplainQuery(ctx context.Context, query string, args []interface{}) (*QueryResult, error)
}

// ParameterizedQueryExecutor is implemented by SQL drivers that can bind
// template values without interpolating them into the query string.
type ParameterizedQueryExecutor interface {
	ExecuteQueryWithArgs(
		ctx context.Context,
		query string,
		args []interface{},
		limit int,
	) (*QueryResult, error)
}

// Config holds the connection configuration for a data source.
// It is derived from the DataSource model with decrypted credentials.
type Config struct {
	ID          int64
	Host        string
	Port        int
	Username    string
	Password    string // already decrypted
	Database    string
	SSLMode     string
	SchemaName  string
	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration
	MaxIdleTime time.Duration

	// Extra holds driver-specific parameters (ES urls, auth type, etc.)
	Extra map[string]interface{}
}
