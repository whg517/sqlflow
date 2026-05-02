package sqlparser

import (
	"database/sql"
	"fmt"
	"strings"
)

// OperationType represents the type of SQL operation.
type OperationType string

const (
	OpSelect  OperationType = "select"
	OpDDL     OperationType = "ddl"
	OpDML     OperationType = "dml"
	OpUpdate  OperationType = "update"
	OpDelete  OperationType = "delete"
)

// RiskLevel represents the risk level of a SQL operation.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// SQLParseResult is the unified result for both MySQL and MongoDB parsing.
type SQLParseResult struct {
	DBType      string        // "mysql" or "mongodb"
	Operation   OperationType // For MySQL: select/dml/update/delete/ddl; for Mongo maps to similar concept
	Tables      []string      // MySQL tables or MongoDB collection
	RiskLevel   RiskLevel
	IsBlocked   bool
	BlockReason string
	Warnings    []string
}

// ParseSQL is the unified entry point for SQL parsing.
// dbType should be "mysql" or "mongodb".
func ParseSQL(input string, dbType string) (*SQLParseResult, error) {
	switch strings.ToLower(dbType) {
	case "mysql":
		return parseMySQLUnified(input)
	case "mongodb", "mongo":
		return parseMongoUnified(input)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
}

// parseMySQLUnified parses MySQL SQL and applies static rules.
func parseMySQLUnified(sql string) (*SQLParseResult, error) {
	result := &SQLParseResult{
		DBType: "mysql",
	}

	mysqlResult, err := ParseMySQL(sql)
	if err != nil {
		return nil, err
	}

	result.Operation = mysqlResult.Operation
	result.Tables = mysqlResult.Tables

	// Apply static rules
	result.applyMySQLRules(mysqlResult)

	return result, nil
}

// parseMongoUnified parses MongoDB command and applies static rules.
func parseMongoUnified(body string) (*SQLParseResult, error) {
	result := &SQLParseResult{
		DBType: "mongodb",
	}

	mongoResult, err := ParseMongo(body)
	if err != nil {
		return nil, err
	}

	// Map Mongo operation to unified OperationType
	switch mongoResult.Operation {
	case MongoOpFind:
		result.Operation = OpSelect
	case MongoOpUpdate:
		result.Operation = OpUpdate
	case MongoOpDelete:
		result.Operation = OpDelete
	case MongoOpAggregate:
		result.Operation = OpSelect
	default:
		result.Operation = OpSelect
	}

	if mongoResult.Collection != "" {
		result.Tables = []string{mongoResult.Collection}
	}

	// Apply static rules
	result.applyMongoRules(mongoResult)

	return result, nil
}

// applyMySQLRules applies static risk rules for MySQL.
func (r *SQLParseResult) applyMySQLRules(mr *MySQLParseResult) {
	// Rule: DROP DATABASE / DROP TABLE / TRUNCATE → direct block
	if mr.IsDropDatabase {
		r.IsBlocked = true
		r.BlockReason = "DROP DATABASE is not allowed"
		r.RiskLevel = RiskHigh
		return
	}
	if mr.IsDropTable {
		r.IsBlocked = true
		r.BlockReason = "DROP TABLE is not allowed"
		r.RiskLevel = RiskHigh
		return
	}
	if mr.Operation == OpDDL && !mr.IsDropDatabase && !mr.IsDropTable {
		// Check for TRUNCATE
		if len(mr.Tables) > 0 && mr.Operation == OpDDL {
			// TRUNCATE is already mapped to OpDDL
			r.IsBlocked = true
			r.BlockReason = "TRUNCATE is not allowed"
			r.RiskLevel = RiskHigh
			return
		}
	}

	// Rule: DELETE/UPDATE without WHERE → high risk
	if (mr.Operation == OpUpdate || mr.Operation == OpDelete) && !mr.HasWhere {
		r.RiskLevel = RiskHigh
		r.IsBlocked = true
		r.BlockReason = fmt.Sprintf("%s without WHERE clause is not allowed", strings.ToUpper(string(mr.Operation)))
		return
	}

	// Rule: SELECT without LIMIT → warning (handled by caller if needed)
	if mr.Operation == OpSelect && !mr.HasLimit {
		r.Warnings = append(r.Warnings, "query without LIMIT may return large result set")
	}

	// Default risk levels
	if r.RiskLevel == "" {
		switch mr.Operation {
		case OpSelect:
			r.RiskLevel = RiskLow
		case OpDDL:
			r.RiskLevel = RiskMedium
		case OpDML, OpUpdate, OpDelete:
			r.RiskLevel = RiskMedium
		default:
			r.RiskLevel = RiskMedium
		}
	}
}

// applyMongoRules applies static risk rules for MongoDB.
func (r *SQLParseResult) applyMongoRules(mr *MongoParseResult) {
	switch mr.Operation {
	case MongoOpUpdate:
		// updateMany with empty filter → high risk
		if mr.IsMulti && mr.HasEmptyFilter {
			r.RiskLevel = RiskHigh
			r.IsBlocked = true
			r.BlockReason = "updateMany with empty filter is not allowed"
			return
		}
		r.RiskLevel = RiskMedium
	case MongoOpDelete:
		// deleteMany with empty filter → high risk
		if mr.HasEmptyFilter {
			r.RiskLevel = RiskHigh
			r.IsBlocked = true
			r.BlockReason = "delete with empty filter is not allowed"
			return
		}
		r.RiskLevel = RiskMedium
	case MongoOpAggregate:
		if mr.HasDangerousStage {
			r.RiskLevel = RiskHigh
			r.IsBlocked = true
			r.BlockReason = "aggregation contains dangerous pipeline stage"
			return
		}
		r.RiskLevel = RiskLow
	default:
		r.RiskLevel = RiskLow
	}
}

// CheckSensitiveTables checks which of the given tables are marked as sensitive.
// It queries the mask_rules table via the provided database connection.
func CheckSensitiveTables(db *sql.DB, tables []string, datasourceID int) ([]string, error) {
	if len(tables) == 0 || db == nil {
		return nil, nil
	}

	// Build query with placeholders
	placeholders := make([]string, len(tables))
	args := make([]interface{}, 0, len(tables)+1)
	for i, t := range tables {
		placeholders[i] = "?"
		args = append(args, t)
	}
	args = append(args, datasourceID)

	query := fmt.Sprintf(
		"SELECT DISTINCT table_name FROM mask_rules WHERE table_name IN (%s) AND datasource_id = ?",
		strings.Join(placeholders, ","),
	)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sensitive tables: %w", err)
	}
	defer rows.Close()

	var sensitive []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan sensitive table: %w", err)
		}
		sensitive = append(sensitive, name)
	}
	return sensitive, rows.Err()
}

// ---------------------------------------------------------------------------
// Backward-compatible legacy functions — these are used by service/query.go
// and must remain with the same signatures.
// ---------------------------------------------------------------------------

// ExtractOperation determines the operation type of a SQL statement.
// Uses the AST-based MySQL parser internally.
func ExtractOperation(sqlContent string) OperationType {
	mysqlResult, err := ParseMySQL(sqlContent)
	if err != nil {
		// Fallback to regex for non-parseable SQL (e.g. comments, edge cases)
		return extractOperationRegex(sqlContent)
	}
	return mysqlResult.Operation
}

// ExtractTables returns a list of table names referenced in the SQL.
// Uses the AST-based MySQL parser internally.
func ExtractTables(sqlContent string) []string {
	mysqlResult, err := ParseMySQL(sqlContent)
	if err != nil {
		return nil
	}
	return mysqlResult.Tables
}

// IsHighRisk determines if a SQL statement is high risk based on static rules.
func IsHighRisk(sqlContent string, op OperationType) bool {
	mysqlResult, err := ParseMySQL(sqlContent)
	if err != nil {
		// Fallback to simple heuristic
		s := strings.ToUpper(strings.TrimSpace(sqlContent))
		s = strings.TrimRight(s, ";")
		if strings.HasPrefix(s, "DROP") || strings.HasPrefix(s, "TRUNCATE") {
			return true
		}
		return (op == OpUpdate || op == OpDelete) && !strings.Contains(strings.ToUpper(s), "WHERE")
	}

	// Apply blocking rules
	if mysqlResult.IsDropDatabase || mysqlResult.IsDropTable {
		return true
	}
	// TRUNCATE is always high risk (DDL operation)
	if mysqlResult.Operation == OpDDL {
		// Check if it's a TRUNCATE or other dangerous DDL
		s := strings.ToUpper(strings.TrimSpace(sqlContent))
		if strings.HasPrefix(s, "TRUNCATE") {
			return true
		}
	}
	if (mysqlResult.Operation == OpUpdate || mysqlResult.Operation == OpDelete) && !mysqlResult.HasWhere {
		return true
	}
	return false
}

// GetRiskLevel returns the overall risk level for a SQL statement.
func GetRiskLevel(sqlContent string, op OperationType) RiskLevel {
	if IsHighRisk(sqlContent, op) {
		return RiskHigh
	}
	switch op {
	case OpSelect:
		return RiskLow
	case OpDDL:
		return RiskMedium
	case OpDML, OpUpdate, OpDelete:
		return RiskMedium
	default:
		return RiskMedium
	}
}

// ExtractMongoOperation determines the MongoDB operation from a request body.
func ExtractMongoOperation(body string) MongoOperation {
	result, err := ParseMongo(body)
	if err != nil {
		return MongoOpUnknown
	}
	return result.Operation
}

// IsHighRiskMongo determines if a MongoDB operation is high risk.
func IsHighRiskMongo(body string) bool {
	result, err := ParseMongo(body)
	if err != nil {
		return true // invalid JSON is treated as high risk
	}

	unified := &SQLParseResult{}
	unified.applyMongoRules(result)
	return unified.RiskLevel == RiskHigh
}

// extractOperationRegex is a fallback using regex when AST parsing fails.
func extractOperationRegex(sqlContent string) OperationType {
	s := strings.TrimSpace(sqlContent)
	s = strings.TrimRight(s, ";")
	s = removeComments(s)
	upper := strings.ToUpper(s)

	switch {
	case strings.HasPrefix(upper, "SELECT"), strings.HasPrefix(upper, "WITH"):
		return OpSelect
	case strings.HasPrefix(upper, "INSERT"), strings.HasPrefix(upper, "REPLACE"):
		return OpDML
	case strings.HasPrefix(upper, "UPDATE"):
		return OpUpdate
	case strings.HasPrefix(upper, "DELETE"):
		return OpDelete
	case strings.HasPrefix(upper, "DROP"), strings.HasPrefix(upper, "ALTER"),
		strings.HasPrefix(upper, "CREATE"), strings.HasPrefix(upper, "TRUNCATE"):
		return OpDDL
	default:
		return OpSelect
	}
}

func removeComments(sql string) string {
	// Remove single-line comments
	var result []string
	for _, line := range strings.Split(sql, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		result = append(result, line)
	}
	s := strings.Join(result, "\n")
	// Remove multi-line comments
	for {
		start := strings.Index(s, "/*")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "*/")
		if end < 0 {
			s = s[:start]
			break
		}
		s = s[:start] + s[start+end+2:]
	}
	return s
}

func isSQLKeyword(name string) bool {
	keywords := map[string]bool{
		"select": true, "from": true, "where": true, "and": true, "or": true,
		"not": true, "in": true, "exists": true, "between": true, "like": true,
		"is": true, "null": true, "true": true, "false": true, "as": true,
		"on": true, "join": true, "left": true, "right": true, "inner": true,
		"outer": true, "cross": true, "group": true, "by": true, "having": true,
		"order": true, "limit": true, "offset": true, "union": true, "all": true,
		"distinct": true, "set": true, "values": true, "into": true,
		"case": true, "when": true, "then": true, "else": true, "end": true,
		"dual": true,
	}
	return keywords[name]
}
