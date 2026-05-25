package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
	"github.com/whg517/sqlflow/internal/pkg/mask"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

const (
	defaultRowLimit = 1000
	queryTimeout    = 30 * time.Second
)

var (
	ErrSQLOperationForbidden = errors.New("该操作需要提交工单，仅允许 SELECT 查询")
	ErrSQLHighRisk           = errors.New("高风险操作被拦截，请提交工单")
	ErrSQLBlocked            = errors.New("SQL操作被拦截")
	ErrSQLTimeout            = errors.New("查询超时（30秒）")
	ErrEmptySQL              = errors.New("SQL 不能为空")
	ErrDatasourceType        = errors.New("不支持的数据源类型")
)

// QueryResult holds the result of a query execution.
type QueryResult struct {
	Columns            []string                 `json:"columns"`
	Rows               []map[string]interface{} `json:"rows"`
	Total              int64                    `json:"total"`
	ExecutionTime      int64                    `json:"execution_time_ms"`
	AffectedRows       int64                    `json:"affected_rows"`
	Desensitized       bool                     `json:"desensitized"`
	DesensitizedFields []string                 `json:"desensitized_fields,omitempty"`
	Warnings           []string                 `json:"warnings,omitempty"`
}

// AuditRecord holds data for writing an audit log entry.
type AuditRecord struct {
	UserID             int64
	Action             string
	DatasourceID       int64
	Database           string
	SQLContent         string
	SQLSummary         string
	ResultRows         int64
	AffectedRows       int64
	ExecutionTimeMs    int64
	ErrorMessage       string
	DesensitizedFields string
	IPAddress          string
	AIReviewResult     string
	TicketID           int64
}

// QueryService handles SQL query execution logic.
type QueryService struct {
	db            *sql.DB
	dsSvc         *DatasourceService
	historySvc    *QueryHistoryService
	permSvc       *PermissionService
	auditSvc      *AuditService
	encryptionKey string
	connMgr       *connpool.Manager
}

// NewQueryService creates a new QueryService.
func NewQueryService(db *sql.DB, dsSvc *DatasourceService, historySvc *QueryHistoryService, permSvc *PermissionService, auditSvc *AuditService, encryptionKey string, connMgr *connpool.Manager) *QueryService {
	return &QueryService{
		db:            db,
		dsSvc:         dsSvc,
		historySvc:    historySvc,
		permSvc:       permSvc,
		auditSvc:      auditSvc,
		encryptionKey: encryptionKey,
		connMgr:       connMgr,
	}
}

// ExecuteQuery executes a SQL query on the specified datasource.
func (s *QueryService) ExecuteQuery(ctx context.Context, userID int64, username, role string, datasourceID int64, database, sqlContent, dbType string) (*QueryResult, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrEmptySQL
	}

	// Get datasource
	ds, err := s.dsSvc.GetDataSource(ctx, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("获取数据源失败: %w", err)
	}
	if ds.Status == "disabled" {
		return nil, ErrDatasourceDisabled
	}

	// Use datasource type if dbType not explicitly provided
	if dbType == "" {
		dbType = ds.Type
	}

	// Decrypt password
	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密密码失败: %w", err)
	}

	// Parse SQL via unified parser
	parseResult, err := sqlparser.ParseSQL(sqlContent, dbType)
	if err != nil {
		return nil, fmt.Errorf("SQL解析失败: %w", err)
	}

	// Check if blocked by static rules
	if parseResult.IsBlocked {
		return nil, fmt.Errorf("%w: %s", ErrSQLBlocked, parseResult.BlockReason)
	}

	// Only allow SELECT for direct execution
	if parseResult.Operation != sqlparser.OpSelect {
		return nil, ErrSQLOperationForbidden
	}

	// Check high risk
	if parseResult.RiskLevel == sqlparser.RiskHigh {
		return nil, ErrSQLHighRisk
	}

	// Check table-level permissions via Casbin
	for _, table := range parseResult.Tables {
		allowed, err := s.permSvc.Enforce(role, fmt.Sprintf("ds_%d", datasourceID), table, "select")
		if err != nil {
			return nil, fmt.Errorf("权限校验失败: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("没有表 %s 的查询权限", table)
		}
	}

	// Build pool config from datasource
	poolCfg := connpool.MySQLPoolConfig{
		MaxOpen:     ds.MaxOpen,
		MaxIdle:     ds.MaxIdle,
		MaxLifetime: ds.MaxLifetime,
		MaxIdleTime: ds.MaxIdleTime,
	}

	// Execute based on db type
	var result *QueryResult
	switch dbType {
	case "mysql":
		result, err = s.executeMySQL(ctx, datasourceID, database, sqlContent, ds.Host, ds.Port, ds.Username, password, poolCfg, defaultRowLimit)
	case "mongodb":
		result, err = s.executeMongoDB(ctx, datasourceID, database, sqlContent, ds.Host, ds.Port, ds.Username, password, defaultRowLimit)
	default:
		return nil, ErrDatasourceType
	}

	if err != nil {
		// Write audit log for failed query
		s.auditSvc.Write(ctx, AuditRecord{
			UserID:          userID,
			Action:          "query_failed",
			DatasourceID:    datasourceID,
			Database:        database,
			SQLContent:      sqlContent,
			SQLSummary:      truncateSQL(sqlContent),
			ErrorMessage:    err.Error(),
			ExecutionTimeMs: 0,
		})
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrSQLTimeout
		}
		return nil, err
	}

	// Apply desensitization
	desensitized, maskedFields := s.applyDesensitization(ctx, result, role, datasourceID, database, parseResult.Tables)
	result.Desensitized = desensitized
	result.DesensitizedFields = maskedFields
	result.Warnings = parseResult.Warnings

	// Write query history (async, best-effort)
	summary := truncateSQL(sqlContent)
	history := &model.QueryHistory{
		UserID:        userID,
		DatasourceID:  datasourceID,
		Database:      database,
		SQLContent:    sqlContent,
		SQLSummary:    summary,
		DBType:        dbType,
		ExecutionTime: result.ExecutionTime,
		ResultRows:    result.Total,
		AffectedRows:  result.AffectedRows,
	}
	_ = s.historySvc.CreateHistory(ctx, history)

	// Write audit log for successful query
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:             userID,
		Action:             "query",
		DatasourceID:       datasourceID,
		Database:           database,
		SQLContent:         sqlContent,
		SQLSummary:         summary,
		ResultRows:         result.Total,
		AffectedRows:       result.AffectedRows,
		ExecutionTimeMs:    result.ExecutionTime,
		DesensitizedFields: strings.Join(maskedFields, ","),
	})

	return result, nil
}

func (s *QueryService) executeMySQL(ctx context.Context, datasourceID int64, database, sqlContent, host string, port int, user, password string, poolCfg connpool.MySQLPoolConfig, rowLimit int) (*QueryResult, error) {
	dbName := database
	if dbName == "" {
		dbName = "information_schema"
	}

	start := time.Now()

	targetDB, err := s.connMgr.GetMySQL(datasourceID, host, port, user, password, dbName, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := targetDB.QueryContext(ctx, sqlContent)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrSQLTimeout
		}
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("获取列信息失败: %w", err)
	}

	resultRows := make([]map[string]interface{}, 0, rowLimit)
	rowCount := 0
	for rows.Next() {
		if rowCount >= rowLimit {
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

	return &QueryResult{
		Columns:       cols,
		Rows:          resultRows,
		Total:         int64(len(resultRows)),
		ExecutionTime: elapsed,
		AffectedRows:  0,
	}, nil
}

func (s *QueryService) executeMongoDB(ctx context.Context, datasourceID int64, database, body, host string, port int, user, password string, rowLimit int) (*QueryResult, error) {
	uri := buildMongoURI(host, port, user, password)

	start := time.Now()

	client, err := s.connMgr.GetMongoDB(ctx, datasourceID, uri)
	if err != nil {
		return nil, fmt.Errorf("连接MongoDB失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	// Parse the MongoDB command
	mongoResult, err := sqlparser.ParseMongo(body)
	if err != nil {
		return nil, fmt.Errorf("解析MongoDB命令失败: %w", err)
	}

	dbName := database
	if dbName == "" {
		return nil, fmt.Errorf("MongoDB查询必须指定数据库")
	}

	coll := mongoResult.Collection
	if coll == "" {
		return nil, fmt.Errorf("MongoDB查询必须指定集合名称")
	}

	collection := client.Database(dbName).Collection(coll)

	var cursor *mongo.Cursor
	switch mongoResult.Operation {
	case sqlparser.MongoOpFind:
		findOpts := options.Find().SetLimit(int64(rowLimit))
		filter := bson.M{}
		if mongoResult.HasFilter && !mongoResult.HasEmptyFilter {
			bodyMap := parseMongoBody(body)
			if f, ok := bodyMap["filter"]; ok {
				bsonBytes, _ := bson.MarshalExtJSON(f, false, false)
				_ = bson.Unmarshal(bsonBytes, &filter)
			}
		}
		cursor, err = collection.Find(ctx, filter, findOpts)

	case sqlparser.MongoOpAggregate:
		pipeline := bson.A{}
		bodyMap := parseMongoBody(body)
		if p, ok := bodyMap["pipeline"]; ok {
			bsonBytes, _ := bson.MarshalExtJSON(p, false, false)
			_ = bson.Unmarshal(bsonBytes, &pipeline)
		}
		cursor, err = collection.Aggregate(ctx, pipeline)

	default:
		return nil, ErrSQLOperationForbidden
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrSQLTimeout
		}
		return nil, fmt.Errorf("执行MongoDB查询失败: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	resultRows := make([]map[string]interface{}, 0, rowLimit)
	rowCount := 0
	for cursor.Next(ctx) {
		if rowCount >= rowLimit {
			break
		}
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("读取MongoDB文档失败: %w", err)
		}
		row := make(map[string]interface{})
		for k, v := range doc {
			switch val := v.(type) {
			case primitive.ObjectID:
				row[k] = val.Hex()
			default:
				row[k] = val
			}
		}
		resultRows = append(resultRows, row)
		rowCount++
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("遍历MongoDB结果失败: %w", err)
	}

	// Extract column names from the first row
	columns := make([]string, 0)
	if len(resultRows) > 0 {
		for k := range resultRows[0] {
			columns = append(columns, k)
		}
	}

	elapsed := time.Since(start).Milliseconds()

	return &QueryResult{
		Columns:       columns,
		Rows:          resultRows,
		Total:         int64(len(resultRows)),
		ExecutionTime: elapsed,
		AffectedRows:  0,
	}, nil
}

// applyDesensitization checks if the user has desensitize:bypass permission.
// If not, applies masking rules to the result set.
func (s *QueryService) applyDesensitization(ctx context.Context, result *QueryResult, role string, datasourceID int64, database string, tables []string) (bool, []string) {
	// Check desensitize:bypass permission
	for _, table := range tables {
		if table == "" {
			continue
		}
		bypass, err := s.permSvc.Enforce(role, fmt.Sprintf("ds_%d", datasourceID), table, "desensitize:bypass")
		if err == nil && bypass {
			return false, nil
		}
	}
	// Also check wildcard
	bypass, _ := s.permSvc.Enforce(role, fmt.Sprintf("ds_%d", datasourceID), "*", "desensitize:bypass")
	if bypass {
		return false, nil
	}

	// Load mask rules for this datasource/database/tables
	rules := s.loadMaskRules(ctx, datasourceID, database, tables)
	if len(rules) == 0 {
		return false, nil
	}

	// Apply masking to all rows for all matching tables
	var allMaskedFields []string
	for _, table := range tables {
		tableRules := mask.MatchRules(rules, table)
		if len(tableRules) == 0 {
			continue
		}
		masked := mask.ApplyToRows(result.Rows, tableRules)
		allMaskedFields = append(allMaskedFields, masked...)
	}

	if len(allMaskedFields) == 0 {
		return false, nil
	}
	return true, allMaskedFields
}

// loadMaskRules loads mask rules from the database for the given context.
func (s *QueryService) loadMaskRules(ctx context.Context, datasourceID int64, database string, tables []string) []mask.Rule {
	query := `SELECT datasource_id, database, table_name, field, mask_type, custom_regex, custom_template
			  FROM mask_rules WHERE datasource_id = ?`
	args := []interface{}{datasourceID}

	if database != "" {
		query += ` AND (database = ? OR database = '')`
		args = append(args, database)
	}

	if len(tables) > 0 {
		placeholders := make([]string, len(tables))
		for i, t := range tables {
			placeholders[i] = "?"
			args = append(args, t)
		}
		query += fmt.Sprintf(` AND (table_name IN (%s) OR table_name = '*')`, strings.Join(placeholders, ","))
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		log.Printf("load mask rules: %v", err)
		return nil
	}
	defer func() { _ = rows.Close() }()

	var rules []mask.Rule
	for rows.Next() {
		var r mask.Rule
		var dbName, tbl, field, maskType, customRegex, customTemplate string
		var dsID int64
		if err := rows.Scan(&dsID, &dbName, &tbl, &field, &maskType, &customRegex, &customTemplate); err != nil {
			continue
		}
		r.DatasourceID = dsID
		r.Database = dbName
		r.TableName = tbl
		r.Field = field
		r.MaskType = mask.MaskType(maskType)
		r.CustomRegex = customRegex
		r.CustomTemplate = customTemplate
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		log.Printf("iterate mask rules: %v", err)
	}
	return rules
}

// parseMongoBody parses the MongoDB command body into a map.
func parseMongoBody(body string) map[string]interface{} {
	var m map[string]interface{}
	if err := bson.UnmarshalExtJSON([]byte(body), false, &m); err != nil {
		return nil
	}
	return m
}

// ExplainRow represents a single row from MySQL EXPLAIN output.
type ExplainRow struct {
	ID           int64   `json:"id"`
	SelectType   string  `json:"select_type"`
	Table        string  `json:"table"`
	Partitions   *string `json:"partitions"`
	Type         string  `json:"type"`
	PossibleKeys *string `json:"possible_keys"`
	Key          *string `json:"key"`
	KeyLen       *string `json:"key_len"`
	Ref          *string `json:"ref"`
	Rows         int64   `json:"rows"`
	Filtered     float64 `json:"filtered"`
	Extra        *string `json:"extra"`
}

// ExplainResult holds the result of an EXPLAIN query.
type ExplainResult struct {
	Query        string       `json:"query"`
	DatasourceID int64        `json:"datasource_id"`
	Plan         []ExplainRow `json:"plan"`
	Formatted    string       `json:"formatted"`
}

var (
	ErrExplainNotSupported = errors.New("EXPLAIN 仅支持 MySQL 数据源")
	ErrExplainNonSelect    = errors.New("EXPLAIN 仅支持 SELECT 语句")
)

// ExplainQuery executes EXPLAIN for a SQL query and returns structured results.
func (s *QueryService) ExplainQuery(ctx context.Context, userID int64, role string, datasourceID int64, database, sqlContent string) (*ExplainResult, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrEmptySQL
	}

	// Only allow SELECT statements
	upper := strings.TrimSpace(strings.ToUpper(sqlContent))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "EXPLAIN") {
		return nil, ErrExplainNonSelect
	}

	// Strip leading EXPLAIN if user already prefixed it
	explainSQL := sqlContent
	if strings.HasPrefix(upper, "EXPLAIN") {
		explainSQL = strings.TrimSpace(sqlContent[len("EXPLAIN"):])
	}

	// Get datasource
	ds, err := s.dsSvc.GetDataSource(ctx, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("获取数据源失败: %w", err)
	}
	if ds.Status == "disabled" {
		return nil, ErrDatasourceDisabled
	}
	if ds.Type != "mysql" {
		return nil, ErrExplainNotSupported
	}

	// Decrypt password
	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密密码失败: %w", err)
	}

	// Parse SQL for table-level permission check
	parseResult, err := sqlparser.ParseSQL(explainSQL, "mysql")
	if err != nil {
		return nil, fmt.Errorf("SQL解析失败: %w", err)
	}

	for _, table := range parseResult.Tables {
		allowed, err := s.permSvc.Enforce(role, fmt.Sprintf("ds_%d", datasourceID), table, "select")
		if err != nil {
			return nil, fmt.Errorf("权限校验失败: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("没有表 %s 的查询权限", table)
		}
	}

	// Build pool config
	poolCfg := connpool.MySQLPoolConfig{
		MaxOpen:     ds.MaxOpen,
		MaxIdle:     ds.MaxIdle,
		MaxLifetime: ds.MaxLifetime,
		MaxIdleTime: ds.MaxIdleTime,
	}

	// Get connection
	dbName := database
	if dbName == "" {
		dbName = "information_schema"
	}

	targetDB, err := s.connMgr.GetMySQL(datasourceID, ds.Host, ds.Port, ds.Username, password, dbName, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	// Execute EXPLAIN with timeout
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := targetDB.QueryContext(ctx, "EXPLAIN "+explainSQL)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrSQLTimeout
		}
		return nil, fmt.Errorf("执行 EXPLAIN 失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	explainColumns := []string{"id", "select_type", "table", "partitions", "type", "possible_keys", "key", "key_len", "ref", "rows", "filtered", "Extra"}
	var plan []ExplainRow

	for rows.Next() {
		var r ExplainRow
		var id, rowsVal sql.NullInt64
		var filtered sql.NullFloat64
		var selectType, table, typeVal sql.NullString
		var partitions, possibleKeys, key, keyLen, ref, extra sql.NullString

		if err := rows.Scan(&id, &selectType, &table, &partitions, &typeVal, &possibleKeys, &key, &keyLen, &ref, &rowsVal, &filtered, &extra); err != nil {
			return nil, fmt.Errorf("读取 EXPLAIN 结果失败: %w", err)
		}

		r.ID = id.Int64
		r.Rows = rowsVal.Int64
		r.Filtered = filtered.Float64
		r.SelectType = selectType.String
		r.Table = table.String
		r.Type = typeVal.String
		r.Partitions = nullableString(partitions)
		r.PossibleKeys = nullableString(possibleKeys)
		r.Key = nullableString(key)
		r.KeyLen = nullableString(keyLen)
		r.Ref = nullableString(ref)
		r.Extra = nullableString(extra)

		plan = append(plan, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 EXPLAIN 结果失败: %w", err)
	}

	// Build formatted text table
	formatted := formatExplainTable(explainColumns, plan)

	// Write audit log (best-effort)
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:       userID,
		Action:       "explain",
		DatasourceID: datasourceID,
		Database:     database,
		SQLContent:   sqlContent,
		SQLSummary:   truncateSQL(sqlContent),
	})

	return &ExplainResult{
		Query:        sqlContent,
		DatasourceID: datasourceID,
		Plan:         plan,
		Formatted:    formatted,
	}, nil
}

func nullableString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

func formatExplainTable(columns []string, rows []ExplainRow) string {
	// Build row data as string slices
	strRows := make([][]string, len(rows))
	for i, r := range rows {
		strRows[i] = []string{
			fmt.Sprintf("%d", r.ID),
			r.SelectType,
			r.Table,
			derefOrNull(r.Partitions),
			r.Type,
			derefOrNull(r.PossibleKeys),
			derefOrNull(r.Key),
			derefOrNull(r.KeyLen),
			derefOrNull(r.Ref),
			fmt.Sprintf("%d", r.Rows),
			fmt.Sprintf("%.2f", r.Filtered),
			derefOrNull(r.Extra),
		}
	}

	// Calculate column widths
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
	}
	for _, row := range strRows {
		for i, val := range row {
			if i < len(widths) && len(val) > widths[i] {
				widths[i] = len(val)
			}
		}
	}

	var b strings.Builder
	b.WriteString("EXPLAIN\n")

	// Build separator line
	sep := func() {
		b.WriteByte('+')
		for _, w := range widths {
			b.WriteString(strings.Repeat("-", w+2))
			b.WriteByte('+')
		}
		b.WriteByte('\n')
	}

	sep()

	// Header row
	b.WriteByte('|')
	for i, col := range columns {
		fmt.Fprintf(&b, " %-*s |", widths[i], col)
	}
	b.WriteByte('\n')

	sep()

	// Data rows
	for _, row := range strRows {
		b.WriteByte('|')
		for i, val := range row {
			if i < len(widths) {
				fmt.Fprintf(&b, " %-*s |", widths[i], val)
			}
		}
		b.WriteByte('\n')
	}

	sep()

	return b.String()
}

func derefOrNull(s *string) string {
	if s == nil {
		return "NULL"
	}
	return *s
}

// truncateSQL returns the first 100 characters of a SQL string.
func truncateSQL(sql string) string {
	if len(sql) > 100 {
		return sql[:100]
	}
	return sql
}
