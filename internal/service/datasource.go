package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
)

var (
	ErrDatasourceNotFound  = errors.New("数据源不存在")
	ErrDatasourceNameExists = errors.New("数据源名称已存在")
	ErrDatasourceDisabled  = errors.New("数据源已禁用")
	ErrInvalidDatasourceType = errors.New("数据源类型必须是 mysql 或 mongodb")
)

var ValidDatasourceTypes = map[string]bool{"mysql": true, "mongodb": true}

// DatasourceService handles datasource management logic.
type DatasourceService struct {
	db            *sql.DB
	encryptionKey string
	connMgr       *connpool.Manager
}

// NewDatasourceService creates a new DatasourceService.
func NewDatasourceService(db *sql.DB, encryptionKey string, connMgr *connpool.Manager) *DatasourceService {
	return &DatasourceService{db: db, encryptionKey: encryptionKey, connMgr: connMgr}
}

// CreateDataSource creates a new datasource with encrypted password.
func (s *DatasourceService) CreateDataSource(ctx context.Context, ds *model.DataSource) error {
	if !ValidDatasourceTypes[ds.Type] {
		return ErrInvalidDatasourceType
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

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, database, max_open, max_idle, max_lifetime, max_idle_time, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ds.Name, ds.Type, ds.Host, ds.Port, ds.Username, encrypted, ds.Database,
		ds.MaxOpen, ds.MaxIdle, ds.MaxLifetime, ds.MaxIdleTime, ds.Status,
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
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, type, host, port, username, database, max_open, max_idle, max_lifetime, max_idle_time, status, created_at, updated_at
		 FROM datasources ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("query datasources: %w", err)
	}
	defer rows.Close()

	var list []model.DataSource
	for rows.Next() {
		var ds model.DataSource
		if err := rows.Scan(&ds.ID, &ds.Name, &ds.Type, &ds.Host, &ds.Port, &ds.Username, &ds.Database,
			&ds.MaxOpen, &ds.MaxIdle, &ds.MaxLifetime, &ds.MaxIdleTime, &ds.Status, &ds.CreatedAt, &ds.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan datasource: %w", err)
		}
		list = append(list, ds)
	}
	return list, rows.Err()
}

// GetDataSource returns a single datasource by ID (password not decrypted).
func (s *DatasourceService) GetDataSource(ctx context.Context, id int64) (*model.DataSource, error) {
	ds := &model.DataSource{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, type, host, port, username, password_encrypted, database, max_open, max_idle, max_lifetime, max_idle_time, status, created_at, updated_at
		 FROM datasources WHERE id = ?`, id,
	).Scan(&ds.ID, &ds.Name, &ds.Type, &ds.Host, &ds.Port, &ds.Username, &ds.PasswordEncrypted, &ds.Database,
		&ds.MaxOpen, &ds.MaxIdle, &ds.MaxLifetime, &ds.MaxIdleTime, &ds.Status, &ds.CreatedAt, &ds.UpdatedAt)
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

	result, err := s.db.ExecContext(ctx,
		`UPDATE datasources SET name=?, type=?, host=?, port=?, username=?, password_encrypted=?, database=?,
		 max_open=?, max_idle=?, max_lifetime=?, max_idle_time=?, updated_at=datetime('now') WHERE id=?`,
		ds.Name, ds.Type, ds.Host, ds.Port, ds.Username, encrypted, ds.Database,
		ds.MaxOpen, ds.MaxIdle, ds.MaxLifetime, ds.MaxIdleTime, id,
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
		// Also remove pool for old config in case host/port/database changed
		if existing.Host != ds.Host || existing.Port != ds.Port || existing.Database != ds.Database {
			s.connMgr.Remove(id, existing.Host, existing.Port, existing.Database)
		}
	}
	if ds.Type == "mongodb" || existing.Type == "mongodb" {
		s.connMgr.RemoveMongo(id)
	}

	return nil
}

// DisableDataSource marks a datasource as disabled.
func (s *DatasourceService) DisableDataSource(ctx context.Context, id int64) error {
	// Get existing datasource for pool cleanup
	existing, err := s.GetDataSource(ctx, id)
	if err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx,
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
	if existing.Type == "mongodb" {
		s.connMgr.RemoveMongo(id)
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
	case "mongodb":
		uri := buildMongoURI(ds.Host, ds.Port, ds.Username, password)
		return connpool.MongoPing(ctx, uri)
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
		defer rows.Close()

		tables := make([]string, 0)
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, fmt.Errorf("scan table name: %w", err)
			}
			tables = append(tables, name)
		}
		return tables, rows.Err()
	case "mongodb":
		uri := buildMongoURI(ds.Host, ds.Port, ds.Username, password)
		return s.connMgr.GetMongoDatabaseNames(ctx, id, uri)
	default:
		return nil, ErrInvalidDatasourceType
	}
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

func buildMongoURI(host string, port int, user, password string) string {
	if user != "" && password != "" {
		return "mongodb://" + user + ":" + password + "@" + host + ":" + strconv.Itoa(port)
	}
	return "mongodb://" + host + ":" + strconv.Itoa(port)
}
