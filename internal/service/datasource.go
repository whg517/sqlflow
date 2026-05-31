package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	ErrDatasourceNotFound    = errors.New("数据源不存在")
	ErrDatasourceNameExists  = errors.New("数据源名称已存在")
	ErrDatasourceDisabled    = errors.New("数据源已禁用")
	ErrInvalidDatasourceType = errors.New("数据源类型必须是 mysql、postgresql、mongodb 或 elasticsearch")
)

var ValidDatasourceTypes = map[string]bool{"mysql": true, "postgresql": true, "mongodb": true, "elasticsearch": true}

// DatasourceService handles datasource management logic.
type DatasourceService struct {
	database      *db.DB
	encryptionKey string
	connMgr       *connpool.Manager
}

// NewDatasourceService creates a new DatasourceService.
func NewDatasourceService(database *db.DB, encryptionKey string, connMgr *connpool.Manager) *DatasourceService {
	return &DatasourceService{database: database, encryptionKey: encryptionKey, connMgr: connMgr}
}

// CreateDataSource creates a new datasource with encrypted password.
func (s *DatasourceService) CreateDataSource(ctx context.Context, ds *model.DataSource) error {
	if !ValidDatasourceTypes[ds.Type] {
		return ErrInvalidDatasourceType
	}

	// ES security: enforce HTTPS
	if ds.Type == "elasticsearch" {
		if err := validateESURLs(ds.ESUrls); err != nil {
			return err
		}
	}

	encrypted, err := crypto.Encrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return fmt.Errorf("encrypt password: %w", err)
	}

	// Apply defaults
	if ds.MaxOpen == 0 {
		ds.MaxOpen = 10
	}
	if ds.MaxIdle == 0 {
		ds.MaxIdle = 5
	}
	if ds.MaxLifetime == 0 {
		ds.MaxLifetime = 3600
	}
	if ds.MaxIdleTime == 0 {
		ds.MaxIdleTime = 600
	}
	if ds.Status == "" {
		ds.Status = "active"
	}

	// 加密 ES API Key（如果使用 API Key 认证）
	encryptedESApiKey := ""
	if ds.ESApiKey != "" {
		enc, err := crypto.Encrypt(ds.ESApiKey, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("encrypt es_api_key: %w", err)
		}
		encryptedESApiKey = enc
	}

	result, err := s.database.DB.ExecContext(ctx,
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, database, sslmode, schema_name, max_open, max_idle, max_lifetime, max_idle_time, status, es_urls, es_version, es_auth_type, es_api_key, es_index_pattern, es_verify_certs)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ds.Name, ds.Type, ds.Host, ds.Port, ds.Username, encrypted, ds.Database, ds.SSLMode, ds.SchemaName,
		ds.MaxOpen, ds.MaxIdle, ds.MaxLifetime, ds.MaxIdleTime, ds.Status,
		ds.ESUrls, ds.ESVersion, ds.ESAuthType, encryptedESApiKey, ds.ESIndexPattern, ds.ESVerifyCerts,
	)
	if err != nil {
		return fmt.Errorf("insert datasource: %w", err)
	}

	id, _ := result.LastInsertId()
	created, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}
	*ds = *created
	return nil
}

// ListDataSources returns all datasources without encrypted passwords.
func (s *DatasourceService) ListDataSources(ctx context.Context) ([]model.DataSource, error) {
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT id, name, type, host, port, username, database, sslmode, schema_name, max_open, max_idle, max_lifetime, max_idle_time, status, es_urls, es_version, es_auth_type, es_index_pattern, es_verify_certs, created_at, updated_at
		 FROM datasources ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("query datasources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []model.DataSource
	for rows.Next() {
		var ds model.DataSource
		if err := rows.Scan(&ds.ID, &ds.Name, &ds.Type, &ds.Host, &ds.Port, &ds.Username, &ds.Database,
			&ds.SSLMode, &ds.SchemaName,
			&ds.MaxOpen, &ds.MaxIdle, &ds.MaxLifetime, &ds.MaxIdleTime, &ds.Status,
			&ds.ESUrls, &ds.ESVersion, &ds.ESAuthType, &ds.ESIndexPattern, &ds.ESVerifyCerts,
			&ds.CreatedAt, &ds.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan datasource: %w", err)
		}
		list = append(list, ds)
	}
	return list, rows.Err()
}

// GetDataSource returns a single datasource by ID (password not decrypted).
func (s *DatasourceService) GetDataSource(ctx context.Context, id int64) (*model.DataSource, error) {
	ds := &model.DataSource{}
	err := s.database.DB.QueryRowContext(ctx,
		`SELECT id, name, type, host, port, username, password_encrypted, database, sslmode, schema_name, max_open, max_idle, max_lifetime, max_idle_time, status, es_urls, es_version, es_auth_type, es_api_key, es_index_pattern, es_verify_certs, created_at, updated_at
		 FROM datasources WHERE id = ?`, id,
	).Scan(&ds.ID, &ds.Name, &ds.Type, &ds.Host, &ds.Port, &ds.Username, &ds.PasswordEncrypted, &ds.Database,
		&ds.SSLMode, &ds.SchemaName,
		&ds.MaxOpen, &ds.MaxIdle, &ds.MaxLifetime, &ds.MaxIdleTime, &ds.Status,
		&ds.ESUrls, &ds.ESVersion, &ds.ESAuthType, &ds.ESApiKey, &ds.ESIndexPattern, &ds.ESVerifyCerts,
		&ds.CreatedAt, &ds.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDatasourceNotFound
		}
		return nil, fmt.Errorf("query datasource: %w", err)
	}
	return ds, nil
}

// UpdateDataSource updates an existing datasource.
func (s *DatasourceService) UpdateDataSource(ctx context.Context, id int64, ds *model.DataSource) error {
	if !ValidDatasourceTypes[ds.Type] {
		return ErrInvalidDatasourceType
	}

	// ES security: enforce HTTPS
	if ds.Type == "elasticsearch" {
		if err := validateESURLs(ds.ESUrls); err != nil {
			return err
		}
	}

	// Get existing datasource for pool invalidation
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}

	// Build update query — if password is provided, re-encrypt; otherwise keep existing
	var encrypted string
	if ds.PasswordEncrypted != "" {
		enc, err := crypto.Encrypt(ds.PasswordEncrypted, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("encrypt password: %w", err)
		}
		encrypted = enc
	} else {
		encrypted = existing.PasswordEncrypted
	}

	// ES API Key: 如果提供了新的则加密，否则保留现有值
	var encryptedESApiKey string
	if ds.ESApiKey != "" {
		enc, err := crypto.Encrypt(ds.ESApiKey, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("encrypt es_api_key: %w", err)
		}
		encryptedESApiKey = enc
	} else {
		encryptedESApiKey = existing.ESApiKey
	}

	result, err := s.database.DB.ExecContext(ctx,
		`UPDATE datasources SET name=?, type=?, host=?, port=?, username=?, password_encrypted=?, database=?, sslmode=?, schema_name=?,
		 max_open=?, max_idle=?, max_lifetime=?, max_idle_time=?, es_urls=?, es_version=?, es_auth_type=?, es_api_key=?, es_index_pattern=?, es_verify_certs=?, updated_at=datetime('now') WHERE id=?`,
		ds.Name, ds.Type, ds.Host, ds.Port, ds.Username, encrypted, ds.Database, ds.SSLMode, ds.SchemaName,
		ds.MaxOpen, ds.MaxIdle, ds.MaxLifetime, ds.MaxIdleTime,
		ds.ESUrls, ds.ESVersion, ds.ESAuthType, encryptedESApiKey, ds.ESIndexPattern, ds.ESVerifyCerts,
		id,
	)
	if err != nil {
		return fmt.Errorf("update datasource: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrDatasourceNotFound
	}

	// Invalidate cached connection pool since config may have changed
	if ds.Type == "mysql" {
		s.connMgr.Remove(id, ds.Host, ds.Port, ds.Database)
		if existing.Host != ds.Host || existing.Port != ds.Port || existing.Database != ds.Database {
			s.connMgr.Remove(id, existing.Host, existing.Port, existing.Database)
		}
	}
	if ds.Type == "postgresql" {
		s.connMgr.RemovePG(id, ds.Host, ds.Port, ds.Database)
		if existing.Host != ds.Host || existing.Port != ds.Port || existing.Database != ds.Database {
			s.connMgr.RemovePG(id, existing.Host, existing.Port, existing.Database)
		}
	}
	if ds.Type == "mongodb" || existing.Type == "mongodb" {
		s.connMgr.RemoveMongo(id)
	}
	if ds.Type == "elasticsearch" || existing.Type == "elasticsearch" {
		s.connMgr.RemoveElasticsearch(id)
	}

	return nil
}
func (s *DatasourceService) DisableDataSource(ctx context.Context, id int64) error {
	// Get existing datasource for pool cleanup
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}

	result, err := s.database.DB.ExecContext(ctx,
		`UPDATE datasources SET status='disabled', updated_at=datetime('now') WHERE id=?`, id,
	)
	if err != nil {
		return fmt.Errorf("disable datasource: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrDatasourceNotFound
	}

	// Clean up cached connection pool
	if existing.Type == "mysql" {
		s.connMgr.Remove(id, existing.Host, existing.Port, existing.Database)
	}
	if existing.Type == "postgresql" {
		s.connMgr.RemovePG(id, existing.Host, existing.Port, existing.Database)
	}
	if existing.Type == "mongodb" {
		s.connMgr.RemoveMongo(id)
	}
	if existing.Type == "elasticsearch" {
		s.connMgr.RemoveElasticsearch(id)
	}

	return nil
}

// TestConnection attempts to connect to the datasource.
func (s *DatasourceService) TestConnection(ctx context.Context, ds *model.DataSource) error {
	password := ds.PasswordEncrypted

	// If the datasource has an ID, try to decrypt the stored password
	if ds.ID > 0 {
		stored, err := s.GetDataSource(ctx, ds.ID)
		if err != nil {
			return err
		}
		decrypted, err := crypto.Decrypt(stored.PasswordEncrypted, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("decrypt password: %w", err)
		}
		password = decrypted
	}

	switch ds.Type {
	case "mysql":
		return connpool.MySQLPing(ctx, ds.Host, ds.Port, ds.Username, password)
	case "postgresql":
		return connpool.PostgreSQLPing(ctx, ds.Host, ds.Port, ds.Username, password, ds.Database, ds.SSLMode)
	case "mongodb":
		uri := buildMongoURI(ds.Host, ds.Port, ds.Username, password)
		return connpool.MongoPing(ctx, uri)
	case "elasticsearch":
		if err := validateESURLs(ds.ESUrls); err != nil {
			return err
		}
		urls := parseESUrls(ds.ESUrls)
		esApiKey := ""
		if ds.ESApiKey != "" {
			dec, err := crypto.Decrypt(ds.ESApiKey, s.encryptionKey)
			if err != nil {
				return fmt.Errorf("decrypt es_api_key: %w", err)
			}
			esApiKey = dec
		}
		return connpool.ElasticsearchPing(ctx, urls, ds.ESAuthType, ds.Username, password, esApiKey, ds.ESVerifyCerts)
	default:
		return ErrInvalidDatasourceType
	}
}

// GetTables returns table names for a MySQL datasource or database names for MongoDB.
func (s *DatasourceService) GetTables(ctx context.Context, id int64) ([]string, error) {
	ds, err := s.GetDataSource(ctx, id)
	if err != nil {
		return nil, err
	}

	if ds.Status == "disabled" {
		return nil, ErrDatasourceDisabled
	}

	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt password: %w", err)
	}

	switch ds.Type {
	case "mysql":
		dbName := ds.Database
		if dbName == "" {
			dbName = "information_schema"
		}
		poolCfg := connpool.MySQLPoolConfig{
			MaxOpen:     ds.MaxOpen,
			MaxIdle:     ds.MaxIdle,
			MaxLifetime: ds.MaxLifetime,
			MaxIdleTime: ds.MaxIdleTime,
		}
		targetDB, err := s.connMgr.GetMySQL(id, ds.Host, ds.Port, ds.Username, password, dbName, poolCfg)
		if err != nil {
			return nil, fmt.Errorf("connect mysql: %w", err)
		}
		rows, err := targetDB.QueryContext(ctx, "SHOW TABLES")
		if err != nil {
			return nil, fmt.Errorf("show tables: %w", err)
		}
		defer func() { _ = rows.Close() }()

		tables := make([]string, 0)
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, fmt.Errorf("scan table name: %w", err)
			}
			tables = append(tables, name)
		}
		return tables, rows.Err()
	case "postgresql":
		return s.getPGTables(ctx, ds, password)
	case "mongodb":
		uri := buildMongoURI(ds.Host, ds.Port, ds.Username, password)
		return s.connMgr.GetMongoDatabaseNames(ctx, id, uri)
	default:
		return nil, ErrInvalidDatasourceType
	}
}

// ColumnInfo represents a table column's metadata.
type ColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment"`
}

// GetTableColumns returns column information for a specific table in a datasource.
func (s *DatasourceService) GetTableColumns(ctx context.Context, id int64, tableName string) ([]ColumnInfo, error) {
	ds, err := s.GetDataSource(ctx, id)
	if err != nil {
		return nil, err
	}

	if ds.Status == "disabled" {
		return nil, ErrDatasourceDisabled
	}

	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt password: %w", err)
	}

	switch ds.Type {
	case "mysql":
		return s.getMySQLColumns(ctx, ds, password, tableName)
	case "postgresql":
		return s.getPGColumns(ctx, ds, password, tableName)
	case "mongodb":
		return s.getMongoColumns(ctx, ds, password, tableName)
	default:
		return nil, ErrInvalidDatasourceType
	}
}

// getMySQLColumns queries INFORMATION_SCHEMA.COLUMNS for column metadata.
func (s *DatasourceService) getMySQLColumns(ctx context.Context, ds *model.DataSource, password, tableName string) ([]ColumnInfo, error) {
	dbName := ds.Database
	if dbName == "" {
		dbName = "information_schema"
	}
	poolCfg := connpool.MySQLPoolConfig{
		MaxOpen:     ds.MaxOpen,
		MaxIdle:     ds.MaxIdle,
		MaxLifetime: ds.MaxLifetime,
		MaxIdleTime: ds.MaxIdleTime,
	}
	targetDB, err := s.connMgr.GetMySQL(ds.ID, ds.Host, ds.Port, ds.Username, password, dbName, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect mysql: %w", err)
	}

	query := `SELECT COLUMN_NAME, DATA_TYPE, COLUMN_COMMENT FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION`
	rows, err := targetDB.QueryContext(ctx, query, dbName, tableName)
	if err != nil {
		return nil, fmt.Errorf("query columns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var columns []ColumnInfo
	for rows.Next() {
		var c ColumnInfo
		if err := rows.Scan(&c.Name, &c.Type, &c.Comment); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		columns = append(columns, c)
	}
	return columns, rows.Err()
}

// getMongoColumns samples documents from a MongoDB collection to infer field types.
func (s *DatasourceService) getMongoColumns(ctx context.Context, ds *model.DataSource, password, tableName string) ([]ColumnInfo, error) {
	uri := buildMongoURI(ds.Host, ds.Port, ds.Username, password)
	client, err := s.connMgr.GetMongoDB(ctx, ds.ID, uri)
	if err != nil {
		return nil, fmt.Errorf("connect mongodb: %w", err)
	}

	database := ds.Database
	if database == "" {
		database = tableName // If no database specified, tableName might be the DB name
	}
	collection := client.Database(database).Collection(tableName)

	// Sample up to 100 documents to infer field names and types
	pipeline := []bson.M{
		{"$limit": int32(100)},
		{"$project": bson.M{"_id": 0}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate mongo columns: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	seen := make(map[string]string) // name → type
	for cursor.Next(ctx) {
		var doc map[string]interface{}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		for field, val := range doc {
			t := "unknown"
			if val != nil {
				switch val.(type) {
				case string:
					t = "string"
				case float64:
					t = "number"
				case bool:
					t = "boolean"
				case int, int32, int64:
					t = "number"
				case map[string]interface{}:
					t = "object"
				case []interface{}:
					t = "array"
				}
			}
			if _, exists := seen[field]; !exists {
				seen[field] = t
			}
		}
	}

	columns := make([]ColumnInfo, 0, len(seen))
	for name, typ := range seen {
		columns = append(columns, ColumnInfo{Name: name, Type: typ, Comment: ""})
	}
	return columns, nil
}

// GetDataSourceSafe returns a datasource without the encrypted password for API responses.
func (s *DatasourceService) GetDataSourceSafe(ctx context.Context, id int64) (*model.DataSource, error) {
	ds, err := s.GetDataSource(ctx, id)
	if err != nil {
		return nil, err
	}
	ds.PasswordEncrypted = ""
	return ds, nil
}

// getPGTables returns table names from a PostgreSQL datasource.
func (s *DatasourceService) getPGTables(ctx context.Context, ds *model.DataSource, password string) ([]string, error) {
	schemaName := ds.SchemaName
	if schemaName == "" {
		schemaName = "public"
	}
	dbName := ds.Database
	if dbName == "" {
		dbName = "postgres"
	}
	poolCfg := connpool.PGPoolConfig{
		MaxOpen:     ds.MaxOpen,
		MaxIdle:     ds.MaxIdle,
		MaxLifetime: ds.MaxLifetime,
		MaxIdleTime: ds.MaxIdleTime,
	}
	targetDB, err := s.connMgr.GetPostgreSQL(ds.ID, ds.Host, ds.Port, ds.Username, password, dbName, ds.SSLMode, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgresql: %w", err)
	}
	rows, err := targetDB.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = $1 ORDER BY tablename`, schemaName)
	if err != nil {
		return nil, fmt.Errorf("list postgresql tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tables := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// getPGColumns queries information_schema.columns for PostgreSQL column metadata.
func (s *DatasourceService) getPGColumns(ctx context.Context, ds *model.DataSource, password, tableName string) ([]ColumnInfo, error) {
	schemaName := ds.SchemaName
	if schemaName == "" {
		schemaName = "public"
	}
	dbName := ds.Database
	if dbName == "" {
		dbName = "postgres"
	}
	poolCfg := connpool.PGPoolConfig{
		MaxOpen:     ds.MaxOpen,
		MaxIdle:     ds.MaxIdle,
		MaxLifetime: ds.MaxLifetime,
		MaxIdleTime: ds.MaxIdleTime,
	}
	targetDB, err := s.connMgr.GetPostgreSQL(ds.ID, ds.Host, ds.Port, ds.Username, password, dbName, ds.SSLMode, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgresql: %w", err)
	}

	// Map PostgreSQL data_type to simplified types for frontend display
	// Also fetch udt_name for ARRAY types to preserve element type (e.g. "integer[]")
	query := `SELECT column_name, data_type, udt_name, COALESCE(col_description(($1||'.'||$2)::regclass, ordinal_position), '') AS comment
		 FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2
		 ORDER BY ordinal_position`
	rows, err := targetDB.QueryContext(ctx, query, schemaName, tableName)
	if err != nil {
		return nil, fmt.Errorf("query postgresql columns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var columns []ColumnInfo
	for rows.Next() {
		var c ColumnInfo
		var udtName string
		if err := rows.Scan(&c.Name, &c.Type, &udtName, &c.Comment); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		c.Type = mapPGType(c.Type, udtName)
		columns = append(columns, c)
	}
	return columns, rows.Err()
}

// mapPGType normalizes PostgreSQL data_type names to user-friendly types.
// For ARRAY types, udtName is used to preserve element type (e.g. "integer[]", "text[]").
func mapPGType(pgType, udtName string) string {
	switch pgType {
	case "smallint", "integer", "bigint", "int", "int2", "int4", "int8":
		return "integer"
	case "decimal", "numeric", "real", "double precision", "float4", "float8":
		return "number"
	case "character varying", "character", "text", "char", "varchar", "bpchar", "name":
		return "string"
	case "boolean", "bool":
		return "boolean"
	case "date":
		return "date"
	case "timestamp without time zone", "timestamp with time zone", "timestamp", "timestamptz":
		return "timestamp"
	case "time without time zone", "time with time zone", "time", "timetz":
		return "time"
	case "uuid":
		return "uuid"
	case "json", "jsonb":
		return "json"
	case "bytea":
		return "binary"
	case "ARRAY":
		// Preserve element type from udt_name (e.g. _int4 → integer[], _text → text[])
		return mapArrayElementType(udtName)
	default:
		return pgType
	}
}

// buildMongoURI constructs a MongoDB connection URI.
// Format: mongodb://user:password@host:port (with credentials) or mongodb://host:port (without)
func buildMongoURI(host string, port int, user, password string) string {
	if user != "" && password != "" {
		return fmt.Sprintf("mongodb://%s:%s@%s:%d",
			url.QueryEscape(user), url.QueryEscape(password), host, port)
	}
	return fmt.Sprintf("mongodb://%s:%d", host, port)
}

// parseESUrls 将逗号分隔的 ES URL 字符串解析为 []string。
// validateESURLs checks that all ES URLs use HTTPS.
// Returns an error if any URL uses plain HTTP (security requirement).
func validateESURLs(raw string) error {
	urls := parseESUrls(raw)
	for _, u := range urls {
		if strings.HasPrefix(u, "http://") {
			return fmt.Errorf("Elasticsearch 连接地址必须使用 HTTPS，当前地址 %s 使用了 HTTP", u)
		}
	}
	return nil
}

func parseESUrls(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	urls := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			urls = append(urls, trimmed)
		}
	}
	return urls
}

// mapArrayElementType converts PostgreSQL udt_name for arrays to a readable type.
// PostgreSQL stores array types with a leading underscore (e.g. _int4, _text, _float8).
func mapArrayElementType(udtName string) string {
	if len(udtName) == 0 || udtName[0] != '_' {
		return "array"
	}
	elemType := udtName[1:]
	switch elemType {
	case "int2":
		return "smallint[]"
	case "int4":
		return "integer[]"
	case "int8":
		return "bigint[]"
	case "float4":
		return "real[]"
	case "float8":
		return "double precision[]"
	case "numeric":
		return "numeric[]"
	case "text", "varchar", "bpchar", "char", "name":
		return "text[]"
	case "bool":
		return "boolean[]"
	case "date":
		return "date[]"
	case "timestamp", "timestamptz":
		return "timestamp[]"
	case "uuid":
		return "uuid[]"
	case "json", "jsonb":
		return "json[]"
	default:
		return elemType + "[]"
	}
}
