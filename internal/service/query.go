package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

var (
	ErrSQLOperationForbidden = errors.New("该操作需要提交工单，仅允许 SELECT 查询")
	ErrSQLHighRisk           = errors.New("高风险操作被拦截，请提交工单")
	ErrSQLTimeout            = errors.New("查询超时（30秒）")
	ErrEmptySQL              = errors.New("SQL 不能为空")
)

// QueryResult holds the result of a query execution.
type QueryResult struct {
	Columns        []string        `json:"columns"`
	Rows           []map[string]interface{} `json:"rows"`
	Total          int64           `json:"total"`
	ExecutionTime  int64           `json:"execution_time_ms"`
	AffectedRows   int64           `json:"affected_rows"`
}

// QueryService handles SQL query execution logic.
type QueryService struct {
	db            *sql.DB
	dsSvc         *DatasourceService
	historySvc    *QueryHistoryService
	encryptionKey string
}

// NewQueryService creates a new QueryService.
func NewQueryService(db *sql.DB, dsSvc *DatasourceService, historySvc *QueryHistoryService, encryptionKey string) *QueryService {
	return &QueryService{
		db:            db,
		dsSvc:         dsSvc,
		historySvc:    historySvc,
		encryptionKey: encryptionKey,
	}
}

// ExecuteQuery executes a SQL query on the specified datasource.
func (s *QueryService) ExecuteQuery(userID int64, datasourceID int64, database string, sqlContent string, dbType string) (*QueryResult, error) {
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

	// Decrypt password
	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密密码失败: %w", err)
	}

	// Parse SQL
	op := sqlparser.ExtractOperation(sqlContent)
	riskLevel := sqlparser.GetRiskLevel(sqlContent, op)

	// Only allow SELECT for direct execution
	if op != sqlparser.OpSelect {
		return nil, ErrSQLOperationForbidden
	}

	// Check high risk
	if riskLevel == sqlparser.RiskHigh {
		return nil, ErrSQLHighRisk
	}

	// Execute based on db type
	switch dbType {
	case "mysql":
		return s.executeMySQL(userID, datasourceID, database, sqlContent, ds.Host, ds.Port, ds.Username, password)
	default:
		return nil, fmt.Errorf("不支持的数据源类型: %s", dbType)
	}
}

func (s *QueryService) executeMySQL(userID, datasourceID int64, database, sqlContent, host string, port int, user, password string) (*QueryResult, error) {
	dbName := database
	if dbName == "" {
		dbName = "information_schema"
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=30s&parseTime=true", user, password, host, port, dbName)

	start := time.Now()

	// Open a temporary connection for this query (not pooled at platform level)
	targetDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	defer targetDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := targetDB.QueryContext(ctx, sqlContent)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrSQLTimeout
		}
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer rows.Close()

	// Get column names
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("获取列信息失败: %w", err)
	}

	// Read all rows
	var resultRows []map[string]interface{}
	for rows.Next() {
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
			// Convert []byte to string for JSON serialization
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果失败: %w", err)
	}

	elapsed := time.Since(start).Milliseconds()

	result := &QueryResult{
		Columns:       cols,
		Rows:          resultRows,
		Total:         int64(len(resultRows)),
		ExecutionTime: elapsed,
		AffectedRows:  0,
	}

	// Write query history (async, best-effort)
	summary := sqlContent
	if len(summary) > 100 {
		summary = summary[:100]
	}
	history := &model.QueryHistory{
		UserID:        userID,
		DatasourceID:  datasourceID,
		Database:      database,
		SQLContent:    sqlContent,
		SQLSummary:    summary,
		DBType:        "mysql",
		ExecutionTime: elapsed,
		ResultRows:    result.Total,
		AffectedRows:  0,
	}
	_ = s.historySvc.CreateHistory(history)

	return result, nil
}
