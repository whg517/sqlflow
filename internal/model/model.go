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

// TicketStatus represents the status of a ticket in the workflow.
type TicketStatus string

const (
	TicketStatusSubmitted       TicketStatus = "SUBMITTED"
	TicketStatusAIReviewed      TicketStatus = "AI_REVIEWED"
	TicketStatusPendingApproval TicketStatus = "PENDING_APPROVAL"
	TicketStatusApproved        TicketStatus = "APPROVED"
	TicketStatusRejected        TicketStatus = "REJECTED"
	TicketStatusExecuting       TicketStatus = "EXECUTING"
	TicketStatusDone            TicketStatus = "DONE"
	TicketStatusCancelled       TicketStatus = "CANCELLED"
)

// Ticket represents a change ticket (DDL/DML) in the system.
type Ticket struct {
	ID             int64        `json:"id"`
	SubmitterID    int64        `json:"submitter_id"`
	SubmitterName  string       `json:"submitter_name,omitempty"`
	DatasourceID   int64        `json:"datasource_id"`
	Database       string       `json:"database"`
	SQLContent     string       `json:"sql_content"`
	SQLSummary     string       `json:"sql_summary"`
	DBType         string       `json:"db_type"`
	ChangeReason   string       `json:"change_reason"`
	Status         TicketStatus `json:"status"`
	RiskLevel      string       `json:"risk_level"`
	AIReviewResult string       `json:"ai_review_result,omitempty"`
	ReviewerID     int64        `json:"reviewer_id"`
	ReviewerName   string       `json:"reviewer_name,omitempty"`
	ReviewComment  string       `json:"review_comment,omitempty"`
	ExecutedAt     *time.Time   `json:"executed_at,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// MaskRule represents a field-level masking rule.
type MaskRule struct {
	ID              int64     `json:"id"`
	DatasourceID    int64     `json:"datasource_id"`
	Database        string    `json:"database"`
	TableName       string    `json:"table_name"`
	Field           string    `json:"field"`
	MaskType        string    `json:"mask_type"`
	CustomRegex     string    `json:"custom_regex,omitempty"`
	CustomTemplate  string    `json:"custom_template,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SensitiveTable represents a table marked as containing sensitive data.
type SensitiveTable struct {
	ID              int64     `json:"id"`
	DatasourceID    int64     `json:"datasource_id"`
	Database        string    `json:"database"`
	TableName       string    `json:"table_name"`
	SensitivityLevel string   `json:"sensitivity_level"` // low, medium, high
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// AuditLog represents an audit log entry.
type AuditLog struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id"`
	Username           string    `json:"username,omitempty"`
	Action             string    `json:"action"`
	DatasourceID       int64     `json:"datasource_id"`
	Database           string    `json:"database"`
	SQLContent         string    `json:"sql_content"`
	SQLSummary         string    `json:"sql_summary"`
	ResultRows         int64     `json:"result_rows"`
	AffectedRows       int64     `json:"affected_rows"`
	ExecutionTimeMs    int64     `json:"execution_time_ms"`
	ErrorMessage       string    `json:"error_message,omitempty"`
	DesensitizedFields string    `json:"desensitized_fields,omitempty"`
	IPAddress          string    `json:"ip_address,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
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
