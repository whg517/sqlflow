package sqlparser

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// OperationType represents the type of SQL operation.
type OperationType string

const (
	OpSelect OperationType = "select"
	OpDDL    OperationType = "ddl"
	OpDML    OperationType = "dml"
	OpUpdate OperationType = "update"
	OpDelete OperationType = "delete"
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
	case "postgresql", "postgres", "pg":
		return parsePostgreSQLUnified(input)
	case "mongodb", "mongo":
		return parseMongoUnified(input)
	case "elasticsearch", "es":
		return parseElasticsearch(input)
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

// parsePostgreSQLUnified parses PostgreSQL SQL and applies security rules.
// PostgreSQL and MySQL share common SQL syntax for basic operation types
// (SELECT/INSERT/UPDATE/DELETE/DDL), so we reuse the MySQL parser for
// operation type detection. PostgreSQL-specific syntax (e.g. RETURNING,
// ON CONFLICT) that the MySQL parser cannot handle will cause a parse error,
// which we treat as potentially dangerous and block.
func parsePostgreSQLUnified(sql string) (*SQLParseResult, error) {
	result := &SQLParseResult{
		DBType: "postgresql",
	}

	// First, try to parse with the MySQL parser for standard SQL operations.
	// This handles SELECT, INSERT, UPDATE, DELETE, and common DDL.
	mysqlResult, err := ParseMySQL(sql)
	if err != nil {
		// MySQL parser failed — could be PostgreSQL-specific syntax.
		// Fall back to keyword-based detection for common operations.
		op := detectSQLOperation(sql)
		result.Operation = op
		if op != OpSelect {
			result.IsBlocked = true
			result.BlockReason = "仅允许 SELECT 查询，其他操作请提交工单"
		}
		result.RiskLevel = classifyRiskLevel(op)
		return result, nil
	}

	result.Operation = mysqlResult.Operation
	result.Tables = mysqlResult.Tables

	// Apply same static rules as MySQL
	result.applyMySQLRules(mysqlResult)

	return result, nil
}

// detectSQLOperation uses keyword-based detection as a fallback when
// the AST parser cannot handle PostgreSQL-specific syntax.
func detectSQLOperation(sql string) OperationType {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	// Remove leading comments
	for strings.HasPrefix(upper, "--") || strings.HasPrefix(upper, "/*") {
		if strings.HasPrefix(upper, "--") {
			if idx := strings.Index(upper, "\n"); idx >= 0 {
				upper = strings.TrimSpace(upper[idx+1:])
			} else {
				break
			}
		} else if strings.HasPrefix(upper, "/*") {
			if idx := strings.Index(upper, "*/"); idx >= 0 {
				upper = strings.TrimSpace(upper[idx+2:])
			} else {
				break
			}
		}
	}

	switch {
	case strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH"):
		return OpSelect
	case strings.HasPrefix(upper, "INSERT"):
		return OpDML
	case strings.HasPrefix(upper, "UPDATE"):
		return OpUpdate
	case strings.HasPrefix(upper, "DELETE"):
		return OpDelete
	case strings.HasPrefix(upper, "DROP"):
		return OpDDL
	case strings.HasPrefix(upper, "ALTER"):
		return OpDDL
	case strings.HasPrefix(upper, "CREATE"):
		return OpDDL
	case strings.HasPrefix(upper, "TRUNCATE"):
		return OpDDL
	case strings.HasPrefix(upper, "GRANT") || strings.HasPrefix(upper, "REVOKE"):
		return OpDDL
	default:
		// Unknown — treat as DDL for safety
		return OpDDL
	}
}

// classifyRiskLevel returns a risk level based on operation type.
func classifyRiskLevel(op OperationType) RiskLevel {
	switch op {
	case OpSelect:
		return RiskLow
	case OpDML, OpUpdate, OpDelete:
		return RiskMedium
	case OpDDL:
		return RiskHigh
	default:
		return RiskHigh
	}
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
	case MongoOpInsert:
		result.Operation = OpDML
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
	if mr.IsTruncate {
		r.IsBlocked = true
		r.BlockReason = "TRUNCATE is not allowed"
		r.RiskLevel = RiskHigh
		return
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
func CheckSensitiveTables(ctx context.Context, db *sql.DB, tables []string, datasourceID int) ([]string, error) {
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

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sensitive tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
	if mysqlResult.IsTruncate {
		return true
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

// ---------------------------------------------------------------------------
// Elasticsearch 解析
// ---------------------------------------------------------------------------

// ESQueryRequest 前端提交的 Elasticsearch 查询请求结构。
type ESQueryRequest struct {
	Index     string          `json:"index"`
	Operation string          `json:"operation"` // search | count (默认 search)
	Body      json.RawMessage `json:"body"`
}

// esDangerousEndpoints ES 禁止访问的危险端点。
var esDangerousEndpoints = map[string]bool{
	"_bulk":            true,
	"_delete_by_query": true,
	"_update_by_query": true,
	"_search/scroll":   true,
	"_reindex":         true,
	"_update":          true,
	"_delete":          true,
	"_mget":            true,
	"_msearch":         true,
	"_msearch/template": true,
}

// esAllowedOperations ES 允许的查询操作白名单。
var esAllowedOperations = map[string]bool{
	"search": true, // _search（默认）
	"count":  true, // _count
}

// esAllowedBodyFields ES body 允许的顶级字段白名单。
// 任何不在此列表中的字段将被拒绝，防止注入危险操作。
var esAllowedBodyFields = map[string]bool{
	// 查询
	"query":    true,
	"bool":     true, // 嵌套在 query 中
	"must":     true,
	"should":   true,
	"filter":   true,
	"must_not": true,
	// 分页
	"from": true,
	"size": true,
	// 排序
	"sort": true,
	"docvalue_fields": true,
	// 字段选择
	"_source":           true,
	"stored_fields":     true,
	// 高亮
	"highlight": true,
	// 聚合
	"aggs":         true,
	"aggregations": true,
	// 统计
	"track_total_hits": true,
	"explain":          true,
	// 超时
	"timeout": true,
	// 搜索后 (profile, min_score)
	"profile":    true,
	"min_score":  true,
	"terminate_after": true,
	// 预过滤
	"pre_filter_shard_size": true,
}

// parseElasticsearch 解析 Elasticsearch 查询请求并执行安全检查。
// 前端提交格式: {"index": "my-index-*", "body": {"query": {...}, "size": 100}}
func parseElasticsearch(input string) (*SQLParseResult, error) {
	result := &SQLParseResult{
		DBType: "elasticsearch",
	}

	var req ESQueryRequest
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return nil, fmt.Errorf("Elasticsearch 查询格式错误，需要 JSON 格式: %w", err)
	}

	if strings.TrimSpace(req.Index) == "" {
		return nil, fmt.Errorf("Elasticsearch 查询必须指定 index")
	}

	// 检查危险端点
	for endpoint := range esDangerousEndpoints {
		if strings.Contains(req.Index, endpoint) {
			result.IsBlocked = true
			result.BlockReason = fmt.Sprintf("禁止操作: %s", endpoint)
			result.RiskLevel = RiskHigh
			return result, nil
		}
	}

	// 验证操作类型白名单
	op := strings.ToLower(strings.TrimSpace(req.Operation))
	if op == "" {
		op = "search" // 默认 _search
	}
	if !esAllowedOperations[op] {
		result.IsBlocked = true
		result.BlockReason = fmt.Sprintf("不允许的操作类型: %s，仅支持: search, count", op)
		result.RiskLevel = RiskHigh
		return result, nil
	}

	// 解析 body，检查安全性
	var bodyMap map[string]interface{}
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &bodyMap); err != nil {
			return nil, fmt.Errorf("Elasticsearch body JSON 解析失败（请检查 JSON 格式）: %w", err)
		}

		// 检查 script 字段
		if _, hasScript := bodyMap["script"]; hasScript {
			result.IsBlocked = true
			result.BlockReason = "禁止使用 script 字段"
			result.RiskLevel = RiskHigh
			return result, nil
		}

		// 检查嵌套 script（如 script_fields、inline）
		if hasNestedScript(bodyMap) {
			result.IsBlocked = true
			result.BlockReason = "禁止使用 script 字段"
			result.RiskLevel = RiskHigh
			return result, nil
		}

		// 白名单校验：body 顶级字段必须在允许列表中
		for field := range bodyMap {
			if !esAllowedBodyFields[field] {
				result.IsBlocked = true
				result.BlockReason = fmt.Sprintf("不允许的查询字段: %s", field)
				result.RiskLevel = RiskHigh
				return result, nil
			}
		}

		// _search/_count 查询 → OpSelect
		result.Operation = OpSelect

		// 包含 aggs → RiskMedium
		if _, hasAggs := bodyMap["aggs"]; hasAggs {
			result.RiskLevel = RiskMedium
			result.Warnings = append(result.Warnings, "查询包含聚合操作，可能影响性能")
		} else if _, hasAggregations := bodyMap["aggregations"]; hasAggregations {
			result.RiskLevel = RiskMedium
			result.Warnings = append(result.Warnings, "查询包含聚合操作，可能影响性能")
		} else {
			result.RiskLevel = RiskLow
		}
	} else {
		// 无 body，视为 match_all
		result.Operation = OpSelect
		result.RiskLevel = RiskLow
	}

	// 提取 index pattern 作为 Tables
	result.Tables = []string{req.Index}

	return result, nil
}

// hasNestedScript 递归检查 body 中是否包含 script 相关字段。
func hasNestedScript(m map[string]interface{}) bool {
	for key, val := range m {
		if key == "script" || key == "script_fields" {
			return true
		}
		switch v := val.(type) {
		case map[string]interface{}:
			if hasNestedScript(v) {
				return true
			}
		case []interface{}:
			for _, item := range v {
				if sub, ok := item.(map[string]interface{}); ok && hasNestedScript(sub) {
					return true
			}
			}
		}
	}
	return false
}
