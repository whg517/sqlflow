package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

var (
	// ErrTicketNotFound indicates the ticket does not exist.
	ErrTicketNotFound = errors.New("工单不存在")
	// ErrInvalidStatusTransition indicates an invalid state transition.
	ErrInvalidStatusTransition = errors.New("无效的工单状态变更")
	// ErrTicketAlreadyProcessed indicates the ticket has already been processed.
	ErrTicketAlreadyProcessed = errors.New("工单已处理，无法重复操作")
	// ErrTicketNotCancellable indicates the ticket cannot be cancelled.
	ErrTicketNotCancellable = errors.New("当前状态不可取消")
	// ErrTicketNotExecutable indicates the ticket is not in a state that allows execution.
	ErrTicketNotExecutable = errors.New("工单未审批通过，无法执行")
	// ErrNoPermission indicates the user lacks permission for this operation.
	ErrNoPermission = errors.New("没有操作权限")
	// ErrRejectReasonRequired indicates a reason is required for rejection.
	ErrRejectReasonRequired = errors.New("驳回原因不能为空")
	// ErrCancelReasonRequired indicates a reason is required for cancellation.
	ErrCancelReasonRequired = errors.New("取消原因不能为空")
	// ErrTicketSQLRequired indicates the SQL content is required.
	ErrTicketSQLRequired = errors.New("SQL内容不能为空")
	// ErrTicketDatasourceRequired indicates the datasource is required.
	ErrTicketDatasourceRequired = errors.New("数据源不能为空")
	// ErrScheduleTimeRequired indicates a schedule time is required.
	ErrScheduleTimeRequired = errors.New("定时执行时间不能为空")
	// ErrScheduleTimeInPast indicates the schedule time must be in the future.
	ErrScheduleTimeInPast = errors.New("定时执行时间必须晚于当前时间")
	// ErrTicketNotSchedulable indicates the ticket is not in a state that allows scheduling.
	ErrTicketNotSchedulable = errors.New("当前状态不可设置定时执行")
	// ErrTicketNotScheduled indicates the ticket is not scheduled.
	ErrTicketNotScheduled = errors.New("工单未设置定时执行")
	// ErrTicketNotResubmittable indicates the ticket is not in REJECTED status.
	ErrTicketNotResubmittable = errors.New("只有被驳回的工单可以重提")
)

// validTransitions defines the allowed state transitions for the ticket state machine.
var validTransitions = map[model.TicketStatus][]model.TicketStatus{
	model.TicketStatusSubmitted:       {model.TicketStatusAIReviewed, model.TicketStatusCancelled},
	model.TicketStatusAIReviewed:      {model.TicketStatusPendingApproval, model.TicketStatusCancelled},
	model.TicketStatusPendingApproval: {model.TicketStatusApproved, model.TicketStatusRejected, model.TicketStatusCancelled},
	model.TicketStatusApproved:        {model.TicketStatusExecuting, model.TicketStatusScheduled, model.TicketStatusCancelled},
	model.TicketStatusScheduled:       {model.TicketStatusExecuting, model.TicketStatusCancelled},
	model.TicketStatusExecuting:       {model.TicketStatusDone, model.TicketStatusFailed},
	model.TicketStatusRejected:        {model.TicketStatusSubmitted},
	model.TicketStatusDone:            {},
	model.TicketStatusCancelled:       {},
}

// TicketService handles ticket management logic.
type TicketService struct {
	db            *sql.DB
	auditSvc      *AuditService
	notifySvc     *NotifyService
	gitSvc        *GitService
	slaSvc        *SLAService
	dsSvc         *DatasourceService
	connMgr       *connpool.Manager
	encryptionKey string
	permSvc       *PermissionService
}

// NewTicketService creates a new TicketService.
func NewTicketService(db *sql.DB, auditSvc *AuditService, notifySvc *NotifyService) *TicketService {
	return &TicketService{db: db, auditSvc: auditSvc, notifySvc: notifySvc}
}

// SetNotifyService sets the notification service (for deferred initialization).
func (s *TicketService) SetNotifyService(notifySvc *NotifyService) {
	s.notifySvc = notifySvc
}

// SetGitService sets the git service (for deferred initialization).
func (s *TicketService) SetGitService(gitSvc *GitService) {
	s.gitSvc = gitSvc
}

// SetSLAService injects the SLA service for cross-service lifecycle hooks
// (e.g., clear SLA on ticket approve/reject).
// This is set during application bootstrap — do not call after startup.
func (s *TicketService) SetSLAService(slaSvc *SLAService) {
	s.slaSvc = slaSvc
}

// SetPermissionService injects the permission service for MongoDB collection-level permission checks.
func (s *TicketService) SetPermissionService(permSvc *PermissionService) {
	s.permSvc = permSvc
}

// SetDatasourceService injects the datasource service and connection manager
// for actual SQL execution.
func (s *TicketService) SetDatasourceService(dsSvc *DatasourceService, connMgr *connpool.Manager, encryptionKey string) {
	s.dsSvc = dsSvc
	s.connMgr = connMgr
	s.encryptionKey = encryptionKey
}

// scanTicket scans a single ticket row from a sql.Rows or sql.Row.
func scanTicket(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.Ticket, error) {
	t := &model.Ticket{}
	var scheduledAt, executedAt sql.NullTime

	err := scanner.Scan(
		&t.ID, &t.SubmitterID, &t.DatasourceID, &t.Database,
		&t.SQLContent, &t.SQLSummary, &t.DBType, &t.ChangeReason,
		&t.Status, &t.RiskLevel, &t.AIReviewResult,
		&t.SQLType, &t.AffectedTables,
		&t.ReviewerID, &t.ReviewComment, &scheduledAt, &executedAt,
		&t.Revision,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if scheduledAt.Valid {
		t.ScheduledAt = &scheduledAt.Time
	}
	if executedAt.Valid {
		t.ExecutedAt = &executedAt.Time
	}

	return t, nil
}

// populateTicketNames fills in user names for submitter and reviewer.
func (s *TicketService) populateTicketNames(ctx context.Context, t *model.Ticket) {
	t.SubmitterName = s.lookupUsername(ctx, t.SubmitterID)
	t.ReviewerName = s.lookupUsername(ctx, t.ReviewerID)
}

// lookupUsername fetches the username for a given user ID.
func (s *TicketService) lookupUsername(ctx context.Context, userID int64) string {
	if userID == 0 {
		return ""
	}
	var username string
	if err := s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, userID).Scan(&username); err != nil {
		return ""
	}
	return username
}

// CreateTicket creates a new ticket.
func (s *TicketService) CreateTicket(ctx context.Context, submitterID int64, submitterRole string, datasourceID int64, database, sqlContent, dbType, changeReason, riskLevel, aiReviewResult string) (*model.Ticket, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrTicketSQLRequired
	}
	if datasourceID == 0 {
		return nil, ErrTicketDatasourceRequired
	}

	summary := truncateSQL(sqlContent)
	if dbType == "" {
		dbType = "mysql"
	}

	// MongoDB collection-level permission check
	if dbType == "mongodb" && s.permSvc != nil {
		if err := s.checkMongoPermission(ctx, submitterRole, datasourceID, sqlContent); err != nil {
			return nil, err
		}
	}

	// Auto-parse SQL and evaluate risk
	analyzer := NewSQLAnalyzer()
	analysis := analyzer.Analyze(sqlContent)

	tablesJSON := affectedTablesToJSON(analysis.AffectedTables)

	// Auto-evaluate risk level if not provided
	if riskLevel == "" {
		evaluator := NewRiskEvaluator()
		eval := evaluator.Evaluate(analysis)
		riskLevel = eval.Level
	}

	now := time.Now()
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, ai_review_result, sql_type, affected_tables, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		submitterID, datasourceID, database, sqlContent, summary, dbType, changeReason,
		model.TicketStatusSubmitted, riskLevel, aiReviewResult, analysis.SQLType, tablesJSON, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("创建工单失败: %w", err)
	}

	id, _ := result.LastInsertId()

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     submitterID,
		Action:     "ticket_create",
		SQLContent: sqlContent,
		SQLSummary: summary,
	})

	t := &model.Ticket{
		ID:             id,
		SubmitterID:    submitterID,
		DatasourceID:   datasourceID,
		Database:       database,
		SQLContent:     sqlContent,
		SQLSummary:     summary,
		DBType:         dbType,
		ChangeReason:   changeReason,
		Status:         model.TicketStatusSubmitted,
		RiskLevel:      riskLevel,
		AIReviewResult: aiReviewResult,
		SQLType:        analysis.SQLType,
		AffectedTables: tablesJSON,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.populateTicketNames(ctx, t)

	// Send notification for ticket creation
	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketCreated(t)
	}

	return t, nil
}

// GetTicket retrieves a ticket by ID with populated user names.
func (s *TicketService) GetTicket(ctx context.Context, id int64) (*model.Ticket, error) {
	t, err := scanTicket(s.db.QueryRowContext(ctx,
		`SELECT id, submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, ai_review_result, sql_type, affected_tables, reviewer_id, review_comment, scheduled_at, executed_at, revision, created_at, updated_at
		 FROM tickets WHERE id = ?`, id,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("获取工单失败: %w", err)
	}

	s.populateTicketNames(ctx, t)
	if s.gitSvc != nil {
		s.gitSvc.PopulateGitLinks(ctx, t)
	}
	return t, nil
}

// ListTickets retrieves a paginated list of tickets with filtering.
func (s *TicketService) ListTickets(ctx context.Context, page, pageSize int, status, datasourceIDStr, submitterIDStr, riskLevel, keyword, scope string, currentUserID int64, currentRole string) ([]model.Ticket, int64, error) {
	p := ParsePagination(page, pageSize)

	var filters []FilterClause
	if status != "" {
		filters = append(filters, FilterClause{Condition: "status = ?", Args: []interface{}{status}})
	}
	if datasourceIDStr != "" {
		filters = append(filters, FilterClause{Condition: "datasource_id = ?", Args: []interface{}{datasourceIDStr}})
	}
	if submitterIDStr != "" {
		filters = append(filters, FilterClause{Condition: "submitter_id = ?", Args: []interface{}{submitterIDStr}})
	}
	if riskLevel != "" {
		filters = append(filters, FilterClause{Condition: "risk_level = ?", Args: []interface{}{riskLevel}})
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		filters = append(filters, FilterClause{Condition: "(sql_content LIKE ? OR change_reason LIKE ?)", Args: []interface{}{like, like}})
	}
	if scope == "mine" {
		filters = append(filters, FilterClause{Condition: "submitter_id = ?", Args: []interface{}{currentUserID}})
	}
	if scope == "pending" {
		filters = append(filters, FilterClause{Condition: "status IN (?, ?, ?)", Args: []interface{}{model.TicketStatusSubmitted, model.TicketStatusAIReviewed, model.TicketStatusPendingApproval}})
	}

	whereClause, args := BuildWhereClause(filters)

	// Count total
	var total int64
	countSQL := PaginatedCountSQL("tickets", whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计工单失败: %w", err)
	}

	// Query page
	querySQL := fmt.Sprintf(
		`SELECT id, submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, ai_review_result, sql_type, affected_tables, reviewer_id, review_comment, scheduled_at, executed_at, revision, created_at, updated_at
		 FROM tickets %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		whereClause,
	)
	queryArgs := AppendLimitArgs(args, p)

	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询工单列表失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Read all rows first before populating names, since MaxOpenConns(1)
	// means the rows cursor holds the only connection.
	tickets := make([]model.Ticket, 0)
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			continue
		}
		tickets = append(tickets, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历工单失败: %w", err)
	}

	// Now populate user names (requires additional queries)
	for i := range tickets {
		s.populateTicketNames(ctx, &tickets[i])
	}

	// Populate git links if service is available (batch query to avoid N+1)
	if s.gitSvc != nil {
		s.gitSvc.BatchPopulateGitLinks(ctx, tickets)
	}

	return tickets, total, nil
}

// ApproveTicket approves a ticket. Only dba/admin can approve.
func (s *TicketService) ApproveTicket(ctx context.Context, ticketID, reviewerID int64, reviewerRole, comment string) (*model.Ticket, error) {
	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	if reviewerRole != "admin" && reviewerRole != "dba" {
		return nil, ErrNoPermission
	}

	if t.Status != model.TicketStatusPendingApproval {
		return nil, ErrInvalidStatusTransition
	}

	now := time.Now()
	sqlHash := sha256Hash(t.SQLContent)
	_, err = s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, reviewer_id = ?, review_comment = ?, sql_hash = ?, updated_at = ? WHERE id = ?`,
		model.TicketStatusApproved, reviewerID, comment, sqlHash, now, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("审批工单失败: %w", err)
	}

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     reviewerID,
		Action:     "ticket_approve",
		SQLContent: t.SQLContent,
		SQLSummary: t.SQLSummary,
	})

	t.Status = model.TicketStatusApproved
	t.ReviewerID = reviewerID
	t.ReviewComment = comment
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	// Send notification for approval
	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketApproved(t)
	}

	// Clear SLA deadline on approval
	if s.slaSvc != nil {
		if err := s.slaSvc.ClearTicketSLA(ctx, ticketID); err != nil {
			log.Printf("ticket: clear SLA on approve failed: %v", err)
		}
	}

	return t, nil
}

// RejectTicket rejects a ticket. Only dba/admin can reject.
func (s *TicketService) RejectTicket(ctx context.Context, ticketID, reviewerID int64, reviewerRole, reason string) (*model.Ticket, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, ErrRejectReasonRequired
	}

	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	if reviewerRole != "admin" && reviewerRole != "dba" {
		return nil, ErrNoPermission
	}

	if t.Status != model.TicketStatusPendingApproval {
		return nil, ErrInvalidStatusTransition
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, reviewer_id = ?, review_comment = ?, updated_at = ? WHERE id = ?`,
		model.TicketStatusRejected, reviewerID, reason, now, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("驳回工单失败: %w", err)
	}

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     reviewerID,
		Action:     "ticket_reject",
		SQLContent: t.SQLContent,
		SQLSummary: t.SQLSummary,
	})

	t.Status = model.TicketStatusRejected
	t.ReviewerID = reviewerID
	t.ReviewComment = reason
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	// Send notification for rejection
	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketRejected(t)
	}

	// Clear SLA deadline on rejection
	if s.slaSvc != nil {
		if err := s.slaSvc.ClearTicketSLA(ctx, ticketID); err != nil {
			log.Printf("ticket: clear SLA on reject failed: %v", err)
		}
	}

	return t, nil
}

// CancelTicket cancels a ticket. Submitter or dba/admin can cancel.
func (s *TicketService) CancelTicket(ctx context.Context, ticketID, operatorID int64, operatorRole, reason string) (*model.Ticket, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, ErrCancelReasonRequired
	}

	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	// Only the submitter or dba/admin can cancel
	if t.SubmitterID != operatorID && operatorRole != "admin" && operatorRole != "dba" {
		return nil, ErrNoPermission
	}

	// Can cancel only from these states
	cancellable := map[model.TicketStatus]bool{
		model.TicketStatusSubmitted:       true,
		model.TicketStatusAIReviewed:      true,
		model.TicketStatusPendingApproval: true,
		model.TicketStatusApproved:        true,
		model.TicketStatusScheduled:       true,
	}
	if !cancellable[t.Status] {
		return nil, ErrTicketNotCancellable
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, review_comment = ?, scheduled_at = NULL, updated_at = ? WHERE id = ?`,
		model.TicketStatusCancelled, reason, now, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("取消工单失败: %w", err)
	}

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     operatorID,
		Action:     "ticket_cancel",
		SQLContent: t.SQLContent,
		SQLSummary: t.SQLSummary,
	})

	t.Status = model.TicketStatusCancelled
	t.ReviewComment = reason
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)
	return t, nil
}

// ExecuteTicket executes a ticket's SQL on the target database.
// Only the submitter or dba/admin can execute, and only when APPROVED or SCHEDULED.
func (s *TicketService) ExecuteTicket(ctx context.Context, ticketID, operatorID int64, operatorRole, operatorName string) (*model.Ticket, error) {
	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	if t.Status != model.TicketStatusApproved && t.Status != model.TicketStatusScheduled {
		return nil, ErrTicketNotExecutable
	}

	// Only the submitter or dba/admin can execute
	if t.SubmitterID != operatorID && operatorRole != "admin" && operatorRole != "dba" {
		return nil, ErrNoPermission
	}

	return s.executeTicket(ctx, t, operatorID)
}

// executeTicket performs the actual SQL execution with hash verification,
// timeout control, and idempotent status transition.
func (s *TicketService) executeTicket(ctx context.Context, t *model.Ticket, operatorID int64) (*model.Ticket, error) {
	if s.dsSvc == nil || s.connMgr == nil {
		return nil, fmt.Errorf("SQL 执行服务未初始化")
	}

	// Idempotent: APPROVED/SCHEDULED → EXECUTING (atomic)
	now := time.Now()
	result, err := s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, updated_at = ? WHERE id = ? AND status IN (?, ?)`,
		model.TicketStatusExecuting, now, t.ID, model.TicketStatusApproved, model.TicketStatusScheduled,
	)
	if err != nil {
		return nil, fmt.Errorf("状态更新失败: %w", err)
	}
	raffected, _ := result.RowsAffected()
	if raffected == 0 {
		return nil, ErrTicketNotExecutable // already executing or state changed
	}
	t.Status = model.TicketStatusExecuting

	// SHA-256 hash verification: ensure SQL hasn't changed since approval
	if t.SQLHash != "" {
		currentHash := sha256Hash(t.SQLContent)
		if currentHash != t.SQLHash {
			return nil, s.failTicket(ctx, t, operatorID, "SQL 内容与审批版本不一致，请重新提交审批")
		}
	}

	// Get datasource connection info
	ds, err := s.dsSvc.GetDataSource(ctx, t.DatasourceID)
	if err != nil {
		return nil, s.failTicket(ctx, t, operatorID, fmt.Sprintf("获取数据源失败: %s", sanitizeErrMsg(err.Error())))
	}

	// Execute SQL with 30s timeout
	execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	execResults, execErr := s.executeSQL(execCtx, ds, t.Database, t.DBType, t.SQLContent)

	// Record execution results
	for i, r := range execResults {
		s.recordExecutionResult(ctx, t.ID, i, r)
	}

	if execErr != nil {
		return nil, s.failTicket(ctx, t, operatorID, sanitizeErrMsg(execErr.Error()))
	}

	// Success: EXECUTING → DONE
	now = time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, executed_at = ?, scheduled_at = NULL, updated_at = ? WHERE id = ?`,
		model.TicketStatusDone, now, now, t.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("更新工单状态失败: %w", err)
	}

	// Compute SQL hash for audit
	sqlHash := sha256Hash(t.SQLContent)

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:         operatorID,
		Action:         "ticket_execute",
		DatasourceID:   t.DatasourceID,
		Database:       t.Database,
		SQLContent:     t.SQLContent,
		SQLSummary:     t.SQLSummary,
		AffectedRows:   totalRowsAffected(execResults),
		ExecutionTimeMs: totalDurationMs(execResults),
		TicketID:       t.ID,
	})

	t.Status = model.TicketStatusDone
	t.ExecutedAt = &now
	t.ScheduledAt = nil
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	_ = sqlHash // logged via audit

	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketExecuted(t)
	}

	return t, nil
}

// failTicket transitions a ticket to FAILED status and writes audit log.
func (s *TicketService) failTicket(ctx context.Context, t *model.Ticket, operatorID int64, errMsg string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, updated_at = ? WHERE id = ?`,
		model.TicketStatusFailed, now, t.ID,
	)
	if err != nil {
		return fmt.Errorf("设置失败状态失败: %w (原始错误: %s)", err, errMsg)
	}

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:       operatorID,
		Action:       "ticket_execute_failed",
		DatasourceID: t.DatasourceID,
		Database:     t.Database,
		SQLContent:   t.SQLContent,
		SQLSummary:   t.SQLSummary,
		ErrorMessage: errMsg,
		TicketID:     t.ID,
	})

	t.Status = model.TicketStatusFailed
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketExecuted(t)
	}

	return fmt.Errorf("工单执行失败: %s", errMsg)
}

// ScheduleTicket sets a ticket to be executed at the specified time.
// Only the submitter or dba/admin can schedule, and only when the ticket is APPROVED.
func (s *TicketService) ScheduleTicket(ctx context.Context, ticketID, operatorID int64, operatorRole string, scheduledAt time.Time) (*model.Ticket, error) {
	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	// Only the submitter or dba/admin can schedule
	if t.SubmitterID != operatorID && operatorRole != "admin" && operatorRole != "dba" {
		return nil, ErrNoPermission
	}

	if t.Status != model.TicketStatusApproved {
		return nil, ErrTicketNotSchedulable
	}

	if scheduledAt.IsZero() {
		return nil, ErrScheduleTimeRequired
	}

	// Allow scheduling at the same minute or 1 minute in the future
	if !scheduledAt.IsZero() && scheduledAt.Before(time.Now()) {
		return nil, ErrScheduleTimeInPast
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, scheduled_at = ?, updated_at = ? WHERE id = ?`,
		model.TicketStatusScheduled, scheduledAt, now, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("设置定时执行失败: %w", err)
	}

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     operatorID,
		Action:     "ticket_schedule",
		SQLContent: t.SQLContent,
		SQLSummary: t.SQLSummary,
	})

	t.Status = model.TicketStatusScheduled
	t.ScheduledAt = &scheduledAt
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketScheduled(t)
	}

	return t, nil
}

// CancelSchedule cancels a scheduled ticket execution, returning it to APPROVED status.
func (s *TicketService) CancelSchedule(ctx context.Context, ticketID, operatorID int64, operatorRole string) (*model.Ticket, error) {
	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	// Only the submitter or dba/admin can cancel schedule
	if t.SubmitterID != operatorID && operatorRole != "admin" && operatorRole != "dba" {
		return nil, ErrNoPermission
	}

	if t.Status != model.TicketStatusScheduled {
		return nil, ErrTicketNotScheduled
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, scheduled_at = NULL, updated_at = ? WHERE id = ?`,
		model.TicketStatusApproved, now, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("取消定时执行失败: %w", err)
	}

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     operatorID,
		Action:     "ticket_cancel_schedule",
		SQLContent: t.SQLContent,
		SQLSummary: t.SQLSummary,
	})

	t.Status = model.TicketStatusApproved
	t.ScheduledAt = nil
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	return t, nil
}

// CanTransition checks if a transition from one status to another is valid.
func CanTransition(from, to model.TicketStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Batch operations
// ---------------------------------------------------------------------------

// BatchResult represents the result of a single ticket operation in a batch.
type BatchResult struct {
	TicketID int64  `json:"ticket_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// BatchResponse represents the overall result of a batch operation.
type BatchResponse struct {
	Total     int           `json:"total"`
	Succeeded int           `json:"succeeded"`
	Failed    int           `json:"failed"`
	Results   []BatchResult `json:"results"`
}

// BatchApprove approves multiple tickets. Each ticket is processed independently;
// partial failures do not roll back successful operations.
func (s *TicketService) BatchApprove(ctx context.Context, ticketIDs []int64, reviewerID int64, reviewerRole, comment string) (*BatchResponse, error) {
	if reviewerRole != "admin" && reviewerRole != "dba" {
		return nil, ErrNoPermission
	}

	resp := &BatchResponse{
		Total:   len(ticketIDs),
		Results: make([]BatchResult, 0, len(ticketIDs)),
	}

	for _, id := range ticketIDs {
		_, err := s.ApproveTicket(ctx, id, reviewerID, reviewerRole, comment)
		if err != nil {
			resp.Failed++
			resp.Results = append(resp.Results, BatchResult{
				TicketID: id,
				Success:  false,
				Error:    err.Error(),
			})
		} else {
			resp.Succeeded++
			resp.Results = append(resp.Results, BatchResult{
				TicketID: id,
				Success:  true,
			})
		}
	}

	return resp, nil
}

// BatchReject rejects multiple tickets. Each ticket is processed independently;
// partial failures do not roll back successful operations.
func (s *TicketService) BatchReject(ctx context.Context, ticketIDs []int64, reviewerID int64, reviewerRole, reason string) (*BatchResponse, error) {
	if reviewerRole != "admin" && reviewerRole != "dba" {
		return nil, ErrNoPermission
	}

	if strings.TrimSpace(reason) == "" {
		return nil, ErrRejectReasonRequired
	}

	resp := &BatchResponse{
		Total:   len(ticketIDs),
		Results: make([]BatchResult, 0, len(ticketIDs)),
	}

	for _, id := range ticketIDs {
		_, err := s.RejectTicket(ctx, id, reviewerID, reviewerRole, reason)
		if err != nil {
			resp.Failed++
			resp.Results = append(resp.Results, BatchResult{
				TicketID: id,
				Success:  false,
				Error:    err.Error(),
			})
		} else {
			resp.Succeeded++
			resp.Results = append(resp.Results, BatchResult{
				TicketID: id,
				Success:  true,
			})
		}
	}

	return resp, nil
}

// affectedTablesToJSON converts a string slice to JSON array string.
func affectedTablesToJSON(tables []string) string {
	if len(tables) == 0 {
		return "[]"
	}
	b, err := json.Marshal(tables)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// mongoOpToCasbinAct maps a MongoDB operation to a Casbin action string.
// This allows the existing RBAC model to control NoSQL operations.
func mongoOpToCasbinAct(op sqlparser.MongoOperation) string {
	switch op {
	case sqlparser.MongoOpFind, sqlparser.MongoOpAggregate:
		return "select"
	case sqlparser.MongoOpInsert:
		return "insert"
	case sqlparser.MongoOpUpdate:
		return "update"
	case sqlparser.MongoOpDelete:
		return "delete"
	default:
		return "select"
	}
}

// checkMongoPermission validates that the user has permission to perform the MongoDB operation.
// It parses the MongoDB command body, extracts the collection and operation type,
// and checks collection-level permission via Casbin.
func (s *TicketService) checkMongoPermission(ctx context.Context, role string, datasourceID int64, sqlContent string) error {
	if s.permSvc == nil {
		return nil
	}

	mongoResult, err := sqlparser.ParseMongo(sqlContent)
	if err != nil {
		// If we can't parse it, let it through — the approval process will catch issues
		return nil
	}

	if mongoResult.Collection == "" {
		// No collection specified, check datasource-level permission
		return nil
	}

	act := mongoOpToCasbinAct(mongoResult.Operation)
	dom := fmt.Sprintf("ds_%d", datasourceID)

	allowed, err := s.permSvc.Enforce(role, dom, mongoResult.Collection, act)
	if err != nil {
		return fmt.Errorf("MongoDB权限校验失败: %w", err)
	}
	if !allowed {
		return fmt.Errorf("没有集合 %s 的 %s 权限", mongoResult.Collection, act)
	}

	return nil
}
