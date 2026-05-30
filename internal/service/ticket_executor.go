package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
)

// statementResult holds the outcome of executing a single SQL statement.
type statementResult struct {
	SQL          string
	Status       string // "success" or "error"
	RowsAffected int64
	Error        string
	DurationMs   int64
}

// executeSQL connects to the target database and executes the ticket's SQL.
// It splits multi-statement SQL and executes each statement individually.
func (s *TicketService) executeSQL(ctx context.Context, ds *model.DataSource, database, dbType, sqlContent string) ([]statementResult, error) {
	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密数据库密码失败: %w", err)
	}

	var targetDB *sql.DB

	switch ds.Type {
	case "mysql":
		targetDB, err = s.connMgr.GetMySQL(ds.ID, ds.Host, ds.Port, ds.Username, password, database, connpool.MySQLPoolConfig{})
	case "postgresql", "postgres":
		targetDB, err = s.connMgr.GetPostgreSQL(ds.ID, ds.Host, ds.Port, ds.Username, password, database, ds.SSLMode, connpool.PGPoolConfig{})
	default:
		return nil, fmt.Errorf("不支持的数据源类型: %s", ds.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("连接数据源失败: %w", err)
	}

	// Split multi-statement SQL by semicolons (simple split, handles most DDL/DML cases)
	statements := splitStatements(sqlContent)
	results := make([]statementResult, 0, len(statements))
	var firstErr error

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		start := time.Now()
		sqlResult, execErr := targetDB.ExecContext(ctx, stmt)
		duration := time.Since(start).Milliseconds()

		r := statementResult{
			SQL:        stmt,
			DurationMs: duration,
		}

		if execErr != nil {
			r.Status = "error"
			r.Error = sanitizeErrMsg(execErr.Error())
			if firstErr == nil {
				firstErr = fmt.Errorf("statement failed: %s", r.Error)
			}
		} else {
			r.Status = "success"
			if sqlResult != nil {
				r.RowsAffected, _ = sqlResult.RowsAffected()
			}
		}

		results = append(results, r)
	}

	return results, firstErr
}

// recordExecutionResult writes a single statement execution result to the database.
func (s *TicketService) recordExecutionResult(ctx context.Context, ticketID int64, index int, r statementResult) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO execution_results (ticket_id, statement_index, sql, status, rows_affected, error, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		ticketID, index, r.SQL, r.Status, r.RowsAffected, r.Error, r.DurationMs,
	)
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
func (s *TicketService) GetExecutionResults(ctx context.Context, ticketID int64) ([]model.ExecutionResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ticket_id, statement_index, sql, status, rows_affected, error, duration_ms, created_at
		 FROM execution_results WHERE ticket_id = ? ORDER BY statement_index ASC`,
		ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询执行结果失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := make([]model.ExecutionResult, 0)
	for rows.Next() {
		var r model.ExecutionResult
		if err := rows.Scan(&r.ID, &r.TicketID, &r.StatementIndex, &r.SQL,
			&r.Status, &r.RowsAffected, &r.Error, &r.DurationMs, &r.CreatedAt); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
