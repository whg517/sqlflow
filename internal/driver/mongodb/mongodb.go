// Package mongodb implements the Driver interface for MongoDB data sources.
package mongodb

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/platform/sqlparser"
)

func init() {
	driver.Register("mongodb", func() driver.Driver { return &MongoDBDriver{} })
}

// Document is a generic document type for MongoDB insert operations.
type Document = map[string]interface{}

// MongoDBDriver implements driver.Driver for MongoDB.
type MongoDBDriver struct {
	client *mongo.Client
}

// Compile-time proof of the contracts this driver claims.
//
// The optional interfaces are satisfied structurally, so a method that is
// renamed or never lands would otherwise only surface as a capability that
// silently reports false. These assertions turn that into a build failure.
var (
	_ driver.Driver            = (*MongoDBDriver)(nil)
	_ driver.MetadataBrowser   = (*MongoDBDriver)(nil)
	_ driver.StatementExecutor = (*MongoDBDriver)(nil)
	_ driver.ConfigDecoder     = (*MongoDBDriver)(nil)
	_ driver.ConfigValidator   = (*MongoDBDriver)(nil)
)

// Type returns "mongodb".
func (d *MongoDBDriver) Type() string { return "mongodb" }

// DecodeConfig reads MongoDB's connection URI out of extra_config.
//
// Unlike Elasticsearch this never got dedicated columns, so the adapter
// answered GetExtra("mongo_uri") with an empty string and a TODO. The URI now
// has one place to live, and it is the same place every other driver's settings
// live.
func (d *MongoDBDriver) DecodeConfig(cfg *driver.Config, extra map[string]interface{}) error {
	if v, ok := extra["uri"].(string); ok && v != "" {
		cfg.Extra["uri"] = v
	}
	return nil
}

// ValidateConfig checks the shape of a saved configuration.
//
// MongoDB accepts either a full connection URI or host/port plus credentials,
// so it cannot reuse the plain host-and-port rule the SQL drivers use — which
// is exactly why this decision belongs to the driver and not to a switch in the
// HTTP handler.
func (d *MongoDBDriver) ValidateConfig(cfg *driver.Config) error {
	if uri, ok := cfg.Extra["uri"].(string); ok && uri != "" {
		return nil
	}
	if cfg.Host == "" {
		return fmt.Errorf("mongodb: 需要连接 URI 或主机地址")
	}
	if cfg.Port == 0 {
		return fmt.Errorf("mongodb: 端口不能为空")
	}
	return nil
}

// QueryForm declares how read queries are composed for this data source.
func (d *MongoDBDriver) QueryForm() driver.QueryForm { return driver.QueryFormDocument }

// Connect establishes a connection to the MongoDB server.
func (d *MongoDBDriver) Connect(ctx context.Context, cfg *driver.Config) error {
	uri := extractURI(cfg)
	if uri == "" {
		return fmt.Errorf("mongodb: connection URI is required")
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("connect mongodb: %w", err)
	}

	if err := client.Ping(connectCtx, nil); err != nil {
		_ = client.Disconnect(connectCtx)
		return fmt.Errorf("ping mongodb: %w", err)
	}

	d.client = client
	return nil
}

// Close releases the connection.
func (d *MongoDBDriver) Close() error {
	if d.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return d.client.Disconnect(ctx)
	}
	return nil
}

// Ping verifies the connection is alive.
func (d *MongoDBDriver) Ping(ctx context.Context) error {
	if d.client == nil {
		return fmt.Errorf("mongodb: not connected")
	}
	return d.client.Ping(ctx, nil)
}

// ListDatabases returns all database names on the server.
func (d *MongoDBDriver) ListDatabases(ctx context.Context) ([]string, error) {
	if d.client == nil {
		return nil, fmt.Errorf("mongodb: not connected")
	}

	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	names, err := d.client.ListDatabaseNames(listCtx, map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	return names, nil
}

// ListTables returns collections for the given database.
func (d *MongoDBDriver) ListTables(ctx context.Context, database string) ([]driver.TableInfo, error) {
	if d.client == nil {
		return nil, fmt.Errorf("mongodb: not connected")
	}
	if database == "" {
		return nil, fmt.Errorf("mongodb: database name is required")
	}

	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cols, err := d.client.Database(database).ListCollectionNames(listCtx, map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}

	tables := make([]driver.TableInfo, len(cols))
	for i, col := range cols {
		tables[i] = driver.TableInfo{Name: col}
	}
	return tables, nil
}

// GetColumns returns field metadata for a MongoDB collection.
// MongoDB is schemaless, so we sample documents to infer field types.
func (d *MongoDBDriver) GetColumns(ctx context.Context, database, table string) ([]driver.ColumnInfo, error) {
	if d.client == nil {
		return nil, fmt.Errorf("mongodb: not connected")
	}
	if database == "" || table == "" {
		return nil, fmt.Errorf("mongodb: database and collection are required")
	}

	sampleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	coll := d.client.Database(database).Collection(table)
	cursor, err := coll.Find(sampleCtx, map[string]interface{}{},
		options.Find().SetLimit(100).SetProjection(map[string]interface{}{"_id": 0}))
	if err != nil {
		return nil, fmt.Errorf("sample documents: %w", err)
	}
	defer func() { _ = cursor.Close(sampleCtx) }()

	fieldSet := make(map[string]string)
	for cursor.Next(sampleCtx) {
		var doc map[string]interface{}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		for k, v := range doc {
			if _, exists := fieldSet[k]; !exists {
				fieldSet[k] = inferBSONType(v)
			}
		}
	}

	columns := make([]driver.ColumnInfo, 0, len(fieldSet))
	for name, typ := range fieldSet {
		columns = append(columns, driver.ColumnInfo{Name: name, Type: typ})
	}
	return columns, nil
}

// ExecuteQuery executes a read-only query (find or aggregate).
// The query string is a JSON body in the MongoDB command format.
func (d *MongoDBDriver) ExecuteQuery(ctx context.Context, database string, query string, limit int) (*driver.QueryResult, error) {
	if d.client == nil {
		return nil, fmt.Errorf("mongodb: not connected")
	}
	if database == "" {
		return nil, fmt.Errorf("mongodb: database name is required")
	}

	if limit <= 0 {
		limit = 1000
	}

	// Parse the MongoDB command body
	mongoResult, err := sqlparser.ParseMongo(query)
	if err != nil {
		return nil, fmt.Errorf("parse mongodb command: %w", err)
	}

	coll := mongoResult.Collection
	if coll == "" {
		return nil, fmt.Errorf("mongodb: collection name is required")
	}

	collection := d.client.Database(database).Collection(coll)

	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	start := time.Now()

	var cursor *mongo.Cursor

	switch mongoResult.Operation {
	case sqlparser.MongoOpFind:
		findOpts := options.Find().SetLimit(int64(limit))
		filter := bson.M{}
		if mongoResult.HasFilter && !mongoResult.HasEmptyFilter {
			if f, ok := extractField(query, "filter"); ok {
				var parsed interface{}
				if err := bson.UnmarshalExtJSON([]byte(f), false, &parsed); err == nil {
					if m, ok := parsed.(map[string]interface{}); ok {
						for k, v := range m {
							filter[k] = v
						}
					}
				}
			}
		}
		cursor, err = collection.Find(queryCtx, filter, findOpts)

	case sqlparser.MongoOpAggregate:
		var pipeline bson.A
		if p, ok := extractField(query, "pipeline"); ok {
			var parsed interface{}
			if err := bson.UnmarshalExtJSON([]byte(p), false, &parsed); err == nil {
				if arr, ok := parsed.([]interface{}); ok {
					pipeline = arr
				}
			}
		}
		cursor, err = collection.Aggregate(queryCtx, pipeline)

	default:
		return nil, fmt.Errorf("mongodb: operation %s is not a query operation", mongoResult.Operation)
	}

	if err != nil {
		if queryCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("查询超时")
		}
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer func() { _ = cursor.Close(queryCtx) }()

	resultRows := make([]map[string]interface{}, 0, limit)
	rowCount := 0
	columnSet := make(map[string]bool)

	for cursor.Next(queryCtx) {
		if rowCount >= limit {
			break
		}
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("读取文档失败: %w", err)
		}
		row := make(map[string]interface{})
		for k, v := range doc {
			switch val := v.(type) {
			case primitive.ObjectID:
				row[k] = val.Hex()
			case primitive.DateTime:
				row[k] = val.Time().Format(time.RFC3339)
			default:
				row[k] = val
			}
			columnSet[k] = true
		}
		resultRows = append(resultRows, row)
		rowCount++
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果失败: %w", err)
	}

	// Extract column names from all collected fields (sorted for stable ordering)
	columns := make([]string, 0, len(columnSet))
	for k := range columnSet {
		columns = append(columns, k)
	}
	sort.Strings(columns)

	elapsed := time.Since(start).Milliseconds()

	return &driver.QueryResult{
		Columns:       columns,
		Rows:          resultRows,
		Total:         int64(len(resultRows)),
		ExecutionTime: elapsed,
	}, nil
}

// ExecuteStatement executes a single DML statement (insert, update, delete).
// The stmt string is a JSON body in the MongoDB command format.
func (d *MongoDBDriver) ExecuteStatement(ctx context.Context, database string, stmt string) (*driver.StatementResult, error) {
	if d.client == nil {
		return nil, fmt.Errorf("mongodb: not connected")
	}
	if database == "" {
		return nil, fmt.Errorf("mongodb: database name is required")
	}

	mongoResult, err := sqlparser.ParseMongo(stmt)
	if err != nil {
		return nil, fmt.Errorf("parse mongodb command: %w", err)
	}

	coll := mongoResult.Collection
	if coll == "" {
		return nil, fmt.Errorf("mongodb: collection name is required")
	}

	collection := d.client.Database(database).Collection(coll)
	start := time.Now()

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	switch mongoResult.Operation {
	case sqlparser.MongoOpInsert:
		doc := extractMap(stmt, "document")
		if doc == nil {
			docs := extractSlice(stmt, "documents")
			if docs == nil {
				return nil, fmt.Errorf("mongodb: document or documents field required")
			}
			if _, err := collection.InsertMany(execCtx, docs); err != nil {
				return errorResult(stmt, time.Since(start).Milliseconds(), err)
			}
			return successResult(stmt, int64(len(docs)), time.Since(start).Milliseconds()), nil
		}
		if _, err := collection.InsertOne(execCtx, doc); err != nil {
			return errorResult(stmt, time.Since(start).Milliseconds(), err)
		}
		return successResult(stmt, 1, time.Since(start).Milliseconds()), nil

	case sqlparser.MongoOpUpdate:
		filter := extractMap(stmt, "filter")
		update := extractMap(stmt, "update")
		if filter == nil || update == nil {
			return nil, fmt.Errorf("mongodb: filter and update fields required")
		}
		var res *mongo.UpdateResult
		if mongoResult.IsMulti {
			res, err = collection.UpdateMany(execCtx, filter, update)
		} else {
			res, err = collection.UpdateOne(execCtx, filter, update)
		}
		if err != nil {
			return errorResult(stmt, time.Since(start).Milliseconds(), err)
		}
		return successResult(stmt, res.ModifiedCount, time.Since(start).Milliseconds()), nil

	case sqlparser.MongoOpDelete:
		filter := extractMap(stmt, "filter")
		if filter == nil {
			return nil, fmt.Errorf("mongodb: filter field required")
		}
		res, err := collection.DeleteMany(execCtx, filter)
		if err != nil {
			return errorResult(stmt, time.Since(start).Milliseconds(), err)
		}
		return successResult(stmt, res.DeletedCount, time.Since(start).Milliseconds()), nil

	default:
		return nil, fmt.Errorf("mongodb: operation %s is not supported for execution", mongoResult.Operation)
	}
}

// ExecuteStatements 逐条执行多条 MongoDB 命令（MongoDB 无跨文档事务语义，逐条独立执行）。
// 任一语句失败后继续执行剩余语句，首错通过 error 返回。
// 降级实现：MongoDB 无跨命令事务，逐条调用 ExecuteStatement。
func (d *MongoDBDriver) ExecuteStatements(ctx context.Context, database string, statements []string) ([]driver.StatementResult, error) {
	if d.client == nil {
		return nil, fmt.Errorf("mongodb: not connected")
	}

	results := make([]driver.StatementResult, 0, len(statements))
	var firstErr error

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		r, err := d.ExecuteStatement(ctx, database, stmt)
		if r != nil {
			results = append(results, *r)
		}
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("statement failed: %v", err)
			}
			continue
		}
	}

	return results, firstErr
}

// Parse analyzes a MongoDB command JSON body.
func (d *MongoDBDriver) Parse(query string) (*driver.ParseResult, error) {
	result, err := sqlparser.ParseMongoDialect(query)
	if err != nil {
		return nil, err
	}

	pr := &driver.ParseResult{
		Operation: string(result.Operation),
		Targets:   result.Tables,
		Warnings:  result.Warnings,
	}

	if result.IsBlocked {
		pr.IsBlocked = true
		pr.BlockReason = result.BlockReason
	}

	pr.RiskLevel = string(result.RiskLevel)

	return pr, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractURI(cfg *driver.Config) string {
	// If host looks like a URI, use it directly. isURI is the single place that
	// decides what "looks like a URI" means; hand-rolled prefix slicing here
	// used to disagree with it in both directions — it panicked on any 11-byte
	// host, and its 12-byte slice could never equal the 11-byte "mongodb+srv".
	if isURI(cfg.Host) {
		return cfg.Host
	}
	// Try Extra["uri"]
	if cfg.Extra != nil {
		if uri, ok := cfg.Extra["uri"].(string); ok && uri != "" {
			return uri
		}
	}
	// Build from components.
	//
	// url.URL does the percent-encoding. Concatenation with fmt.Sprintf did not,
	// so a password containing any character a connection string gives meaning
	// to — /, @, :, ? — produced a URI the driver refused with "error parsing
	// uri", which reads like a configuration typo rather than a password
	// character.
	if cfg.Host != "" && cfg.Port > 0 && !isURI(cfg.Host) {
		u := url.URL{
			Scheme: "mongodb",
			Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		}
		if cfg.Username != "" {
			u.User = url.UserPassword(cfg.Username, cfg.Password)
		}
		if cfg.Database != "" {
			u.Path = "/" + cfg.Database
		}
		return u.String()
	}
	return ""
}

func isURI(s string) bool {
	return strings.HasPrefix(s, "mongodb://") || strings.HasPrefix(s, "mongodb+srv")
}

func extractField(jsonStr, field string) (string, bool) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return "", false
	}
	v, ok := m[field]
	if !ok {
		return "", false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	return string(b), true
}

func extractMap(jsonStr, field string) bson.M {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil
	}
	v, ok := m[field]
	if !ok {
		return nil
	}
	// Convert to bson.M via JSON round-trip
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var result bson.M
	if err := json.Unmarshal(b, &result); err != nil {
		return nil
	}
	return result
}

func extractSlice(jsonStr, field string) []interface{} {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil
	}
	v, ok := m[field]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	return arr
}

func inferBSONType(v interface{}) string {
	switch v.(type) {
	case string:
		return "string"
	case int, int32, int64:
		return "int"
	case float64:
		return "double"
	case bool:
		return "bool"
	case nil:
		return "null"
	case primitive.ObjectID:
		return "objectId"
	case primitive.DateTime:
		return "date"
	case primitive.Regex:
		return "regex"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "unknown"
	}
}

func errorResult(stmt string, ms int64, err error) (*driver.StatementResult, error) {
	return &driver.StatementResult{
		Statement:  stmt,
		Status:     "error",
		Error:      err.Error(),
		DurationMs: ms,
	}, err
}

// successResult builds the result of a statement that succeeded.
//
// It returns no error: there is nothing here that can fail, and a signature
// that says otherwise makes every caller write a branch that cannot be taken.
func successResult(stmt string, affected int64, ms int64) *driver.StatementResult {
	return &driver.StatementResult{
		Statement:    stmt,
		Status:       "success",
		RowsAffected: affected,
		DurationMs:   ms,
	}
}
