// Package driver defines the data source driver abstraction layer.
// New data sources only need to implement the Driver interface and call Register().
package driver

import (
	"context"
	"time"
)

// Capability represents a single data source capability.
type Capability int

const (
	CapQuery               Capability = 1 << iota // Execute read-only queries
	CapTicketExec                                  // Execute DML/DDL via ticket workflow
	CapMetadata                                    // List databases/tables/columns
	CapTableLevelPermission                        // Table-level permission control (Casbin)
	CapFieldMasking                                // Field-level data masking
	CapSQLParse                                    // SQL/query syntax parsing
	CapExport                                      // Data export
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

// QueryResult is the unified query result type.
type QueryResult struct {
	Columns       []string                 `json:"columns"`
	Rows          []map[string]interface{} `json:"rows"`
	Total         int64                    `json:"total"`
	ExecutionTime int64                    `json:"execution_time_ms"`
	AffectedRows  int64                    `json:"affected_rows"`
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

// ParseResult is the output of SQL/query syntax parsing.
type ParseResult struct {
	Operation   string   // select, insert, update, delete, ddl, aggregate, unknown
	Targets     []string // involved tables/collections/indices
	RiskLevel   string   // low, medium, high
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
	// 此方法严格对齐 service.TicketService.executeSQLTransactional 的事务语义，
	// 用于工单 DDL/DML 执行路径的 driver 层迁移。
	ExecuteStatements(ctx context.Context, database string, statements []string) ([]StatementResult, error)

	// Parse analyzes a query string and returns operation metadata (CapSQLParse).
	Parse(query string) (*ParseResult, error)
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
