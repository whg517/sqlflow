// Package driver defines the data source driver abstraction layer.
// New data sources only need to implement the Driver interface and call Register().
package driver

import (
	"context"
	"encoding/json"
	"time"
)

// Capability represents a single data source capability.
type Capability int

const (
	CapQuery                Capability = 1 << iota // Execute read-only queries
	CapTicketExec                                  // Execute DML/DDL via ticket workflow
	CapMetadata                                    // List databases/tables/columns
	CapTableLevelPermission                        // Table-level permission control (Casbin)
	CapFieldMasking                                // Field-level data masking
	CapSQLParse                                    // SQL/query syntax parsing
	CapExport                                      // Data export
)

// QueryForm describes how a read query is composed for a data source.
//
// It is a separate axis from Capabilities: Elasticsearch and MySQL both declare
// CapQuery, but a user composes those queries in fundamentally different ways,
// so the UI must pick a different editor and request payload for each.
type QueryForm string

const (
	// QueryFormSQL is free-form SQL text.
	QueryFormSQL QueryForm = "sql"
	// QueryFormDocument is a MongoDB-style collection + operation + filter document.
	QueryFormDocument QueryForm = "document"
	// QueryFormDSL is an Elasticsearch index pattern + JSON query DSL body.
	QueryFormDSL QueryForm = "dsl"
)

// CapabilitySet is a set of capabilities.
type CapabilitySet Capability

// Has checks whether a capability is present.
func (c CapabilitySet) Has(cap Capability) bool {
	return Capability(c)&cap != 0
}

// String returns a human-readable representation.
func (c CapabilitySet) String() string {
	var names []string
	if c.Has(CapQuery) {
		names = append(names, "query")
	}
	if c.Has(CapTicketExec) {
		names = append(names, "ticket_exec")
	}
	if c.Has(CapMetadata) {
		names = append(names, "metadata")
	}
	if c.Has(CapTableLevelPermission) {
		names = append(names, "permission")
	}
	if c.Has(CapFieldMasking) {
		names = append(names, "masking")
	}
	if c.Has(CapSQLParse) {
		names = append(names, "parse")
	}
	if c.Has(CapExport) {
		names = append(names, "export")
	}
	if len(names) == 0 {
		return "none"
	}
	return joinStrings(names, ",")
}

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

	// Capabilities declares which capabilities this driver supports.
	Capabilities() CapabilitySet

	// QueryForm declares how read queries are composed for this data source.
	QueryForm() QueryForm

	// Connect establishes a connection using the provided config.
	Connect(ctx context.Context, cfg *Config) error

	// Close releases all resources held by this driver.
	Close() error

	// Ping verifies the connection is alive.
	Ping(ctx context.Context) error

	// ListDatabases returns available databases (CapMetadata).
	ListDatabases(ctx context.Context) ([]string, error)

	// ListTables returns tables for the given database (CapMetadata).
	// If the driver cannot provide column info, Columns will be empty.
	ListTables(ctx context.Context, database string) ([]TableInfo, error)

	// GetColumns returns column metadata for a specific table (CapMetadata).
	GetColumns(ctx context.Context, database, table string) ([]ColumnInfo, error)

	// ExecuteQuery executes a read-only query and returns results (CapQuery).
	ExecuteQuery(ctx context.Context, database string, query string, limit int) (*QueryResult, error)

	// ExecuteStatement executes a single DML/DDL statement (CapTicketExec).
	ExecuteStatement(ctx context.Context, database string, stmt string) (*StatementResult, error)

	// ExecuteStatements executes multiple DML/DDL statements in a batch (CapTicketExec).
	//
	// 事务语义因驱动而异：
	//   - PostgreSQL: 所有语句包在单个事务中，任一语句失败立即停止并回滚已执行的语句
	//     （成功执行的语句在结果中标记为 "rolled_back"）
	//   - MySQL: 逐条 auto-commit 执行（DDL 无法回滚），任一语句失败后继续执行剩余语句
	//     （收集所有语句的结果，首错通过 error 返回但不中断）
	//   - MongoDB/Elasticsearch: 不支持批量（仅单条），实现可降级为循环调用 ExecuteStatement
	//
	// 工单执行路径完全依赖此方法：事务语义归驱动所有，Service 不再按类型区分。
	ExecuteStatements(ctx context.Context, database string, statements []string) ([]StatementResult, error)

	// Parse analyzes a query string and returns operation metadata (CapSQLParse).
	Parse(query string) (*ParseResult, error)
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
	ExplainQuery(ctx context.Context, database string, query string, args []interface{}) (*QueryResult, error)
}

// ParameterizedQueryExecutor is implemented by SQL drivers that can bind
// template values without interpolating them into the query string.
type ParameterizedQueryExecutor interface {
	ExecuteQueryWithArgs(
		ctx context.Context,
		database string,
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

// helper
func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}
