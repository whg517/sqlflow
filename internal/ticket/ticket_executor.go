package ticket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/db/ent"
	executionresult "github.com/whg517/sqlflow/internal/db/ent/executionresult"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
)

// statementResult holds the outcome of executing a single SQL statement.
type statementResult struct {
	SQL          string
	Status       string // "success" or "error"
	RowsAffected int64
	Error        string
	DurationMs   int64
}

// convertDriverResults 将 driver 层的 StatementResult 转换为 service 层的 statementResult。
// 两者的字段结构基本一致（SQL/Status/Error/RowsAffected/DurationMs），仅类型不同。
func convertDriverResults(drvResults []driver.StatementResult) []statementResult {
	results := make([]statementResult, 0, len(drvResults))
	for _, dr := range drvResults {
		results = append(results, statementResult{
			SQL:          dr.Statement,
			Status:       dr.Status,
			RowsAffected: dr.RowsAffected,
			Error:        dr.Error,
			DurationMs:   dr.DurationMs,
		})
	}
	return results
}

// executeSQL connects to the target database and executes the ticket's SQL.
// For MySQL/PostgreSQL: splits multi-statement SQL and executes each statement individually.
// For MongoDB: parses JSON command body and executes via mongo driver.
func (s *Service) executeSQL(ctx context.Context, ds *model.DataSource, database, dbType, sqlContent string) ([]statementResult, error) {
	secrets, err := datasource.DecryptSecrets(ds, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密数据源凭据失败: %w", err)
	}

	if s.poolMgr == nil {
		return nil, ErrTicketExecUnavailable
	}

	adapter := datasource.NewAdapter(ds)
	cfg, err := driver.BuildConfigFromDataSource(adapter, secrets)
	if err != nil {
		return nil, fmt.Errorf("构建连接配置失败: %w", err)
	}
	d, err := s.poolMgr.Get(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("连接数据源失败: %w", err)
	}
	if !d.Capabilities().Has(driver.CapTicketExec) {
		return nil, ErrTicketExecNotSupported
	}

	// Transaction semantics differ per engine and are the driver's business:
	// PostgreSQL rolls the batch back, MySQL auto-commits each DDL statement,
	// MongoDB applies commands one by one. ExecuteStatements documents that
	// contract; this function must not re-implement it per type.
	statements := splitStatements(sqlContent)
	drvResults, err := d.ExecuteStatements(ctx, database, statements)
	// A failure still carries the results of whatever already ran, including
	// any rolled_back markers, so they are returned either way.
	return convertDriverResults(drvResults), err
}

// recordExecutionResult writes a single statement execution result to the database.
func (s *Service) recordExecutionResult(ctx context.Context, ticketID int64, index int, r statementResult) {
	_, err := s.client.ExecutionResult.Create().
		SetTicketID(ticketID).
		SetStatementIndex(index).
		SetSQL(r.SQL).
		SetStatus(r.Status).
		SetRowsAffected(r.RowsAffected).
		SetError(r.Error).
		SetDurationMs(r.DurationMs).
		Save(ctx)
	if err != nil {
		log.Printf("ticket: record execution result failed: %v", err)
	}
}

// splitStatements splits SQL content into individual statements.
// Handles simple semicolon-separated statements.
func splitStatements(sqlContent string) []string {
	// Simple split by semicolons - trim whitespace and skip empty
	parts := strings.Split(sqlContent, ";")
	statements := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			statements = append(statements, p)
		}
	}
	return statements
}

// sha256Hash computes the SHA-256 hash of a string.
func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// totalRowsAffected sums rows affected across all statement results.
func totalRowsAffected(results []statementResult) int64 {
	var total int64
	for _, r := range results {
		total += r.RowsAffected
	}
	return total
}

// totalDurationMs sums duration across all statement results.
func totalDurationMs(results []statementResult) int64 {
	var total int64
	for _, r := range results {
		total += r.DurationMs
	}
	return total
}

// sanitizeErrMsg removes sensitive information (hostnames, IPs, ports, paths) from error messages.
var sensitivePattern = regexp.MustCompile(`(?i)(host|addr|address|server|ip|port|path|file|socket)\s*[=:]\s*\S+`)

func sanitizeErrMsg(msg string) string {
	// Remove common sensitive patterns
	cleaned := sensitivePattern.ReplaceAllString(msg, "$1=***")
	// Remove anything that looks like an IP:port
	ipPortPattern := regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?`)
	cleaned = ipPortPattern.ReplaceAllString(cleaned, "***")
	// Remove file paths
	pathPattern := regexp.MustCompile(`/[\w./-]+`)
	cleaned = pathPattern.ReplaceAllString(cleaned, "/***")
	return cleaned
}

// GetExecutionResults returns the execution results for a ticket.
func (s *Service) GetExecutionResults(ctx context.Context, ticketID int64) ([]model.ExecutionResult, error) {
	results, err := s.client.ExecutionResult.Query().
		Where(executionresult.TicketIDEQ(ticketID)).
		Order(ent.Asc(executionresult.FieldStatementIndex)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询执行结果失败: %w", err)
	}

	out := make([]model.ExecutionResult, 0, len(results))
	for _, r := range results {
		out = append(out, model.ExecutionResult{
			ID:             int64(r.ID),
			TicketID:       r.TicketID,
			StatementIndex: r.StatementIndex,
			SQL:            r.SQL,
			Status:         r.Status,
			RowsAffected:   r.RowsAffected,
			Error:          r.Error,
			DurationMs:     r.DurationMs,
			CreatedAt:      r.CreatedAt,
		})
	}
	return out, nil
}
