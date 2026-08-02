package query

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/platform/crypto"
)

const exportRowLimit = 10000

// ErrExportRowLimit indicates the export result exceeds the maximum allowed rows.
var ErrExportRowLimit = errors.New("导出数据超过10000行上限，请添加 LIMIT 条件缩小范围")

// ExportQuery executes a query for data export with a higher row limit (10000).
func (s *Service) ExportQuery(ctx context.Context, userID int64, username, role string, datasourceID int64, database, sqlContent, dbType string, queryParams ...interface{}) (*QueryResult, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrEmptySQL
	}

	// Get datasource
	ds, err := s.dsSvc.GetDataSource(ctx, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("获取数据源失败: %w", err)
	}
	if ds.Status == "disabled" {
		return nil, datasource.ErrDatasourceDisabled
	}
	if datasource.IsInternal(ds) && role != "admin" {
		return nil, ErrInternalDatasourceOnly
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
	parseResult, err := driver.ParseFor(dbType, sqlContent)
	if err != nil {
		return nil, fmt.Errorf("SQL解析失败: %w", err)
	}

	// Check if blocked by static rules
	if parseResult.IsBlocked {
		return nil, fmt.Errorf("%w: %s", ErrSQLBlocked, parseResult.BlockReason)
	}

	// Only allow SELECT for export
	if parseResult.Operation != driver.OpSelect {
		return nil, ErrSQLOperationForbidden
	}

	// Check high risk
	if parseResult.RiskLevel == driver.RiskHigh {
		return nil, ErrSQLHighRisk
	}

	// Check table-level permissions via Casbin
	for _, table := range parseResult.Targets {
		allowed, err := s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), table, "select")
		if err != nil {
			return nil, fmt.Errorf("权限校验失败: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("没有表 %s 的查询权限", table)
		}
		allowed, err = s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), table, "export")
		if err != nil {
			return nil, fmt.Errorf("导出权限校验失败: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("没有表 %s 的导出权限", table)
		}
	}

	// Build pool config from datasource (legacy fallback)
	if s.poolMgr == nil {
		return nil, ErrQueryUnavailable
	}
	adapter := datasource.NewAdapter(ds)
	cfg, err := driver.BuildConfigFromDataSource(adapter, password, "")
	if err != nil {
		return nil, err
	}
	// Both failure modes below fall through to the shared error handling, so an
	// export that never reached the datasource still leaves an audit record.
	var result *QueryResult

	d, connErr := s.poolMgr.Get(ctx, cfg)
	if connErr != nil {
		// Map to the same retryable error the query path uses, so the handler
		// answers 400 rather than 500 for an unreachable datasource.
		err = ErrSQLTimeout
	} else {
		// The driver decides what an empty scope means; substituting a default
		// here once made MySQL export from the server's own catalog.
		dbName := database
		if dbName == "" {
			dbName = ds.Database
		}

		// Fetch one row beyond the limit so the caller can detect overflow.
		var drvResult *driver.QueryResult
		drvResult, err = executeDriverQuery(ctx, d, dbName, sqlContent, queryParams, exportRowLimit+1)
		if err == nil {
			result = &QueryResult{
				Shape:         drvResult.Shape,
				Columns:       drvResult.Columns,
				Rows:          drvResult.Rows,
				Total:         drvResult.Total,
				ExecutionTime: drvResult.ExecutionTime,
				AffectedRows:  drvResult.AffectedRows,
				Aggregations:  drvResult.Aggregations,
			}
		}
	}

	if err != nil {
		s.auditSvc.Write(ctx, auditlog.Record{
			UserID:          userID,
			Action:          "export_failed",
			DatasourceID:    datasourceID,
			Database:        database,
			SQLContent:      sqlContent,
			SQLSummary:      auditlog.Summarize(sqlContent),
			ErrorMessage:    err.Error(),
			ExecutionTimeMs: 0,
		})
		return nil, err
	}

	// Check row limit
	if result.Total > int64(exportRowLimit) {
		s.auditSvc.Write(ctx, auditlog.Record{
			UserID:       userID,
			Action:       "export_failed",
			DatasourceID: datasourceID,
			Database:     database,
			SQLContent:   sqlContent,
			SQLSummary:   auditlog.Summarize(sqlContent),
			ResultRows:   result.Total,
		})
		return nil, ErrExportRowLimit
	}

	// Export is a second entrance to the same data, so it consults the same
	// decision about shapes the masker cannot process. Without this an
	// aggregation exported over a protected target would ship unmasked.
	if err := s.refuseUnmaskableShape(ctx, result.Shape, userID, role, datasourceID, database, parseResult.Targets); err != nil {
		s.auditSvc.Write(ctx, auditlog.Record{
			UserID:       userID,
			Action:       "export_failed",
			DatasourceID: datasourceID,
			Database:     database,
			SQLContent:   sqlContent,
			SQLSummary:   auditlog.Summarize(sqlContent),
			ErrorMessage: err.Error(),
		})
		return nil, err
	}

	// Apply desensitization
	desensitized, maskedFields := s.applyDesensitizationForActor(ctx, result, userID, role, datasourceID, database, parseResult.Targets)
	result.Desensitized = desensitized
	result.DesensitizedFields = maskedFields

	// Write audit log
	summary := auditlog.Summarize(sqlContent)
	s.auditSvc.Write(ctx, auditlog.Record{
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
