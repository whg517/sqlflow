package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	es "github.com/elastic/go-elasticsearch/v8"
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
	pgPoolCfg := connpool.PGPoolConfig{
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
	case "postgresql":
		result, err = s.executePostgreSQL(ctx, datasourceID, database, sqlContent, ds, password, pgPoolCfg, defaultRowLimit)
	case "mongodb":
		result, err = s.executeMongoDB(ctx, datasourceID, database, sqlContent, ds.Host, ds.Port, ds.Username, password, defaultRowLimit)
	case "elasticsearch":
		result, err = s.executeElasticsearch(ctx, datasourceID, ds, password, sqlContent, defaultRowLimit)
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

// executePostgreSQL executes a SQL query on a PostgreSQL datasource.
func (s *QueryService) executePostgreSQL(ctx context.Context, datasourceID int64, database, sqlContent string, ds *model.DataSource, password string, poolCfg connpool.PGPoolConfig, rowLimit int) (*QueryResult, error) {
	dbName := database
	if dbName == "" {
		dbName = ds.Database
		if dbName == "" {
			dbName = "postgres"
		}
	}

	start := time.Now()

	targetDB, err := s.connMgr.GetPostgreSQL(datasourceID, ds.Host, ds.Port, ds.Username, password, dbName, ds.SSLMode, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("连接PostgreSQL失败: %w", err)
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
			// PostgreSQL pgx driver returns []byte for some types; convert to string
			switch v := val.(type) {
			case []byte:
				row[col] = string(v)
			case time.Time:
				row[col] = v.Format("2006-01-02T15:04:05Z07:00")
			case nil:
				row[col] = nil
			default:
				row[col] = v
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
		// Use ApplyToMongoRows which supports dot-notation paths for nested documents.
		// For flat SQL results, behaves identically to ApplyToRows.
		masked := mask.ApplyToMongoRows(result.Rows, tableRules)
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

// esMaxSize ES 查询最大返回条数。
const esMaxSize = 1000

// esDefaultSize ES 查询默认返回条数。
const esDefaultSize = 100

// executeElasticsearch 执行 Elasticsearch _search 或 _count 查询。
// sqlContent 为 JSON 格式: {"index": "my-index-*", "operation": "search|count", "body": {"query": {...}}}
// operation 默认为 "search"，仅允许 "search" 和 "count" 两种操作。
func (s *QueryService) executeElasticsearch(ctx context.Context, datasourceID int64, ds *model.DataSource, password, sqlContent string, rowLimit int) (*QueryResult, error) {
	// 解析请求
	var req sqlparser.ESQueryRequest
	if err := json.Unmarshal([]byte(sqlContent), &req); err != nil {
		return nil, fmt.Errorf("解析 Elasticsearch 查询失败: %w", err)
	}

	// 验证操作类型
	op := strings.ToLower(strings.TrimSpace(req.Operation))
	if op == "" {
		op = "search"
	}
	if op != "search" && op != "count" {
		return nil, fmt.Errorf("不允许的 ES 操作类型: %s，仅支持 search 和 count", op)
	}

	index := strings.TrimSpace(req.Index)
	if index == "" {
		// 尝试使用数据源的默认索引模式
		index = ds.ESIndexPattern
	}
	if index == "" {
		return nil, fmt.Errorf("Elasticsearch 查询必须指定 index")
	}

	// 解析 body，强制注入超时和 size 限制
	var bodyMap map[string]interface{}
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &bodyMap); err != nil {
			return nil, fmt.Errorf("解析 Elasticsearch body 失败: %w", err)
		}
	}
	if bodyMap == nil {
		bodyMap = make(map[string]interface{})
	}

	// 强制注入 timeout
	bodyMap["timeout"] = "30s"

	// 限制 size
	if sizeVal, ok := bodyMap["size"]; ok {
		if sizeNum, ok := toFloat64(sizeVal); ok {
			if sizeNum > float64(esMaxSize) {
				bodyMap["size"] = float64(esMaxSize)
			}
		}
	} else {
		bodyMap["size"] = float64(esDefaultSize)
	}

	// 获取 ES 连接
	urls := parseESUrls(ds.ESUrls)
	if len(urls) == 0 {
		return nil, fmt.Errorf("Elasticsearch 数据源未配置连接地址")
	}

	// 解密 ES API Key（如果使用 API Key 认证）
	esApiKey := ""
	if ds.ESApiKey != "" {
		dec, err := crypto.Decrypt(ds.ESApiKey, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("解密 ES API Key 失败: %w", err)
		}
		esApiKey = dec
	}

	client, err := s.connMgr.GetElasticsearch(ctx, datasourceID, urls, ds.ESAuthType, ds.Username, password, esApiKey, ds.ESVerifyCerts)
	if err != nil {
		return nil, fmt.Errorf("连接 Elasticsearch 失败: %w", err)
	}

	// 序列化 body
	bodyJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("序列化 Elasticsearch 请求失败: %w", err)
	}

	start := time.Now()

	// 根据操作类型执行不同的 ES API
	switch op {
	case "count":
		return s.executeESCount(ctx, client, index, bodyJSON, start)
	default: // "search"
		return s.executeESSearch(ctx, client, index, bodyJSON, start)
	}
}

// executeESSearch 执行 ES _search 并转换为标准 QueryResult.
func (s *QueryService) executeESSearch(ctx context.Context, client *es.Client, index string, bodyJSON []byte, start time.Time) (*QueryResult, error) {
	searchResp, err := client.Search(
		client.Search.WithContext(ctx),
		client.Search.WithIndex(index),
		client.Search.WithBody(strings.NewReader(string(bodyJSON))),
	)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrSQLTimeout
		}
		return nil, fmt.Errorf("执行 Elasticsearch _search 失败: %w", err)
	}
	defer searchResp.Body.Close()

	bodyBytes, err := io.ReadAll(searchResp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 Elasticsearch 响应失败: %w", err)
	}

	if searchResp.IsError() {
		return nil, fmt.Errorf("Elasticsearch 返回错误 (%d): %s", searchResp.StatusCode, string(bodyBytes))
	}

	// 解析响应
	var esResp struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Index  string                 `json:"_index"`
				ID     string                 `json:"_id"`
				Score  *float64               `json:"_score"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(bodyBytes, &esResp); err != nil {
		return nil, fmt.Errorf("解析 Elasticsearch 响应失败: %w", err)
	}

	// 转换 hits 为标准 QueryResult
	resultRows := make([]map[string]interface{}, 0, len(esResp.Hits.Hits))
	columnSet := make(map[string]bool)

	for _, hit := range esResp.Hits.Hits {
		row := make(map[string]interface{})
		row["_id"] = hit.ID
		row["_index"] = hit.Index
		if hit.Score != nil {
			row["_score"] = *hit.Score
		}
		for k, v := range hit.Source {
			row[k] = v
		}
		for k := range row {
			columnSet[k] = true
		}
		resultRows = append(resultRows, row)
	}

	// 提取列名（保持顺序: _id, _index, _score, 其余按出现顺序）
	columns := []string{"_id", "_index", "_score"}
	for k := range columnSet {
		if k != "_id" && k != "_index" && k != "_score" {
			columns = append(columns, k)
		}
	}

	elapsed := time.Since(start).Milliseconds()

	return &QueryResult{
		Columns:       columns,
		Rows:          resultRows,
		Total:         esResp.Hits.Total.Value,
		ExecutionTime: elapsed,
		AffectedRows:  0,
	}, nil
}

// executeESCount 执行 ES _count 并转换为标准 QueryResult.
func (s *QueryService) executeESCount(ctx context.Context, client *es.Client, index string, bodyJSON []byte, start time.Time) (*QueryResult, error) {
	countResp, err := client.Count(
		client.Count.WithContext(ctx),
		client.Count.WithIndex(index),
		client.Count.WithBody(strings.NewReader(string(bodyJSON))),
	)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrSQLTimeout
		}
		return nil, fmt.Errorf("执行 Elasticsearch _count 失败: %w", err)
	}
	defer countResp.Body.Close()

	bodyBytes, err := io.ReadAll(countResp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 Elasticsearch 响应失败: %w", err)
	}

	if countResp.IsError() {
		return nil, fmt.Errorf("Elasticsearch 返回错误 (%d): %s", countResp.StatusCode, string(bodyBytes))
	}

	var countResult struct {
		Count int64 `json:"count"`
	}
	if err := json.Unmarshal(bodyBytes, &countResult); err != nil {
		return nil, fmt.Errorf("解析 Elasticsearch _count 响应失败: %w", err)
	}

	elapsed := time.Since(start).Milliseconds()

	return &QueryResult{
		Columns:       []string{"count"},
		Rows:          []map[string]interface{}{{"count": countResult.Count}},
		Total:         1,
		ExecutionTime: elapsed,
		AffectedRows:  0,
	}, nil
}

// toFloat64 将 interface{} 转换为 float64。
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
