package model

import "time"

// User represents a user in the system.
type User struct {
	ID              int64     `json:"id"`
	Username        string    `json:"username"`
	PasswordHash    string    `json:"-"`
	Role            string    `json:"role"`
	DingTalkUserID  string    `json:"dingtalk_user_id,omitempty"`
	DingTalkUnionID string    `json:"dingtalk_union_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// QueryHistory represents a user's query execution history record.
type QueryHistory struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	DatasourceID  int64     `json:"datasource_id"`
	Database      string    `json:"database"`
	SQLContent    string    `json:"sql_content"`
	SQLSummary    string    `json:"sql_summary"`
	DBType        string    `json:"db_type"`
	ExecutionTime int64     `json:"execution_time"` // ms
	ResultRows    int64     `json:"result_rows"`
	AffectedRows  int64     `json:"affected_rows"`
	CreatedAt     time.Time `json:"created_at"`
}

// TicketStatus represents the status of a ticket in the workflow.
type TicketStatus string

const (
	TicketStatusSubmitted       TicketStatus = "SUBMITTED"
	TicketStatusAIReviewed      TicketStatus = "AI_REVIEWED"
	TicketStatusPendingApproval TicketStatus = "PENDING_APPROVAL"
	TicketStatusApproved        TicketStatus = "APPROVED"
	TicketStatusScheduled       TicketStatus = "SCHEDULED"
	TicketStatusExecuting       TicketStatus = "EXECUTING"
	TicketStatusDone            TicketStatus = "DONE"
	TicketStatusRejected        TicketStatus = "REJECTED"
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
	ScheduledAt    *time.Time   `json:"scheduled_at,omitempty"`
	ExecutedAt     *time.Time   `json:"executed_at,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// MaskRule represents a field-level masking rule.
type MaskRule struct {
	ID             int64     `json:"id"`
	DatasourceID   int64     `json:"datasource_id"`
	Database       string    `json:"database"`
	TableName      string    `json:"table_name"`
	Field          string    `json:"field"`
	MaskType       string    `json:"mask_type"`
	CustomRegex    string    `json:"custom_regex,omitempty"`
	CustomTemplate string    `json:"custom_template,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SensitiveTable represents a table marked as containing sensitive data.
type SensitiveTable struct {
	ID               int64     `json:"id"`
	DatasourceID     int64     `json:"datasource_id"`
	Database         string    `json:"database"`
	TableName        string    `json:"table_name"`
	SensitivityLevel string    `json:"sensitivity_level"` // low, medium, high
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RefreshToken represents a stored refresh token for token rotation.
type RefreshToken struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Token     string    `json:"-"` // hashed token, never exposed
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
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
	AIReviewResult     string    `json:"ai_review_result,omitempty"`
	TicketID           int64     `json:"ticket_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// Comment represents a discussion comment on a ticket.
type Comment struct {
	ID        int64     `json:"id"`
	OrderID   int64     `json:"order_id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username,omitempty"`
	Content   string    `json:"content"`
	ParentID  int64     `json:"parent_id"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditLogSearch represents an audit log entry returned by FTS5 full-text search.
type AuditLogSearch struct {
	AuditLog
	HighlightSQLContent string  `json:"highlight_sql_content,omitempty"`
	HighlightSQLSummary string  `json:"highlight_sql_summary,omitempty"`
	Rank                float64 `json:"rank,omitempty"`
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
	SSLMode           string    `json:"sslmode,omitempty"`   // PostgreSQL SSL mode: disable, prefer, require, verify-ca, verify-full
	SchemaName        string    `json:"schema_name,omitempty"` // PostgreSQL schema (default: public)
	MaxOpen           int       `json:"max_open"`
	MaxIdle           int       `json:"max_idle"`
	MaxLifetime       int       `json:"max_lifetime"`
	MaxIdleTime       int       `json:"max_idle_time"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
