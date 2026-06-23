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

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/driver"
	executionresult "github.com/whg517/sqlflow/internal/db/ent/executionresult"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
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
func (s *TicketService) executeSQL(ctx context.Context, ds *model.DataSource, database, dbType, sqlContent string) ([]statementResult, error) {
	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密数据库密码失败: %w", err)
	}

	// MongoDB has a separate execution path
	if ds.Type == "mongodb" {
		return s.executeMongoStatements(ctx, ds, database, password, sqlContent)
	}

	// 优先走 driver 抽象层（poolMgr），支持 MySQL/PostgreSQL 的批量+事务执行。
	// 迁移期间回退到旧 connMgr 路径（见下方 fallback）。
	if s.poolMgr != nil && (ds.Type == "mysql" || ds.Type == "postgresql" || ds.Type == "postgres") {
		adapter := newDataSourceAdapter(ds)
		cfg, err := driver.BuildConfigFromDataSource(adapter, password, "")
		if err != nil {
			return nil, fmt.Errorf("构建连接配置失败: %w", err)
		}
		d, err := s.poolMgr.Get(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("连接数据源失败: %w", err)
		}

		// Split multi-statement SQL by semicolons
		statements := splitStatements(sqlContent)
		drvResults, err := d.ExecuteStatements(ctx, database, statements)
		if err != nil {
			// driver 层返回 error 时，drvResults 仍包含已执行语句的结果（含 rolled_back 标记）
			return convertDriverResults(drvResults), err
		}
		return convertDriverResults(drvResults), nil
	}

	// --- fallback: 旧 connMgr 路径（迁移完成后删除）---
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

	// PostgreSQL: use transactional execution (DDL is rollback-capable in PG)
	// MySQL: execute statements individually (DDL auto-commits, cannot be rolled back)
	if ds.Type == "postgresql" || ds.Type == "postgres" {
		return s.executeSQLTransactional(ctx, targetDB, statements)
	}

	// MySQL: non-transactional execution (each statement auto-commits)
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

// executeSQLTransactional wraps all statements in a single database transaction.
// Used for PostgreSQL where DDL is transactional (can be rolled back).
// On any statement failure, the entire transaction is rolled back.
func (s *TicketService) executeSQLTransactional(ctx context.Context, targetDB *sql.DB, statements []string) ([]statementResult, error) {
	tx, err := targetDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开启事务失败: %w", err)
	}

	results := make([]statementResult, 0, len(statements))
	var firstErr error

	defer func() {
		if firstErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("pg tx rollback failed: %v", rbErr)
			}
		}
	}()

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		start := time.Now()
		sqlResult, execErr := tx.ExecContext(ctx, stmt)
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
			results = append(results, r)
			// Stop executing further statements on first error
			break
		}

		r.Status = "success"
		if sqlResult != nil {
			r.RowsAffected, _ = sqlResult.RowsAffected()
		}
		results = append(results, r)
	}

	if firstErr != nil {
		// Rollback will happen via defer
		// Mark rolled-back statements in results
		for i := range results {
			if results[i].Status == "success" {
				results[i].Status = "rolled_back"
			}
		}
		return results, firstErr
	}

	if err := tx.Commit(); err != nil {
		return results, fmt.Errorf("提交事务失败: %w", err)
	}

	return results, nil
}

// executeMongoStatements executes MongoDB operations from a JSON command body.
// Supports find, aggregate, insert, update, delete operations.
// Multiple operations are delimited by a special separator or sent as separate tickets.
func (s *TicketService) executeMongoStatements(ctx context.Context, ds *model.DataSource, database, password, sqlContent string) ([]statementResult, error) {
	uri := buildMongoURI(ds.Host, ds.Port, ds.Username, password)
	client, err := s.connMgr.GetMongoDB(ctx, ds.ID, uri)
	if err != nil {
		return nil, fmt.Errorf("连接MongoDB失败: %w", err)
	}

	// Parse the MongoDB command
	mongoResult, err := sqlparser.ParseMongo(sqlContent)
	if err != nil {
		return nil, fmt.Errorf("解析MongoDB命令失败: %w", err)
	}

	if database == "" {
		return nil, fmt.Errorf("MongoDB操作必须指定数据库")
	}
	if mongoResult.Collection == "" {
		return nil, fmt.Errorf("MongoDB操作必须指定集合名称 (collection)")
	}

	collection := client.Database(database).Collection(mongoResult.Collection)

	start := time.Now()
	var rowsAffected int64
	var execErr error

	switch mongoResult.Operation {
	case sqlparser.MongoOpFind:
		// find is read-only, but for ticket execution we allow it (e.g. data verification)
		filter := bson.M{}
		if mongoResult.HasFilter && !mongoResult.HasEmptyFilter {
			bodyMap := parseMongoBody(sqlContent)
			if f, ok := bodyMap["filter"]; ok {
				bsonBytes, _ := bson.MarshalExtJSON(f, false, false)
				_ = bson.Unmarshal(bsonBytes, &filter)
			}
		}
		cursor, findErr := collection.Find(ctx, filter)
		if findErr != nil {
			execErr = findErr
		} else {
			defer func() { _ = cursor.Close(ctx) }()
				var results []bson.M
				if err := cursor.All(ctx, &results); err != nil {
					execErr = err
				} else {
					rowsAffected = int64(len(results))
				}
			}

	case sqlparser.MongoOpAggregate:
		pipeline := bson.A{}
		bodyMap := parseMongoBody(sqlContent)
		if p, ok := bodyMap["pipeline"]; ok {
			bsonBytes, _ := bson.MarshalExtJSON(p, false, false)
			_ = bson.Unmarshal(bsonBytes, &pipeline)
		}
		cursor, aggErr := collection.Aggregate(ctx, pipeline)
		if aggErr != nil {
			execErr = aggErr
		} else {
			defer func() { _ = cursor.Close(ctx) }()
				var results []bson.M
			if err := cursor.All(ctx, &results); err != nil {
				execErr = err
			} else {
				rowsAffected = int64(len(results))
			}
		}

	case sqlparser.MongoOpUpdate:
		filter := bson.M{}
		updateDoc := bson.M{}
		bodyMap := parseMongoBody(sqlContent)
		if f, ok := bodyMap["filter"]; ok {
			bsonBytes, _ := bson.MarshalExtJSON(f, false, false)
			_ = bson.Unmarshal(bsonBytes, &filter)
		}
		if u, ok := bodyMap["update"]; ok {
			bsonBytes, _ := bson.MarshalExtJSON(u, false, false)
			_ = bson.Unmarshal(bsonBytes, &updateDoc)
		}
		var updateResult *mongo.UpdateResult
		if mongoResult.IsMulti {
			updateResult, execErr = collection.UpdateMany(ctx, filter, updateDoc)
		} else {
			updateResult, execErr = collection.UpdateOne(ctx, filter, updateDoc)
		}
		if execErr == nil && updateResult != nil {
			rowsAffected = updateResult.ModifiedCount
		}

	case sqlparser.MongoOpDelete:
		filter := bson.M{}
		bodyMap := parseMongoBody(sqlContent)
		if f, ok := bodyMap["filter"]; ok {
			bsonBytes, _ := bson.MarshalExtJSON(f, false, false)
			_ = bson.Unmarshal(bsonBytes, &filter)
		}
		var deleteResult *mongo.DeleteResult
		if mongoResult.IsMulti {
			deleteResult, execErr = collection.DeleteMany(ctx, filter)
		} else {
			deleteResult, execErr = collection.DeleteOne(ctx, filter)
		}
		if execErr == nil && deleteResult != nil {
			rowsAffected = deleteResult.DeletedCount
		}

	case sqlparser.MongoOpInsert:
		bodyMap := parseMongoBody(sqlContent)
		if doc, ok := bodyMap["document"]; ok {
			// Single document insert
			bsonBytes, _ := bson.MarshalExtJSON(doc, false, false)
			var docBSON bson.M
			_ = bson.Unmarshal(bsonBytes, &docBSON)
			_, execErr = collection.InsertOne(ctx, docBSON)
			if execErr == nil {
				rowsAffected = 1
			}
		} else if docs, ok := bodyMap["documents"]; ok {
			// Multi document insert
			bsonBytes, _ := bson.MarshalExtJSON(docs, false, false)
			var docsBSON []interface{}
			_ = bson.Unmarshal(bsonBytes, &docsBSON)
			insertResult, insertErr := collection.InsertMany(ctx, docsBSON)
			if insertErr != nil {
				execErr = insertErr
			} else {
				rowsAffected = int64(len(insertResult.InsertedIDs))
			}
		} else {
			return nil, fmt.Errorf("MongoDB insert操作必须包含 document 或 documents 字段")
		}

	default:
		return nil, fmt.Errorf("不支持的MongoDB操作类型: %s", mongoResult.Operation)
	}

	duration := time.Since(start).Milliseconds()

	r := statementResult{
		SQL:        sqlContent,
		DurationMs: duration,
	}

	if execErr != nil {
		r.Status = "error"
		r.Error = sanitizeErrMsg(execErr.Error())
		return []statementResult{r}, fmt.Errorf("MongoDB操作失败: %s", r.Error)
	}

	r.Status = "success"
	r.RowsAffected = rowsAffected
	return []statementResult{r}, nil
}

// recordExecutionResult writes a single statement execution result to the database.
func (s *TicketService) recordExecutionResult(ctx context.Context, ticketID int64, index int, r statementResult) {
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
func (s *TicketService) GetExecutionResults(ctx context.Context, ticketID int64) ([]model.ExecutionResult, error) {
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
