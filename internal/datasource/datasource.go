package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"

	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/db/ent/auditlog"
	"github.com/whg517/sqlflow/internal/db/ent/casbinrule"
	"github.com/whg517/sqlflow/internal/db/ent/datasource"
	"github.com/whg517/sqlflow/internal/db/ent/maskrule"
	"github.com/whg517/sqlflow/internal/db/ent/permissionrequest"
	"github.com/whg517/sqlflow/internal/db/ent/queryhistory"
	"github.com/whg517/sqlflow/internal/db/ent/sensitivetable"
	"github.com/whg517/sqlflow/internal/db/ent/temppolicy"
	"github.com/whg517/sqlflow/internal/db/ent/ticket"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/crypto"
)

var (
	ErrDatasourceType       = errors.New("不支持的数据源类型")
	ErrDatasourceNotFound   = errors.New("数据源不存在")
	ErrDatasourceNameExists = errors.New("数据源名称已存在")
	ErrDatasourceDisabled   = errors.New("数据源已禁用")
	// ErrInvalidDatasourceType names the registered types rather than a
	// hardcoded list. There used to be two copies of that sentence, so adding a
	// driver meant remembering both — and forgetting left the user a list that
	// omitted a type the server would accept.
	ErrInvalidDatasourceType = fmt.Errorf("数据源类型必须是 %s 之一", strings.Join(driver.SupportedTypes(), "、"))
	ErrSystemDatasource      = errors.New("系统数据源受保护，不能执行此操作")
	ErrDatasourceMustDisable = errors.New("请先禁用数据源，再执行删除")
	// ErrMetadataNotSupported is returned when a driver does not declare
	// CapMetadata, so table/column discovery has no meaning for it.
	ErrMetadataNotSupported = errors.New("该数据源不支持元数据浏览")
)

// IsValidDatasourceType reports whether a driver is registered for the type.
//
// It replaces a hand-written map that was consulted at five entry points while
// SQL templates and connection testing asked the registry. A driver present in
// one and missing from the other produced a datasource nobody could create and
// a template anybody could, with nothing to report the disagreement.
func IsValidDatasourceType(typeName string) bool {
	return driver.IsRegistered(typeName)
}

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

// IsInternal reports whether a datasource exposes SQLFlow's own
// metadata database. These datasources are restricted to administrators.
//
// The marker is the system flag this package writes into extra_config, not the
// driver type. Requiring Type == "sqlite" here was correct only while the
// platform stored its metadata in SQLite; after ADR-0009 moved it to
// PostgreSQL the condition became permanently false and the administrator gate
// it guards silently opened. Which engine the platform happens to use is not
// what makes a datasource internal.
func IsInternal(ds *model.DataSource) bool {
	if ds == nil || ds.ExtraConfig == "" {
		return false
	}
	var extra struct {
		System bool `json:"system"`
	}
	return json.Unmarshal([]byte(ds.ExtraConfig), &extra) == nil && extra.System
}

// Service handles datasource management logic.
type Service struct {
	database      *db.DB
	client        *ent.Client
	encryptionKey string
	connMgr       *connpool.Manager
	poolMgr       *driver.PoolManager
}

// NewService creates a new Service.
func NewService(database *db.DB, encryptionKey string, connMgr *connpool.Manager, poolMgr *driver.PoolManager) *Service {
	return &Service{database: database, client: database.Client(), encryptionKey: encryptionKey, connMgr: connMgr, poolMgr: poolMgr}
}

// PoolManager returns the driver PoolManager (may be nil if not configured).
func (s *Service) PoolManager() *driver.PoolManager {
	return s.poolMgr
}

// validateDriverConfig runs the driver's own configuration rules.
//
// It is deliberately connection-free: this runs before a datasource is saved,
// when the target may legitimately be unreachable. Credentials are not needed
// either — the rules cover shape and transport, not authentication.
func (s *Service) validateDriverConfig(ds *model.DataSource) error {
	cfg, err := driver.BuildConfigFromDataSource(NewAdapter(ds), driver.Secrets{})
	if err != nil {
		return err
	}
	return driver.ValidateConfigFor(ds.Type, cfg)
}

// CreateDataSource creates a new datasource with encrypted password.
func (s *Service) CreateDataSource(ctx context.Context, ds *model.DataSource) error {
	if !IsValidDatasourceType(ds.Type) {
		return ErrInvalidDatasourceType
	}

	// Each driver validates its own configuration: what its fields require is
	// its business, and a branch per type here would go stale on every change.
	if err := s.validateDriverConfig(ds); err != nil {
		return err
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

// internalDatasourceType is the driver the platform's own store speaks.
//
// It follows the platform database, not the other way round: since ADR-0009 the
// platform stores its metadata in PostgreSQL, so browsing that metadata is a
// PostgreSQL connection. Note this is datasource *configuration* — the fact that
// its value happens to match the platform's storage choice is a consequence, not
// a coupling between the two modules.
const internalDatasourceType = "postgresql"

// EnsureInternalDataSource registers SQLFlow's own database as a read-only
// datasource, so operators can query platform metadata through the workbench.
//
// dsn is the platform's own connection string. The operation is idempotent
// across restarts, and re-points the datasource when the DSN changes.
func (s *Service) EnsureInternalDataSource(ctx context.Context, dsn string) (*model.DataSource, error) {
	const name = "SQLFlow 元数据库"

	parsed, err := parseInternalDSN(dsn)
	if err != nil {
		return nil, err
	}

	existing, err := s.client.DataSource.Query().
		Where(datasource.NameEQ(name)).
		Only(ctx)
	if err == nil {
		if existing.Type != internalDatasourceType || existing.ExtraConfig != internalDatasourceExtraConfig {
			return nil, fmt.Errorf("datasource name %q is already used by a non-system datasource", name)
		}
		if existing.Database != parsed.Database || existing.Host != parsed.Host || existing.Port != parsed.Port || existing.Status != "active" {
			existing, err = s.client.DataSource.UpdateOneID(existing.ID).
				SetDatabase(parsed.Database).
				SetHost(parsed.Host).
				SetPort(parsed.Port).
				SetUsername(parsed.Username).
				SetStatus("active").
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

	if err := s.CreateDataSource(ctx, parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// parseInternalDSN turns the platform's connection string into datasource
// fields.
//
// CreateDataSource encrypts PasswordEncrypted on the way in, so the plaintext
// password is handed over as-is here rather than being stored directly.
func parseInternalDSN(dsn string) (*model.DataSource, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse platform dsn: %w", err)
	}
	port := 5432
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	password, _ := u.User.Password()
	return &model.DataSource{
		Name:              "SQLFlow 元数据库",
		Type:              internalDatasourceType,
		Host:              u.Hostname(),
		Port:              port,
		Username:          u.User.Username(),
		PasswordEncrypted: password,
		Database:          strings.TrimPrefix(u.Path, "/"),
		SSLMode:           u.Query().Get("sslmode"),
		MaxOpen:           2,
		MaxIdle:           1,
		Status:            "active",
		ExtraConfig:       internalDatasourceExtraConfig,
	}, nil
}

// ListDataSources returns all datasources without encrypted passwords.
func (s *Service) ListDataSources(ctx context.Context) ([]model.DataSource, error) {
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
func (s *Service) ListAvailableDataSources(ctx context.Context) ([]model.DataSource, error) {
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

// DescribeDataSource returns the driver capability profile for a datasource.
//
// The profile is derived from the registered driver for the datasource's type
// and needs no connection. It describes what the driver can do, never what the
// caller may do — every operation still goes through its own authorization.
func (s *Service) DescribeDataSource(ctx context.Context, id int64) (*driver.Descriptor, error) {
	ds, err := s.GetDataSource(ctx, id)
	if err != nil {
		return nil, err
	}
	desc, err := driver.DescribeType(ds.Type)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrDatasourceType, ds.Type)
	}
	return desc, nil
}

// GetDataSource returns a single datasource by ID (password not decrypted).
func (s *Service) GetDataSource(ctx context.Context, id int64) (*model.DataSource, error) {
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
func (s *Service) UpdateDataSource(ctx context.Context, id int64, ds *model.DataSource) error {
	if !IsValidDatasourceType(ds.Type) {
		return ErrInvalidDatasourceType
	}

	// Each driver validates its own configuration: what its fields require is
	// its business, and a branch per type here would go stale on every change.
	if err := s.validateDriverConfig(ds); err != nil {
		return err
	}

	// Get existing datasource for pool invalidation
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}
	if IsInternal(existing) {
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

	// Invalidate the cached connection since the configuration may have changed.
	// PoolManager keys by datasource ID alone, so one call covers every type —
	// including a type change, which the old per-type branches had to special
	// case by checking both the old and the new type.
	s.removeDatasourcePool(id)

	return nil
}
func (s *Service) DisableDataSource(ctx context.Context, id int64) error {
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}
	if IsInternal(existing) {
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

	s.removeDatasourcePool(id)
	return nil
}

func (s *Service) EnableDataSource(ctx context.Context, id int64) error {
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}
	if IsInternal(existing) {
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

func (s *Service) DeleteDataSource(ctx context.Context, id int64) error {
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}
	if IsInternal(existing) {
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
	s.removeDatasourcePool(id)
	return nil
}

func (s *Service) datasourceDependencies(ctx context.Context, id int64) ([]DatasourceDependency, error) {
	domain := authz.DatasourceDomain(id)

	// Each entry counts the rows that would be orphaned by deleting the
	// datasource. The label is what the user is shown, so the order here is the
	// order the refusal lists them in.
	checks := []struct {
		label string
		count func(context.Context) (int, error)
	}{
		{"工单", func(ctx context.Context) (int, error) {
			return s.client.Ticket.Query().Where(ticket.DatasourceID(id)).Count(ctx)
		}},
		{"查询历史", func(ctx context.Context) (int, error) {
			return s.client.QueryHistory.Query().Where(queryhistory.DatasourceID(id)).Count(ctx)
		}},
		{"审计日志", func(ctx context.Context) (int, error) {
			return s.client.AuditLog.Query().Where(auditlog.DatasourceID(id)).Count(ctx)
		}},
		{"脱敏规则", func(ctx context.Context) (int, error) {
			return s.client.MaskRule.Query().Where(maskrule.DatasourceID(id)).Count(ctx)
		}},
		{"敏感表", func(ctx context.Context) (int, error) {
			return s.client.SensitiveTable.Query().Where(sensitivetable.DatasourceID(id)).Count(ctx)
		}},
		{"临时权限申请", func(ctx context.Context) (int, error) {
			return s.client.PermissionRequest.Query().Where(permissionrequest.DatasourceID(id)).Count(ctx)
		}},
		{"权限策略", func(ctx context.Context) (int, error) {
			return s.client.CasbinRule.Query().Where(
				casbinrule.Or(casbinrule.V1(domain), casbinrule.V2(domain)),
			).Count(ctx)
		}},
		{"临时授权", func(ctx context.Context) (int, error) {
			return s.client.TempPolicy.Query().Where(temppolicy.Dom(domain)).Count(ctx)
		}},
	}

	dependencies := make([]DatasourceDependency, 0)
	for _, check := range checks {
		count, err := check.count(ctx)
		if err != nil {
			return nil, fmt.Errorf("check datasource dependency %s: %w", check.label, err)
		}
		if count > 0 {
			dependencies = append(dependencies, DatasourceDependency{
				Label: check.label,
				Count: int64(count),
			})
		}
	}
	return dependencies, nil
}

// removeDatasourcePool drops every cached connection for a datasource.
//
// Both caches are keyed by datasource ID, so this needs to know nothing about
// the datasource's type or its connection fields — which is also why it has to
// clear both. It used to clear only the driver pool; the Elasticsearch client
// behind index and field browsing lives in connpool, and RemoveElasticsearch
// had no caller outside its own test, so rotating an ES password left that
// client authenticating with the old one until the process restarted.
//
// The ES cache key is datasource id plus URLs and deliberately holds no
// credentials. Evicting on write is what keeps it correct; putting secrets into
// map keys would not.
func (s *Service) removeDatasourcePool(id int64) {
	if s.poolMgr != nil {
		s.poolMgr.Remove(id)
	}
	if s.connMgr != nil {
		s.connMgr.RemoveElasticsearch(id)
	}
}

// datasourceSecrets decrypts the credentials stored for a datasource.
func (s *Service) datasourceSecrets(ds *model.DataSource) (driver.Secrets, error) {
	return DecryptSecrets(ds, s.encryptionKey)
}

// TestConnection attempts to connect to the datasource using the Driver abstraction.
func (s *Service) TestConnection(ctx context.Context, ds *model.DataSource) error {
	password := ds.PasswordEncrypted
	esAPIKey := ds.ESApiKey

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

	// Connection testing always goes through the driver: it builds a throwaway
	// instance from the registry and never touches the pool, so there is nothing
	// for a legacy path to fall back to.
	// Resolve the driver first: an unregistered type is a datasource-level
	// error, not a configuration-building failure.
	d, err := driver.NewDriver(ds.Type)
	if err != nil {
		return ErrInvalidDatasourceType
	}

	// The candidate carries the edited configuration; the secrets resolved
	// above are already plaintext, whether they came from the form or from the
	// stored row.
	adapter := NewAdapter(ds)
	cfg, err := driver.BuildConfigFromDataSource(adapter, driver.Secrets{Password: password, APIKey: esAPIKey})
	if err != nil {
		return err
	}
	// Configuration rules first: an unreachable host and a malformed
	// configuration deserve different messages, and only the second is
	// something the user can fix from the form.
	if validator, ok := d.(driver.ConfigValidator); ok {
		if err := validator.ValidateConfig(cfg); err != nil {
			return err
		}
	}
	if err := d.Connect(ctx, cfg); err != nil {
		return err
	}
	defer d.Close()
	return d.Ping(ctx)
}

// metadataDriver returns a connected driver for metadata reads.
//
// It reports ErrMetadataNotSupported for drivers that do not declare
// CapMetadata, so an unsupported source gets an explicit answer instead of
// being silently routed somewhere that happens to compile.
func (s *Service) metadataDriver(ctx context.Context, ds *model.DataSource, secrets driver.Secrets) (driver.Driver, error) {
	// An unregistered type is invalid regardless of how the service is wired,
	// so it is reported before anything that depends on the pool.
	if !driver.IsRegistered(ds.Type) {
		return nil, ErrInvalidDatasourceType
	}
	if s.poolMgr == nil {
		return nil, ErrMetadataNotSupported
	}
	adapter := NewAdapter(ds)
	cfg, err := driver.BuildConfigFromDataSource(adapter, secrets)
	if err != nil {
		return nil, err
	}
	d, err := s.poolMgr.Get(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", ds.Type, err)
	}
	if !d.Capabilities().Has(driver.CapMetadata) {
		return nil, ErrMetadataNotSupported
	}
	return d, nil
}

// GetTables returns table names for a datasource.
func (s *Service) GetTables(ctx context.Context, id int64) ([]string, error) {
	ds, err := s.GetDataSource(ctx, id)
	if err != nil {
		return nil, err
	}

	if ds.Status == "disabled" {
		return nil, ErrDatasourceDisabled
	}

	secrets, err := s.datasourceSecrets(ds)
	if err != nil {
		return nil, err
	}

	// Route by declared capability. A hand-kept type list here used to exclude
	// MongoDB and Elasticsearch even though both declare CapMetadata, and it
	// substituted a default database name that made MySQL list the server's own
	// catalog. What an empty scope means belongs to the driver.
	d, err := s.metadataDriver(ctx, ds, secrets)
	if err != nil {
		return nil, err
	}
	tables, err := d.ListTables(ctx, ds.Database)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tables))
	for _, t := range tables {
		names = append(names, t.Name)
	}
	return names, nil
}

// ColumnInfo represents a table column's metadata.
type ColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment"`
}

// GetTableColumns returns column information for a specific table in a datasource.
func (s *Service) GetTableColumns(ctx context.Context, id int64, tableName string) ([]ColumnInfo, error) {
	ds, err := s.GetDataSource(ctx, id)
	if err != nil {
		return nil, err
	}

	if ds.Status == "disabled" {
		return nil, ErrDatasourceDisabled
	}

	secrets, err := s.datasourceSecrets(ds)
	if err != nil {
		return nil, err
	}

	d, err := s.metadataDriver(ctx, ds, secrets)
	if err != nil {
		return nil, err
	}
	columns, err := d.GetColumns(ctx, ds.Database, tableName)
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

// GetDataSourceSafe returns a datasource without the encrypted password for API responses.
func (s *Service) GetDataSourceSafe(ctx context.Context, id int64) (*model.DataSource, error) {
	ds, err := s.GetDataSource(ctx, id)
	if err != nil {
		return nil, err
	}
	ds.PasswordEncrypted = ""
	return ds, nil
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
func (s *Service) getESClient(ctx context.Context, id int64) (*model.DataSource, string, *es.Client, error) {
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

	// The only remaining connpool user. Index and field browsing need the raw
	// Elasticsearch client for paginated _cat/indices and mapping calls, which
	// the Driver interface does not model — ListTables reports index names but
	// not doc counts, sizes or pagination. Extending the driver contract is the
	// prerequisite for removing connpool entirely.
	client, err := s.connMgr.GetElasticsearch(ctx, id, urls, ds.ESAuthType, ds.Username, password, esAPIKey, ds.ESVerifyCerts)
	if err != nil {
		return nil, "", nil, fmt.Errorf("连接 Elasticsearch 失败: %w", err)
	}
	return ds, password, client, nil
}

// GetESIndices returns the list of indices in the Elasticsearch cluster.
// Supports keyword filtering and pagination.
func (s *Service) GetESIndices(ctx context.Context, id int64, query string, page, pageSize int) ([]ESIndexInfo, int, error) {
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
func (s *Service) GetESIndexFields(ctx context.Context, id int64, indexName string) ([]ESIndexField, error) {
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
