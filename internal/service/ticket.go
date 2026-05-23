package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
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
)

// validTransitions defines the allowed state transitions for the ticket state machine.
var validTransitions = map[model.TicketStatus][]model.TicketStatus{
	model.TicketStatusSubmitted:       {model.TicketStatusAIReviewed, model.TicketStatusCancelled},
	model.TicketStatusAIReviewed:      {model.TicketStatusPendingApproval, model.TicketStatusCancelled},
	model.TicketStatusPendingApproval: {model.TicketStatusApproved, model.TicketStatusRejected, model.TicketStatusCancelled},
	model.TicketStatusApproved:        {model.TicketStatusExecuting, model.TicketStatusScheduled, model.TicketStatusCancelled},
	model.TicketStatusScheduled:       {model.TicketStatusExecuting, model.TicketStatusCancelled},
	model.TicketStatusExecuting:       {model.TicketStatusDone},
	model.TicketStatusRejected:        {},
	model.TicketStatusDone:            {},
	model.TicketStatusCancelled:       {},
}

// TicketService handles ticket management logic.
type TicketService struct {
	db        *sql.DB
	auditSvc  *AuditService
	notifySvc *NotifyService
}

// NewTicketService creates a new TicketService.
func NewTicketService(db *sql.DB, auditSvc *AuditService, notifySvc *NotifyService) *TicketService {
	return &TicketService{db: db, auditSvc: auditSvc, notifySvc: notifySvc}
}

// SetNotifyService sets the notification service (for deferred initialization).
func (s *TicketService) SetNotifyService(notifySvc *NotifyService) {
	s.notifySvc = notifySvc
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
		&t.ReviewerID, &t.ReviewComment, &scheduledAt, &executedAt,
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
func (s *TicketService) CreateTicket(ctx context.Context, submitterID int64, datasourceID int64, database, sqlContent, dbType, changeReason, riskLevel, aiReviewResult string) (*model.Ticket, error) {
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

	now := time.Now()
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, ai_review_result, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		submitterID, datasourceID, database, sqlContent, summary, dbType, changeReason,
		model.TicketStatusSubmitted, riskLevel, aiReviewResult, now, now,
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
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.populateTicketNames(ctx, t)

	// Send DingTalk notification for ticket creation
	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketCreated(t)
	}

	return t, nil
}

// GetTicket retrieves a ticket by ID with populated user names.
func (s *TicketService) GetTicket(ctx context.Context, id int64) (*model.Ticket, error) {
	t, err := scanTicket(s.db.QueryRowContext(ctx,
		`SELECT id, submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, ai_review_result, reviewer_id, review_comment, scheduled_at, executed_at, created_at, updated_at
		 FROM tickets WHERE id = ?`, id,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("获取工单失败: %w", err)
	}

	s.populateTicketNames(ctx, t)
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
		`SELECT id, submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, ai_review_result, reviewer_id, review_comment, scheduled_at, executed_at, created_at, updated_at
		 FROM tickets %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		whereClause,
	)
	queryArgs := AppendLimitArgs(args, p)

	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询工单列表失败: %w", err)
	}

	// Read all rows first before populating names, since MaxOpenConns(1)
	// means the rows cursor holds the only connection.
	tickets := make([]model.Ticket, 0)
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			rows.Close()
			continue
		}
		tickets = append(tickets, *t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, fmt.Errorf("遍历工单失败: %w", err)
	}
	rows.Close()

	// Now populate user names (requires additional queries)
	for i := range tickets {
		s.populateTicketNames(ctx, &tickets[i])
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
	_, err = s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, reviewer_id = ?, review_comment = ?, updated_at = ? WHERE id = ?`,
		model.TicketStatusApproved, reviewerID, comment, now, ticketID,
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

	// Send DingTalk notification for approval
	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketApproved(t)
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

	// Send DingTalk notification for rejection
	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketRejected(t)
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

// ExecuteTicket executes a ticket's SQL. Only the submitter or dba/admin can execute,
// and only when the ticket is in APPROVED or SCHEDULED status.
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

	now := time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE tickets SET status = ?, executed_at = ?, updated_at = ? WHERE id = ?`,
		model.TicketStatusDone, now, now, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("执行工单失败: %w", err)
	}

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:       operatorID,
		Action:       "ticket_execute",
		DatasourceID: t.DatasourceID,
		Database:     t.Database,
		SQLContent:   t.SQLContent,
		SQLSummary:   t.SQLSummary,
	})

	t.Status = model.TicketStatusDone
	t.ExecutedAt = &now
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	// Send DingTalk notification for execution
	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketExecuted(t)
	}

	return t, nil
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

