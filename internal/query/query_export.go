package query

import (
	"context"
	"errors"
	"strings"

	"github.com/whg517/sqlflow/internal/platform/auditlog"
)

// ErrExportRowLimit indicates the export result exceeds the maximum allowed rows.
var ErrExportRowLimit = errors.New("导出数据超过10000行上限，请添加 LIMIT 条件缩小范围")

// ExportQuery executes a query for data export with a higher row limit (10000).
func (s *Service) ExportQuery(ctx context.Context, userID int64, username, role string, datasourceID int64, database, sqlContent string, queryParams ...interface{}) (*QueryResult, error) {
	req := readRequest{
		UserID:       userID,
		Role:         role,
		DatasourceID: datasourceID,
		Database:     database,
		SQL:          sqlContent,
		Purpose:      purposeExport,
	}
	grant, err := s.authorizeRead(ctx, req)
	if err != nil {
		return nil, err
	}
	scope := grant.Scope

	// One row beyond the limit, so the caller can tell a full page from an
	// overflow.
	var result *QueryResult
	drvResult, err := s.runRead(ctx, grant, queryParams, s.limits.ExportMaxRows+1)
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

	if err != nil {
		s.auditReadFailure(ctx, "export_failed", req, scope, err)
		// The interactive path maps a deadline to ErrSQLTimeout and this one did
		// not, so the same slow statement answered 400 from /execute and 500
		// from /export. The copy's own comment claimed the two were bounded for
		// the same reason.
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrSQLTimeout
		}
		return nil, err
	}

	// Check row limit
	if result.Total > int64(s.limits.ExportMaxRows) {
		s.auditSvc.Write(ctx, auditlog.Record{
			UserID:       userID,
			Action:       "export_failed",
			DatasourceID: datasourceID,
			Database:     scope,
			SQLContent:   sqlContent,
			SQLSummary:   auditlog.Summarize(sqlContent),
			ResultRows:   result.Total,
		})
		return nil, ErrExportRowLimit
	}

	// Export is a second entrance to the same data, so it crosses the same
	// release seam. Without it an aggregation exported over a protected target
	// would ship unmasked — the condition this path was once missing.
	decision, relErr := releaseRows(ctx, s.client, s.permSvc,
		releaseActor{UserID: userID, Role: role},
		datasourceID, scope, grant.Targets, result.Shape, result.Rows)
	if relErr == nil && decision.Verdict == releaseRefuse {
		relErr = decision.Reason
	}
	if relErr != nil {
		s.auditReadFailure(ctx, "export_failed", req, scope, relErr)
		return nil, relErr
	}
	maskedFields := decision.MaskedFields
	result.Desensitized = decision.Verdict == releaseMasked
	result.DesensitizedFields = maskedFields

	// Write audit log
	summary := auditlog.Summarize(sqlContent)
	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:             userID,
		Action:             readPolicies[purposeExport].okAction,
		DatasourceID:       datasourceID,
		Database:           scope,
		SQLContent:         sqlContent,
		SQLSummary:         summary,
		ResultRows:         result.Total,
		ExecutionTimeMs:    result.ExecutionTime,
		DesensitizedFields: strings.Join(maskedFields, ","),
	})

	return result, nil
}
