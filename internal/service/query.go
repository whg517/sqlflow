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
}

// QueryService handles SQL query execution logic.
type QueryService struct {
	db            *sql.DB
	dsSvc         *DatasourceService
	historySvc    *QueryHistoryService
	permSvc       *PermissionService
	encryptionKey string
}

// NewQueryService creates a new QueryService.
func NewQueryService(db *sql.DB, dsSvc *DatasourceService, historySvc *QueryHistoryService, permSvc *PermissionService, encryptionKey string) *QueryService {
	return &QueryService{
		db:            db,
		dsSvc:         dsSvc,
		historySvc:    historySvc,
		permSvc:       permSvc,
		encryptionKey: encryptionKey,
	}
}

// ExecuteQuery executes a SQL query on the specified datasource.
func (s *QueryService) ExecuteQuery(userID int64, username, role string, datasourceID int64, database, sqlContent, dbType string) (*QueryResult, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrEmptySQL
	}

	// Get datasource
	ds, err := s.dsSvc.GetDataSource(datasourceID)
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

	// Execute based on db type
	var result *QueryResult
	switch dbType {
	case "mysql":
		result, err = s.executeMySQL(datasourceID, database, sqlContent, ds.Host, ds.Port, ds.Username, password, defaultRowLimit)
	case "mongodb":
		result, err = s.executeMongoDB(datasourceID, database, sqlContent, ds.Host, ds.Port, ds.Username, password, defaultRowLimit)
	default:
		return nil, ErrDatasourceType
	}

	if err != nil {
		// Write audit log for failed query
		s.writeAuditLog(AuditRecord{
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
	desensitized, maskedFields := s.applyDesensitization(result, role, datasourceID, database, parseResult.Tables)
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
	_ = s.historySvc.CreateHistory(history)

	// Write audit log for successful query
	s.writeAuditLog(AuditRecord{
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

func (s *QueryService) executeMySQL(datasourceID int64, database, sqlContent, host string, port int, user, password string, rowLimit int) (*QueryResult, error) {
	dbName := database
	if dbName == "" {
		dbName = "information_schema"
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=30s&parseTime=true", user, password, host, port, dbName)

	start := time.Now()

	targetDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	defer targetDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := targetDB.QueryContext(ctx, sqlContent)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrSQLTimeout
		}
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer rows.Close()

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

func (s *QueryService) executeMongoDB(datasourceID int64, database, body, host string, port int, user, password string, rowLimit int) (*QueryResult, error) {
	uri := buildMongoURI(host, port, user, password)

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	start := time.Now()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("连接MongoDB失败: %w", err)
	}
	defer client.Disconnect(ctx)

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("连接MongoDB失败: %w", err)
	}

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
				bson.Unmarshal(bsonBytes, &filter)
			}
		}
		cursor, err = collection.Find(ctx, filter, findOpts)

	case sqlparser.MongoOpAggregate:
		pipeline := bson.A{}
		bodyMap := parseMongoBody(body)
		if p, ok := bodyMap["pipeline"]; ok {
			bsonBytes, _ := bson.MarshalExtJSON(p, false, false)
			bson.Unmarshal(bsonBytes, &pipeline)
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
	defer cursor.Close(ctx)

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
func (s *QueryService) applyDesensitization(result *QueryResult, role string, datasourceID int64, database string, tables []string) (bool, []string) {
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
	rules := s.loadMaskRules(datasourceID, database, tables)
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
func (s *QueryService) loadMaskRules(datasourceID int64, database string, tables []string) []mask.Rule {
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

	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("load mask rules: %v", err)
		return nil
	}
	defer rows.Close()

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
	return rules
}

// writeAuditLog writes an audit log entry (best-effort, non-blocking).
func (s *QueryService) writeAuditLog(rec AuditRecord) {
	go func() {
		_, err := s.db.Exec(
			`INSERT INTO audit_logs (user_id, action, datasource_id, database, sql_content, sql_summary, result_rows, affected_rows, execution_time_ms, error_message, desensitized_fields, ip_address)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rec.UserID, rec.Action, rec.DatasourceID, rec.Database,
			rec.SQLContent, rec.SQLSummary, rec.ResultRows, rec.AffectedRows,
			rec.ExecutionTimeMs, rec.ErrorMessage, rec.DesensitizedFields, rec.IPAddress,
		)
		if err != nil {
			log.Printf("write audit log: %v", err)
		}
	}()
}

// parseMongoBody parses the MongoDB command body into a map.
func parseMongoBody(body string) map[string]interface{} {
	var m map[string]interface{}
	if err := bson.UnmarshalExtJSON([]byte(body), false, &m); err != nil {
		return nil
	}
	return m
}

// truncateSQL returns the first 100 characters of a SQL string.
func truncateSQL(sql string) string {
	if len(sql) > 100 {
		return sql[:100]
	}
	return sql
}
