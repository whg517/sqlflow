package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"

	"github.com/whg517/sqlflow/internal/authz"
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
	ErrInvalidDatasourceType = errors.New("数据源类型必须是 mysql、postgresql、sqlite、mongodb 或 elasticsearch")
	ErrSystemDatasource      = errors.New("系统数据源受保护，不能执行此操作")
	ErrDatasourceMustDisable = errors.New("请先禁用数据源，再执行删除")
)

var ValidDatasourceTypes = map[string]bool{"mysql": true, "postgresql": true, "sqlite": true, "mongodb": true, "elasticsearch": true}

const internalDatasourceExtraConfig = `{"system":true,"read_only":true}`

type DatasourceDependency struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type DatasourceInUseError struct {
	Dependencies []DatasourceDependency
}

func (e *DatasourceInUseError) Error() string {
	parts := make([]string, 0, len(e.Dependencies))
	for _, dependency := range e.Dependencies {
		parts = append(parts, fmt.Sprintf("%s %d 条", dependency.Label, dependency.Count))
	}
	return "数据源仍被以下内容引用：" + strings.Join(parts, "、")
}

// IsInternalDataSource reports whether a datasource exposes SQLFlow's own
// metadata database. These datasources are restricted to administrators.
func IsInternalDataSource(ds *model.DataSource) bool {
	if ds == nil || ds.Type != "sqlite" || ds.ExtraConfig == "" {
		return false
	}
	var extra struct {
		System bool `json:"system"`
	}
	return json.Unmarshal([]byte(ds.ExtraConfig), &extra) == nil && extra.System
}

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

// EnsureInternalDataSource registers SQLFlow's own SQLite database as a
// read-only datasource. The operation is idempotent across restarts.
func (s *DatasourceService) EnsureInternalDataSource(ctx context.Context, databasePath string) (*model.DataSource, error) {
	const name = "SQLFlow 元数据库"
	existing, err := s.client.DataSource.Query().
		Where(datasource.NameEQ(name)).
		Only(ctx)
	if err == nil {
		if existing.Type != "sqlite" || existing.ExtraConfig != internalDatasourceExtraConfig {
			return nil, fmt.Errorf("datasource name %q is already used by a non-system datasource", name)
		}
		if existing.Database != databasePath || existing.Status != "active" {
			existing, err = s.client.DataSource.UpdateOneID(existing.ID).
				SetDatabase(databasePath).
				SetHost("localhost").
				SetPort(0).
				SetStatus("active").
				SetMaxOpen(1).
				SetMaxIdle(1).
				Save(ctx)
			if err != nil {
				return nil, fmt.Errorf("update internal datasource: %w", err)
			}
			if s.poolMgr != nil {
				s.poolMgr.Remove(int64(existing.ID))
			}
		}
		result := entDatasourceToModel(existing)
		return &result, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("query internal datasource: %w", err)
	}

	ds := &model.DataSource{
		Name:        name,
		Type:        "sqlite",
		Host:        "localhost",
		Database:    databasePath,
		MaxOpen:     1,
		MaxIdle:     1,
		Status:      "active",
		ExtraConfig: internalDatasourceExtraConfig,
	}
	if err := s.CreateDataSource(ctx, ds); err != nil {
		return nil, err
	}
	return ds, nil
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

// ListAvailableDataSources returns the minimal discovery fields for active
// datasources. It intentionally does not return connection details or imply
// query authorization; downstream operations still enforce table/action
// permissions.
func (s *DatasourceService) ListAvailableDataSources(ctx context.Context) ([]model.DataSource, error) {
	results, err := s.client.DataSource.Query().
		Where(datasource.StatusEQ("active")).
		Select(
			datasource.FieldID,
			datasource.FieldName,
			datasource.FieldType,
			datasource.FieldStatus,
			datasource.FieldExtraConfig,
		).
		Order(datasource.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query available datasources: %w", err)
	}

	list := make([]model.DataSource, 0, len(results))
	for _, d := range results {
		list = append(list, model.DataSource{
			ID:          int64(d.ID),
			Name:        d.Name,
			Type:        d.Type,
			Status:      d.Status,
			ExtraConfig: d.ExtraConfig,
		})
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
	if IsInternalDataSource(existing) {
		return ErrSystemDatasource
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

	// Invalidate cached connection pool since config may have changed.
	// PoolManager.Remove 按 dsID 统一清理所有类型的连接（替代旧 connMgr 的按类型分支清理）。
	if s.poolMgr != nil {
		s.poolMgr.Remove(id)
	} else {
		// Legacy fallback (poolMgr 未注入时)
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
	}

	return nil
}
func (s *DatasourceService) DisableDataSource(ctx context.Context, id int64) error {
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}
	if IsInternalDataSource(existing) {
		return ErrSystemDatasource
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

	s.removeDatasourcePool(id, existing)
	return nil
}

func (s *DatasourceService) EnableDataSource(ctx context.Context, id int64) error {
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}
	if IsInternalDataSource(existing) {
		return ErrSystemDatasource
	}
	if existing.Status == "active" {
		return nil
	}
	if err := s.client.DataSource.UpdateOneID(int(id)).
		SetStatus("active").
		Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return ErrDatasourceNotFound
		}
		return fmt.Errorf("enable datasource: %w", err)
	}
	return nil
}

func (s *DatasourceService) DeleteDataSource(ctx context.Context, id int64) error {
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}
	if IsInternalDataSource(existing) {
		return ErrSystemDatasource
	}
	if existing.Status != "disabled" {
		return ErrDatasourceMustDisable
	}

	dependencies, err := s.datasourceDependencies(ctx, id)
	if err != nil {
		return err
	}
	if len(dependencies) > 0 {
		return &DatasourceInUseError{Dependencies: dependencies}
	}

	if err := s.client.DataSource.DeleteOneID(int(id)).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return ErrDatasourceNotFound
		}
		return fmt.Errorf("delete datasource: %w", err)
	}
	s.removeDatasourcePool(id, existing)
	return nil
}

func (s *DatasourceService) datasourceDependencies(ctx context.Context, id int64) ([]DatasourceDependency, error) {
	domain := authz.DatasourceDomain(id)
	checks := []struct {
		label string
		query string
		args  []interface{}
	}{
		{"工单", `SELECT COUNT(*) FROM tickets WHERE datasource_id = ?`, []interface{}{id}},
		{"查询历史", `SELECT COUNT(*) FROM query_history WHERE datasource_id = ?`, []interface{}{id}},
		{"审计日志", `SELECT COUNT(*) FROM audit_logs WHERE datasource_id = ?`, []interface{}{id}},
		{"脱敏规则", `SELECT COUNT(*) FROM mask_rules WHERE datasource_id = ?`, []interface{}{id}},
		{"敏感表", `SELECT COUNT(*) FROM sensitive_tables WHERE datasource_id = ?`, []interface{}{id}},
		{"临时权限申请", `SELECT COUNT(*) FROM permission_requests WHERE datasource_id = ?`, []interface{}{id}},
		{"权限策略", `SELECT COUNT(*) FROM casbin_rule WHERE v1 = ? OR v2 = ?`, []interface{}{domain, domain}},
		{"临时授权", `SELECT COUNT(*) FROM temp_policies WHERE dom = ?`, []interface{}{domain}},
	}

	dependencies := make([]DatasourceDependency, 0)
	for _, check := range checks {
		var count int64
		if err := s.database.DB.QueryRowContext(ctx, check.query, check.args...).Scan(&count); err != nil {
			return nil, fmt.Errorf("check datasource dependency %s: %w", check.label, err)
		}
		if count > 0 {
			dependencies = append(dependencies, DatasourceDependency{
				Label: check.label,
				Count: count,
			})
		}
	}
	return dependencies, nil
}

func (s *DatasourceService) removeDatasourcePool(id int64, existing *model.DataSource) {
	if s.poolMgr != nil {
		s.poolMgr.Remove(id)
	} else {
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
	}
}

// TestConnection attempts to connect to the datasource using the Driver abstraction.
func (s *DatasourceService) TestConnection(ctx context.Context, ds *model.DataSource) error {
	password := ds.PasswordEncrypted
	esAPIKey := ds.ESApiKey

	if ds.Type == "sqlite" {
		info, err := os.Stat(ds.Database)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("SQLite 文件不存在: %s", ds.Database)
			}
			return fmt.Errorf("无法访问 SQLite 文件: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("SQLite 地址必须指向数据库文件")
		}
	}

	// Candidate configurations used while editing carry the existing datasource
	// ID. Reuse stored secrets only when the user leaves those fields empty;
	// all other connection fields must come from the candidate configuration.
	if ds.ID > 0 {
		stored, err := s.GetDataSource(ctx, ds.ID)
		if err != nil {
			return err
		}
		if password == "" || password == stored.PasswordEncrypted {
			decrypted, err := crypto.Decrypt(stored.PasswordEncrypted, s.encryptionKey)
			if err != nil {
				return fmt.Errorf("decrypt password: %w", err)
			}
			password = decrypted
		}
		if ds.Type == "elasticsearch" && ds.ESAuthType == "api_key" &&
			(esAPIKey == "" || esAPIKey == stored.ESApiKey) {
			decrypted, err := crypto.Decrypt(stored.ESApiKey, s.encryptionKey)
			if err != nil {
				return fmt.Errorf("decrypt es_api_key: %w", err)
			}
			esAPIKey = decrypted
		}
	}

	// If poolMgr is available, use Driver abstraction for all types.
	// NOTE: This means the legacy connpool path below is dead code when poolMgr is set.
	// When poolMgr is fully adopted and connpool is removed, this comment and the
	// fallback block below should be deleted.
	if s.poolMgr != nil {
		candidate := *ds
		candidate.ESApiKey = esAPIKey
		adapter := newDataSourceAdapter(&candidate)
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

	// Use Driver abstraction if available.
	if s.poolMgr != nil {
		switch ds.Type {
		case "mysql", "postgresql", "sqlite":
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

	// Use Driver abstraction if available.
	if s.poolMgr != nil {
		switch ds.Type {
		case "mysql", "postgresql", "sqlite":
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
// validateESURLs requires HTTPS for public endpoints. Plain HTTP is allowed for
// localhost and private IP ranges, which are common for internal ES clusters.
func validateESURLs(raw string) error {
	urls := parseESUrls(raw)
	for _, rawURL := range urls {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Hostname() == "" {
			return fmt.Errorf("Elasticsearch 连接地址无效: %s", rawURL)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "https":
			continue
		case "http":
			host := strings.ToLower(parsed.Hostname())
			ip := net.ParseIP(host)
			if host == "localhost" || (ip != nil && (ip.IsPrivate() || ip.IsLoopback())) {
				continue
			}
			return fmt.Errorf("公网 Elasticsearch 连接地址必须使用 HTTPS，当前地址 %s 使用了 HTTP", rawURL)
		default:
			return fmt.Errorf("Elasticsearch 连接地址必须使用 HTTP 或 HTTPS: %s", rawURL)
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
	Health      string `json:"health"` // green, yellow, red
	Status      string `json:"status"` // open, closed
	DocCount    int64  `json:"doc_count"`
	StoreSize   string `json:"store_size"`   // human-readable, e.g. "4.2mb"
	StoreBytes  int64  `json:"store_bytes"`  // raw bytes
	CreatedTime string `json:"created_time"` // ISO 8601
}

// ESIndexField represents a field in an Elasticsearch index mapping.
type ESIndexField struct {
	Name         string         `json:"name"`
	ESType       string         `json:"es_type"` // text, keyword, date, long, boolean, nested, object, etc.
	Searchable   bool           `json:"searchable"`
	Aggregatable bool           `json:"aggregatable"`
	SubFields    []ESIndexField `json:"sub_fields,omitempty"` // nested/object children
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
							Name:         name + "." + subName,
							ESType:       getStrVal(sm, "type"),
							Searchable:   true,
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
