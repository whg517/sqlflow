package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

const exportRowLimit = 10000

// ErrExportRowLimit indicates the export result exceeds the maximum allowed rows.
var ErrExportRowLimit = errors.New("导出数据超过10000行上限，请添加 LIMIT 条件缩小范围")

// ExportQuery executes a query for data export with a higher row limit (10000).
func (s *QueryService) ExportQuery(ctx context.Context, userID int64, username, role string, datasourceID int64, database, sqlContent, dbType string) (*QueryResult, error) {
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

	// Only allow SELECT for export
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

	// Build pool config from datasource (legacy fallback)
	poolCfg := connpool.MySQLPoolConfig{
		MaxOpen:     ds.MaxOpen,
		MaxIdle:     ds.MaxIdle,
		MaxLifetime: ds.MaxLifetime,
		MaxIdleTime: ds.MaxIdleTime,
	}

	// Execute with exportRowLimit+1 to detect overflow
	var result *QueryResult

	// Use Driver abstraction for MySQL
	if s.poolMgr != nil && dbType == "mysql" {
		adapter := newDataSourceAdapter(ds)
		cfg, err := driver.BuildConfigFromDataSource(adapter, password, "")
		if err != nil {
			return nil, err
		}
		d, err := s.poolMgr.Get(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("connect %s: %w", dbType, err)
		}
		dbName := database
		if dbName == "" {
			dbName = ds.Database
			if dbName == "" {
				dbName = "information_schema"
			}
		}
		drvResult, err := d.ExecuteQuery(ctx, dbName, sqlContent, exportRowLimit+1)
		if err != nil {
			result = nil
			err = fmt.Errorf("driver execute query: %w", err)
		} else {
			result = &QueryResult{
				Columns:       drvResult.Columns,
				Rows:          drvResult.Rows,
				Total:         drvResult.Total,
				ExecutionTime: drvResult.ExecutionTime,
				AffectedRows:  drvResult.AffectedRows,
			}
		}
	}

	if result == nil && err == nil {
		// Fallback to legacy connpool-based execution
		switch dbType {
		case "mysql":
			result, err = s.executeMySQL(ctx, datasourceID, database, sqlContent, ds.Host, ds.Port, ds.Username, password, poolCfg, exportRowLimit+1)
		case "mongodb":
			result, err = s.executeMongoDB(ctx, datasourceID, database, sqlContent, ds.Host, ds.Port, ds.Username, password, exportRowLimit+1)
		default:
			return nil, ErrDatasourceType
		}
	}

	if err != nil {
		s.auditSvc.Write(ctx, AuditRecord{
			UserID:          userID,
			Action:          "export_failed",
			DatasourceID:    datasourceID,
			Database:        database,
			SQLContent:      sqlContent,
			SQLSummary:      truncateSQL(sqlContent),
			ErrorMessage:    err.Error(),
			ExecutionTimeMs: 0,
		})
		return nil, err
	}

	// Check row limit
	if result.Total > int64(exportRowLimit) {
		s.auditSvc.Write(ctx, AuditRecord{
			UserID:       userID,
			Action:       "export_failed",
			DatasourceID: datasourceID,
			Database:     database,
			SQLContent:   sqlContent,
			SQLSummary:   truncateSQL(sqlContent),
			ResultRows:   result.Total,
		})
		return nil, ErrExportRowLimit
	}

	// Apply desensitization
	desensitized, maskedFields := s.applyDesensitization(ctx, result, role, datasourceID, database, parseResult.Tables)
	result.Desensitized = desensitized
	result.DesensitizedFields = maskedFields

	// Write audit log
	summary := truncateSQL(sqlContent)
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:             userID,
		Action:             "export",
		DatasourceID:       datasourceID,
		Database:           database,
		SQLContent:         sqlContent,
		SQLSummary:         summary,
		ResultRows:         result.Total,
		ExecutionTimeMs:    result.ExecutionTime,
		DesensitizedFields: strings.Join(maskedFields, ","),
	})

	return result, nil
}
