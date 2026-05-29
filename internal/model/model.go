package model

import "time"

// ExportTaskStatus represents the status of an async export task.
type ExportTaskStatus string

const (
	ExportTaskStatusPending    ExportTaskStatus = "pending"
	ExportTaskStatusProcessing ExportTaskStatus = "processing"
	ExportTaskStatusCompleted  ExportTaskStatus = "completed"
	ExportTaskStatusFailed     ExportTaskStatus = "failed"
)

// ExportTask represents an asynchronous export job.
type ExportTask struct {
	ID          int64            `json:"id"`
	UserID      int64            `json:"user_id"`
	Username    string           `json:"username"`
	ExportType  string           `json:"export_type"` // "audit" or "ticket"
	Status      ExportTaskStatus `json:"status"`
	Filename    string           `json:"filename"`
	FilePath    string           `json:"-"` // server-local file path, never exposed
	TotalRows   int64            `json:"total_rows"`
	FileBytes   int64            `json:"file_bytes"`
	FiltersJSON string           `json:"filters_json"`
	ErrorMsg    string           `json:"error_msg,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
}

// ExportTaskSlim is a lightweight version returned in list APIs (no server path).
type ExportTaskSlim struct {
	ID          int64            `json:"id"`
	ExportType  string           `json:"export_type"`
	Status      ExportTaskStatus `json:"status"`
	Filename    string           `json:"filename"`
	TotalRows   int64            `json:"total_rows"`
	FileBytes   int64            `json:"file_bytes"`
	ErrorMsg    string           `json:"error_msg,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
}

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
	SLADeadline    *time.Time   `json:"sla_deadline,omitempty"`
	SLAStatus      string       `json:"sla_status,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	GitLinks       []GitLink    `json:"git_links,omitempty"`
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
	SSLMode           string    `json:"sslmode,omitempty"`     // PostgreSQL SSL mode: disable, prefer, require, verify-ca, verify-full
	SchemaName        string    `json:"schema_name,omitempty"` // PostgreSQL schema (default: public)
	MaxOpen           int       `json:"max_open"`
	MaxIdle           int       `json:"max_idle"`
	MaxLifetime       int       `json:"max_lifetime"`
	MaxIdleTime       int       `json:"max_idle_time"`
	Status            string    `json:"status"`
	// Elasticsearch 特有字段
	ESUrls         string `json:"es_urls,omitempty"`          // ES 节点地址，逗号分隔
	ESVersion      string `json:"es_version,omitempty"`       // ES 版本，如 "8.x"
	ESAuthType     string `json:"es_auth_type,omitempty"`     // 认证方式: basic/api_key/none
	ESApiKey       string `json:"-"`                           // API Key 加密存储
	ESIndexPattern string `json:"es_index_pattern,omitempty"` // 默认索引模式
	ESVerifyCerts  bool   `json:"es_verify_certs,omitempty"`   // 是否验证证书
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// GitLinkType represents the type of a git link (commit or PR).
type GitLinkType string

const (
	GitLinkTypeCommit GitLinkType = "commit"
	GitLinkTypePR     GitLinkType = "pr"
)

// GitLink represents an association between a ticket/audit log and a git commit or PR.
type GitLink struct {
	ID          int64        `json:"id"`
	EntityType  string       `json:"entity_type"` // "ticket" or "audit_log"
	EntityID    int64        `json:"entity_id"`
	LinkType    GitLinkType  `json:"link_type"` // "commit" or "pr"
	CommitHash  string       `json:"commit_hash"`
	CommitMsg   string       `json:"commit_message"`
	AuthorName  string       `json:"author_name"`
	AuthorEmail string       `json:"author_email,omitempty"`
	PRNumber    int          `json:"pr_number,omitempty"`
	PRTitle     string       `json:"pr_title,omitempty"`
	PRURL       string       `json:"pr_url,omitempty"`
	RepoURL     string       `json:"repo_url,omitempty"`
	Branch      string       `json:"branch,omitempty"`
	CreatedBy   int64        `json:"created_by"`
	CreatedAt   time.Time    `json:"created_at"`
}

// APIToken represents a personal API token for external integrations.
type APIToken struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Username    string     `json:"username,omitempty"`
	Name        string     `json:"name"`
	TokenHash   string     `json:"-"`
	TokenPrefix string     `json:"token_prefix"`
	Scopes      string     `json:"scopes"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	UseCount    int64      `json:"use_count"`
	IsActive    bool       `json:"is_active"`
	Description string     `json:"description,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// SLAConfig defines the SLA timeout rule for a priority level.
type SLAConfig struct {
	ID              int64     `json:"id"`
	Priority        string    `json:"priority"`
	TimeoutMinutes  int       `json:"timeout_minutes"`
	ReminderPercent int       `json:"reminder_percent"`
	EscalateToRole  string    `json:"escalate_to_role"`
	EscalateToUser  string    `json:"escalate_to_user"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SLANotification records a SLA action log entry.
// Maps to the sla_action_log table.
type SLANotification struct {
	ID               int64     `json:"id"`
	TicketID         int64     `json:"ticket_id"`
	NotificationType string    `json:"notification_type"` // reminder | escalate
	Stage            string    `json:"stage"`             // alias for dedup_key (backward compat)
	NotifiedUser     string    `json:"notified_user"`
	NotifiedAt       time.Time `json:"notified_at"`
	SLAConfigID      int64     `json:"sla_config_id"`
}

// SQLTemplate represents a reusable SQL template/snippet.
type SQLTemplate struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SQLContent  string    `json:"sql_content"`
	DBType      string    `json:"db_type"`
	Category    string    `json:"category"`
	ParamsJSON  string    `json:"params_json"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PermissionRequestStatus represents the status of a permission request.
type PermissionRequestStatus string

const (
	PermReqStatusPending  PermissionRequestStatus = "PENDING"
	PermReqStatusApproved PermissionRequestStatus = "APPROVED"
	PermReqStatusRejected PermissionRequestStatus = "REJECTED"
	PermReqStatusExpired  PermissionRequestStatus = "EXPIRED"
	PermReqStatusRevoked  PermissionRequestStatus = "REVOKED"
)

// PermissionRequest represents a temporary access permission request for a sensitive table.
type PermissionRequest struct {
	ID             int64                  `json:"id"`
	ApplicantID    int64                  `json:"applicant_id"`
	ApplicantName  string                 `json:"applicant_name,omitempty"`
	ApproverID     int64                  `json:"approver_id,omitempty"`
	ApproverName   string                 `json:"approver_name,omitempty"`
	DatasourceID   int64                  `json:"datasource_id"`
	DatasourceName string                 `json:"datasource_name,omitempty"`
	Database       string                 `json:"database"`
	TableName      string                 `json:"table_name"`
	Actions        string                 `json:"actions"` // comma-separated: select,update,delete,ddl,export
	Reason         string                 `json:"reason"`
	Status         PermissionRequestStatus `json:"status"`
	ApproveComment string                 `json:"approve_comment,omitempty"`
	ApprovedAt     *time.Time             `json:"approved_at,omitempty"`
	ExpiresAt      time.Time              `json:"expires_at"`
	RevokedAt      *time.Time             `json:"revoked_at,omitempty"`
	RevokedBy      int64                  `json:"revoked_by,omitempty"`
	RevokeReason   string                 `json:"revoke_reason,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// SharedResult represents a shared query result link.
type SharedResult struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	Username       string     `json:"username,omitempty"`
	Token          string     `json:"token"`
	ColumnsJSON    string     `json:"-"`
	RowsJSON       string     `json:"-"`
	RowCount       int64      `json:"row_count"`
	ExpiresAt      time.Time  `json:"expires_at"`
	PasswordHash   string     `json:"-"`
	SQLSummary     string     `json:"sql_summary,omitempty"`
	DatasourceName string     `json:"datasource_name,omitempty"`
	Revoked        bool       `json:"revoked"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// SharedResultPublic is the public view of a shared result (no sensitive fields).
type SharedResultPublic struct {
	ID             int64                    `json:"id"`
	Columns        []string                 `json:"columns"`
	Rows           []map[string]interface{} `json:"rows"`
	RowCount       int64                    `json:"row_count"`
	SQLSummary     string                   `json:"sql_summary,omitempty"`
	DatasourceName string                   `json:"datasource_name,omitempty"`
	ExpiresAt      string                   `json:"expires_at"`
	HasPassword    bool                     `json:"has_password"`
	CreatedAt      string                   `json:"created_at"`
}

// WebVital represents a Core Web Vitals metric record.
type WebVital struct {
	ID             int64     `json:"id"`
	MetricName     string    `json:"metric_name"`
	Value          float64   `json:"value"`
	Rating         string    `json:"rating"` // good, needs-improvement, poor
	Path           string    `json:"path"`
	NavigationType string    `json:"navigation_type,omitempty"`
	UserAgent      string    `json:"user_agent,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
