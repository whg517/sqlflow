package service

import (
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
}

// NewDatasourceService creates a new DatasourceService.
func NewDatasourceService(db *sql.DB, encryptionKey string) *DatasourceService {
	return &DatasourceService{db: db, encryptionKey: encryptionKey}
}

// CreateDataSource creates a new datasource with encrypted password.
func (s *DatasourceService) CreateDataSource(ds *model.DataSource) error {
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

	result, err := s.db.Exec(
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, database, max_open, max_idle, max_lifetime, max_idle_time, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ds.Name, ds.Type, ds.Host, ds.Port, ds.Username, encrypted, ds.Database,
		ds.MaxOpen, ds.MaxIdle, ds.MaxLifetime, ds.MaxIdleTime, ds.Status,
	)
	if err != nil {
		return fmt.Errorf("insert datasource: %w", err)
	}

	id, _ := result.LastInsertId()
	created, err := s.GetDataSource(id)
	if err != nil {
		return err
	}
	*ds = *created
	return nil
}

// ListDataSources returns all datasources without encrypted passwords.
func (s *DatasourceService) ListDataSources() ([]model.DataSource, error) {
	rows, err := s.db.Query(
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
func (s *DatasourceService) GetDataSource(id int64) (*model.DataSource, error) {
	ds := &model.DataSource{}
	err := s.db.QueryRow(
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
func (s *DatasourceService) UpdateDataSource(id int64, ds *model.DataSource) error {
	if !ValidDatasourceTypes[ds.Type] {
		return ErrInvalidDatasourceType
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
		existing, err := s.GetDataSource(id)
		if err != nil {
			return err
		}
		encrypted = existing.PasswordEncrypted
	}

	result, err := s.db.Exec(
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
	return nil
}

// DisableDataSource marks a datasource as disabled.
func (s *DatasourceService) DisableDataSource(id int64) error {
	result, err := s.db.Exec(
		`UPDATE datasources SET status='disabled', updated_at=datetime('now') WHERE id=?`, id,
	)
	if err != nil {
		return fmt.Errorf("disable datasource: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrDatasourceNotFound
	}
	return nil
}

// TestConnection attempts to connect to the datasource.
func (s *DatasourceService) TestConnection(ds *model.DataSource) error {
	password := ds.PasswordEncrypted

	// If the datasource has an ID, try to decrypt the stored password
	if ds.ID > 0 {
		stored, err := s.GetDataSource(ds.ID)
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
		return connpool.MySQLPing(ds.Host, ds.Port, ds.Username, password)
	case "mongodb":
		uri := buildMongoURI(ds.Host, ds.Port, ds.Username, password)
		return connpool.MongoPing(uri)
	default:
		return ErrInvalidDatasourceType
	}
}

// GetTables returns table names for a MySQL datasource or database names for MongoDB.
func (s *DatasourceService) GetTables(id int64) ([]string, error) {
	ds, err := s.GetDataSource(id)
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
		return connpool.MySQLGetTables(ds.Host, ds.Port, ds.Username, password, ds.Database)
	case "mongodb":
		uri := buildMongoURI(ds.Host, ds.Port, ds.Username, password)
		return connpool.MongoGetDatabases(uri)
	default:
		return nil, ErrInvalidDatasourceType
	}
}

// GetDataSourceSafe returns a datasource without the encrypted password for API responses.
func (s *DatasourceService) GetDataSourceSafe(id int64) (*model.DataSource, error) {
	ds, err := s.GetDataSource(id)
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
