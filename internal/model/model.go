package model

import "time"

// User represents a user in the system.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// QueryHistory represents a user's query execution history record.
type QueryHistory struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	DatasourceID   int64     `json:"datasource_id"`
	Database       string    `json:"database"`
	SQLContent     string    `json:"sql_content"`
	SQLSummary     string    `json:"sql_summary"`
	DBType         string    `json:"db_type"`
	ExecutionTime  int64     `json:"execution_time"` // ms
	ResultRows     int64     `json:"result_rows"`
	AffectedRows   int64     `json:"affected_rows"`
	CreatedAt      time.Time `json:"created_at"`
}

// DataSource represents a registered database instance.
type DataSource struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	Type              string    `json:"type"`
	Host              string    `json:"host"`
	Port              int       `json:"port"`
	Username          string    `json:"username"`
	PasswordEncrypted string    `json:"-"`
	Database          string    `json:"database,omitempty"`
	MaxOpen           int       `json:"max_open"`
	MaxIdle           int       `json:"max_idle"`
	MaxLifetime       int       `json:"max_lifetime"`
	MaxIdleTime       int       `json:"max_idle_time"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
