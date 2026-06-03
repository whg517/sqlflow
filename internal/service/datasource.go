package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	es "github.com/elastic/go-elasticsearch/v8"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/db/ent/datasource"
	"github.com/whg517/sqlflow/internal/driver"
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
	client        *ent.Client
	encryptionKey string
	connMgr       *connpool.Manager
	poolMgr       *driver.PoolManager
}

// NewDatasourceService creates a new DatasourceService.
func NewDatasourceService(database *db.DB, encryptionKey string, connMgr *connpool.Manager, poolMgr *driver.PoolManager) *DatasourceService {
	return &DatasourceService{database: database, client: database.Client(), encryptionKey: encryptionKey, connMgr: connMgr, poolMgr: poolMgr}
}

// PoolManager returns the driver PoolManager (may be nil if not configured).
func (s *DatasourceService) PoolManager() *driver.PoolManager {
	return s.poolMgr
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

	created, err := s.client.DataSource.Create().
		SetName(ds.Name).
		SetType(ds.Type).
		SetHost(ds.Host).
		SetPort(ds.Port).
		SetUsername(ds.Username).
		SetPasswordEncrypted(encrypted).
		SetDatabase(ds.Database).
		SetSslmode(ds.SSLMode).
		SetSchemaName(ds.SchemaName).
		SetMaxOpen(ds.MaxOpen).
		SetMaxIdle(ds.MaxIdle).
		SetMaxLifetime(ds.MaxLifetime).
		SetMaxIdleTime(ds.MaxIdleTime).
		SetStatus(ds.Status).
		SetEsUrls(ds.ESUrls).
		SetEsVersion(ds.ESVersion).
		SetEsAuthType(ds.ESAuthType).
		SetEsAPIKey(encryptedESApiKey).
		SetEsIndexPattern(ds.ESIndexPattern).
		SetEsVerifyCerts(ds.ESVerifyCerts).
		SetExtraConfig(ds.ExtraConfig).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("insert datasource: %w", err)
	}

	found, err := s.GetDataSource(ctx, int64(created.ID))
	if err != nil {
		return err
	}
	*ds = *found
	return nil
}

// ListDataSources returns all datasources without encrypted passwords.
func (s *DatasourceService) ListDataSources(ctx context.Context) ([]model.DataSource, error) {
	results, err := s.client.DataSource.Query().
		Order(datasource.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query datasources: %w", err)
	}

	var list []model.DataSource
	for _, d := range results {
		list = append(list, entDatasourceToModel(d))
	}
	return list, nil
}

// GetDataSource returns a single datasource by ID (password not decrypted).
func (s *DatasourceService) GetDataSource(ctx context.Context, id int64) (*model.DataSource, error) {
	d, err := s.client.DataSource.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrDatasourceNotFound
		}
		return nil, fmt.Errorf("query datasource: %w", err)
	}
	result := entDatasourceToModel(d)
	result.PasswordEncrypted = d.PasswordEncrypted
	result.ESApiKey = d.EsAPIKey
	return &result, nil
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

	n, err := s.client.DataSource.UpdateOneID(int(id)).
		SetName(ds.Name).
		SetType(ds.Type).
		SetHost(ds.Host).
		SetPort(ds.Port).
		SetUsername(ds.Username).
		SetPasswordEncrypted(encrypted).
		SetDatabase(ds.Database).
		SetSslmode(ds.SSLMode).
		SetSchemaName(ds.SchemaName).
		SetMaxOpen(ds.MaxOpen).
		SetMaxIdle(ds.MaxIdle).
		SetMaxLifetime(ds.MaxLifetime).
		SetMaxIdleTime(ds.MaxIdleTime).
		SetEsUrls(ds.ESUrls).
		SetEsVersion(ds.ESVersion).
		SetEsAuthType(ds.ESAuthType).
		SetEsAPIKey(encryptedESApiKey).
		SetEsIndexPattern(ds.ESIndexPattern).
		SetEsVerifyCerts(ds.ESVerifyCerts).
		SetExtraConfig(ds.ExtraConfig).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrDatasourceNotFound
		}
		return fmt.Errorf("update datasource: %w", err)
	}
	_ = n

	// Invalidate cached connection pool since config may have changed
	// Use PoolManager (Driver) if available
	if s.poolMgr != nil {
		s.poolMgr.Remove(id)
	}

	// Also invalidate legacy connpool for backward compatibility
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

	err = s.client.DataSource.UpdateOneID(int(id)).
		SetStatus("disabled").
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrDatasourceNotFound
		}
		return fmt.Errorf("disable datasource: %w", err)
	}

	// Clean up cached connection pool
	if s.poolMgr != nil {
		s.poolMgr.Remove(id)
	}
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

// TestConnection attempts to connect to the datasource using the Driver abstraction.
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
		// Use stored values for ES fields that need decrypted API key
		ds.ESUrls = stored.ESUrls
		ds.ESAuthType = stored.ESAuthType
		ds.ESApiKey = stored.ESApiKey
		ds.ESIndexPattern = stored.ESIndexPattern
		ds.ESVerifyCerts = stored.ESVerifyCerts
	}

	// If poolMgr is available, use Driver abstraction for all types.
	// NOTE: This means the legacy connpool path below is dead code when poolMgr is set.
	// When poolMgr is fully adopted and connpool is removed, this comment and the
	// fallback block below should be deleted.
	if s.poolMgr != nil {
		adapter := newDataSourceAdapter(ds)
		cfg, err := driver.BuildConfigFromDataSource(adapter, password, "")
		if err != nil {
			return err
		}
		d, err := driver.NewDriver(ds.Type)
		if err != nil {
			return err
		}
		if err := d.Connect(ctx, cfg); err != nil {
			return err
		}
		defer d.Close()
		return d.Ping(ctx)
	}

	// Fallback to legacy connpool
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
		esAPIKey := ""
		if ds.ESApiKey != "" {
			dec, err := crypto.Decrypt(ds.ESApiKey, s.encryptionKey)
			if err != nil {
				return fmt.Errorf("decrypt es_api_key: %w", err)
			}
			esAPIKey = dec
		}
		return connpool.ElasticsearchPing(ctx, urls, ds.ESAuthType, ds.Username, password, esAPIKey, ds.ESVerifyCerts)
	default:
		return ErrInvalidDatasourceType
	}
}

// GetTables returns table names for a datasource.
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

	// Use Driver abstraction if available (MySQL, PG)
	if s.poolMgr != nil {
		switch ds.Type {
		case "mysql", "postgresql":
			adapter := newDataSourceAdapter(ds)
			cfg, err := driver.BuildConfigFromDataSource(adapter, password, "")
			if err != nil {
				return nil, err
			}
			d, err := s.poolMgr.Get(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("connect %s: %w", ds.Type, err)
			}
			dbName := ds.Database
			if dbName == "" && ds.Type == "mysql" {
				dbName = "information_schema"
			}
			if dbName == "" && ds.Type == "postgresql" {
				dbName = "postgres"
			}
			tables, err := d.ListTables(ctx, dbName)
			if err != nil {
				return nil, err
			}
			names := make([]string, 0, len(tables))
			for _, t := range tables {
				names = append(names, t.Name)
			}
			return names, nil
		}
	}

	// Fallback to legacy connpool
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

	// Use Driver abstraction if available (MySQL, PG)
	if s.poolMgr != nil {
		switch ds.Type {
		case "mysql", "postgresql":
			adapter := newDataSourceAdapter(ds)
			cfg, err := driver.BuildConfigFromDataSource(adapter, password, "")
			if err != nil {
				return nil, err
			}
			d, err := s.poolMgr.Get(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("connect %s: %w", ds.Type, err)
			}
			dbName := ds.Database
			if dbName == "" && ds.Type == "mysql" {
				dbName = "information_schema"
			}
			if dbName == "" && ds.Type == "postgresql" {
				dbName = "postgres"
			}
			columns, err := d.GetColumns(ctx, dbName, tableName)
			if err != nil {
				return nil, err
			}
			svcColumns := make([]ColumnInfo, 0, len(columns))
			for _, c := range columns {
				svcColumns = append(svcColumns, ColumnInfo{
					Name:    c.Name,
					Type:    c.Type,
					Comment: c.Comment,
				})
			}
			return svcColumns, nil
		}
	}

	// Fallback to legacy connpool
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
// ESIndexInfo represents metadata for a single Elasticsearch index.
type ESIndexInfo struct {
	Name        string `json:"name"`
	Health      string `json:"health"`       // green, yellow, red
	Status      string `json:"status"`       // open, closed
	DocCount    int64  `json:"doc_count"`
	StoreSize   string `json:"store_size"`   // human-readable, e.g. "4.2mb"
	StoreBytes  int64  `json:"store_bytes"`  // raw bytes
	CreatedTime string `json:"created_time"` // ISO 8601
}

// ESIndexField represents a field in an Elasticsearch index mapping.
type ESIndexField struct {
	Name        string         `json:"name"`
	ESType      string         `json:"es_type"`       // text, keyword, date, long, boolean, nested, object, etc.
	Searchable  bool           `json:"searchable"`
	Aggregatable bool          `json:"aggregatable"`
	SubFields   []ESIndexField `json:"sub_fields,omitempty"` // nested/object children
}

// getESClient is a helper that resolves and returns an ES client for a datasource.
func (s *DatasourceService) getESClient(ctx context.Context, id int64) (*model.DataSource, string, *es.Client, error) {
	ds, err := s.GetDataSource(ctx, id)
	if err != nil {
		return nil, "", nil, err
	}
	if ds.Status == "disabled" {
		return nil, "", nil, ErrDatasourceDisabled
	}
	if ds.Type != "elasticsearch" {
		return nil, "", nil, ErrInvalidDatasourceType
	}

	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, "", nil, fmt.Errorf("decrypt password: %w", err)
	}

	urls := parseESUrls(ds.ESUrls)
	if len(urls) == 0 {
		return nil, "", nil, fmt.Errorf("Elasticsearch 数据源未配置连接地址")
	}

	esAPIKey := ""
	if ds.ESApiKey != "" {
		dec, err := crypto.Decrypt(ds.ESApiKey, s.encryptionKey)
		if err != nil {
			return nil, "", nil, fmt.Errorf("解密 ES API Key 失败: %w", err)
		}
		esAPIKey = dec
	}

	client, err := s.connMgr.GetElasticsearch(ctx, id, urls, ds.ESAuthType, ds.Username, password, esAPIKey, ds.ESVerifyCerts)
	if err != nil {
		return nil, "", nil, fmt.Errorf("连接 Elasticsearch 失败: %w", err)
	}
	return ds, password, client, nil
}

// GetESIndices returns the list of indices in the Elasticsearch cluster.
// Supports keyword filtering and pagination.
func (s *DatasourceService) GetESIndices(ctx context.Context, id int64, query string, page, pageSize int) ([]ESIndexInfo, int, error) {
	_, _, client, err := s.getESClient(ctx, id)
	if err != nil {
		return nil, 0, err
	}

	// Use _cat/indices API with format=json
	req := esapi.CatIndicesRequest{
		Format: "json",
	}

	resp, err := req.Do(ctx, client)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, 0, fmt.Errorf("查询 ES 索引超时")
		}
		return nil, 0, fmt.Errorf("查询 ES 索引失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return nil, 0, fmt.Errorf("ES _cat/indices 返回错误: %s", resp.Status())
	}

	// Parse response
	var rawIndices []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawIndices); err != nil {
		return nil, 0, fmt.Errorf("解析 ES 索引响应失败: %w", err)
	}

	// Map to ESIndexInfo and filter
	var all []ESIndexInfo
	for _, raw := range rawIndices {
		name := getStrVal(raw, "index")

		// Filter by query keyword
		if query != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
			continue
		}

		// Skip system indices (starting with .) unless explicitly searched
		if query == "" && strings.HasPrefix(name, ".") {
			continue
		}

		info := ESIndexInfo{
			Name:        name,
			Health:      getStrVal(raw, "health"),
			Status:      getStrVal(raw, "status"),
			StoreSize:   getStrVal(raw, "store.size"),
			CreatedTime: getStrVal(raw, "creation.date.string"),
		}
		info.DocCount, _ = strconv.ParseInt(getStrVal(raw, "docs.count"), 10, 64)
		info.StoreBytes, _ = strconv.ParseInt(getStrVal(raw, "store.size"), 10, 64)

		all = append(all, info)
	}

	total := len(all)

	// Paginate
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	return all[start:end], total, nil
}

// GetESIndexFields returns the field mapping for a specific Elasticsearch index.
func (s *DatasourceService) GetESIndexFields(ctx context.Context, id int64, indexName string) ([]ESIndexField, error) {
	_, _, client, err := s.getESClient(ctx, id)
	if err != nil {
		return nil, err
	}

	if indexName == "" {
		return nil, fmt.Errorf("索引名称不能为空")
	}

	// Use ES _mapping API
	req := esapi.IndicesGetMappingRequest{
		Index: []string{indexName},
	}

	resp, err := req.Do(ctx, client)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("查询 ES 索引字段超时")
		}
		return nil, fmt.Errorf("查询 ES 索引字段失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("索引 %q 不存在", indexName)
		}
		return nil, fmt.Errorf("ES _mapping 返回错误: %s", resp.Status())
	}

	// Parse mapping response: { "index_name": { "mappings": { "properties": { ... } } } }
	var mappingResp map[string]struct {
		Mappings struct {
			Properties map[string]interface{} `json:"properties"`
		} `json:"mappings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mappingResp); err != nil {
		return nil, fmt.Errorf("解析 ES mapping 响应失败: %w", err)
	}

	// Extract fields from the first (and usually only) index in the response
	for _, idxData := range mappingResp {
		return parseESProperties(idxData.Mappings.Properties), nil
	}

	return nil, fmt.Errorf("索引 %q 的 mapping 为空", indexName)
}

// parseESProperties recursively parses ES mapping properties into ESIndexField slice.
func parseESProperties(props map[string]interface{}) []ESIndexField {
	var fields []ESIndexField

	// Sort field names for deterministic output
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		propData, ok := props[name]
		if !ok {
			continue
		}
		propMap, ok := propData.(map[string]interface{})
		if !ok {
			continue
		}

		field := ESIndexField{
			Name:   name,
			ESType: getStrVal(propMap, "type"),
		}

		// Determine searchable / aggregatable from "index" field
		// Default: indexed (searchable) unless explicitly set to false
		if idx, ok := propMap["index"]; ok {
			field.Searchable = idx != false
		} else {
			field.Searchable = true
		}

		// Aggregatable: keyword type and text with fielddata are aggregatable
		field.Aggregatable = isAggregatable(field.ESType, propMap)

		// Recurse for nested/object types
		if field.ESType == "nested" || field.ESType == "object" {
			if subProps, ok := propMap["properties"]; ok {
				if subMap, ok := subProps.(map[string]interface{}); ok {
					field.SubFields = parseESProperties(subMap)
				}
			}
		}

		// Also handle multi-fields ("fields" key)
		if subFields, ok := propMap["fields"]; ok {
			if subMap, ok := subFields.(map[string]interface{}); ok {
				for subName, subData := range subMap {
					if sm, ok := subData.(map[string]interface{}); ok {
						field.SubFields = append(field.SubFields, ESIndexField{
							Name:        name + "." + subName,
							ESType:      getStrVal(sm, "type"),
							Searchable:  true,
							Aggregatable: true, // multi-fields are typically keyword for agg
						})
					}
				}
				sort.Slice(field.SubFields, func(i, j int) bool {
				return field.SubFields[i].Name < field.SubFields[j].Name
			})
			}
		}

		fields = append(fields, field)
	}

	return fields
}

// isAggregatable determines if an ES field type supports aggregation.
func isAggregatable(esType string, propMap map[string]interface{}) bool {
	switch esType {
	case "keyword", "numeric", "long", "integer", "short", "byte",
		"double", "float", "half_float", "scaled_float",
		"date", "boolean", "ip", "geo_point", "geo_shape":
		return true
	case "text":
		// text is aggregatable only if fielddata=true
		if fd, ok := propMap["fielddata"]; ok {
			return fd == true
		}
		return false
	default:
		return false
	}
}

// getStrVal safely extracts a string value from a map[string]interface{}.
func getStrVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

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

// entDatasourceToModel converts an ent DataSource entity to a model.DataSource.
// Does NOT include sensitive fields (PasswordEncrypted, ESApiKey).
func entDatasourceToModel(d *ent.DataSource) model.DataSource {
	return model.DataSource{
		ID:             int64(d.ID),
		Name:           d.Name,
		Type:           d.Type,
		Host:           d.Host,
		Port:           d.Port,
		Username:       d.Username,
		Database:       d.Database,
		SSLMode:        d.Sslmode,
		SchemaName:     d.SchemaName,
		MaxOpen:        d.MaxOpen,
		MaxIdle:        d.MaxIdle,
		MaxLifetime:    d.MaxLifetime,
		MaxIdleTime:    d.MaxIdleTime,
		Status:         d.Status,
		ESUrls:         d.EsUrls,
		ESVersion:      d.EsVersion,
		ESAuthType:     d.EsAuthType,
		ESIndexPattern: d.EsIndexPattern,
		ESVerifyCerts:  d.EsVerifyCerts,
		ExtraConfig:    d.ExtraConfig,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}
