package ticket

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entTicket "github.com/whg517/sqlflow/internal/db/ent/ticket"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/notify"
	"github.com/whg517/sqlflow/internal/ops"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/platform/sqlparser"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"
	"github.com/whg517/sqlflow/internal/security"
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
	// ErrTicketExecUnavailable indicates the service was built without a
	// connection pool, so no datasource can be reached. This is a wiring fault —
	// app.Container always provides one.
	ErrTicketExecUnavailable = errors.New("工单执行服务未正确初始化")
	// ErrTicketExecNotSupported indicates the datasource's driver declares no
	// CapTicketExec, so it has no DML/DDL path at all.
	ErrTicketExecNotSupported = errors.New("该数据源不支持通过工单执行变更")
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

// Deps are the collaborators a ticket Service needs.
//
// They are passed as a struct rather than as positional parameters because
// there are ten of them and most tests need two: a test that exercises the
// state machine names DB and Audit and leaves the rest zero, and the compiler
// still checks every field it does name. The previous shape — a three-argument
// constructor plus six Set* methods — left a Service mutable for its whole
// lifetime and gave no way to tell a deliberately-absent collaborator from one
// the bootstrap forgot to wire.
//
// A nil collaborator disables the behavior that needs it; each field says what
// that means.
type Deps struct {
	// DB is required.
	DB *db.DB
	// Audit records every ticket transition. nil is treated as auditlog.Discard.
	Audit auditlog.Writer
	// Notify sends lifecycle notifications. Without it transitions are silent.
	Notify *notify.Service
	// Git links tickets to commits. Without it links are not recorded.
	Git *ops.GitService
	// SLA clears deadlines on approve and reject. Without it they are left set.
	SLA *SLAService
	// Datasource, PoolManager and EncryptionKey are all needed to execute a
	// ticket's SQL; without them execution fails rather than silently skipping.
	Datasource    *datasource.Service
	PoolManager   *driver.PoolManager
	EncryptionKey string
	// Permission enforces collection-level checks for MongoDB tickets. Without
	// it those checks are skipped.
	Permission *security.Service
	// ApprovalEngine applies policy-based auto-approval. Without it every
	// ticket waits for a human.
	ApprovalEngine *ApprovalEngine
}

// Service handles ticket management logic.
type Service struct {
	database       *db.DB
	client         *ent.Client
	auditSvc       auditlog.Writer
	notifySvc      *notify.Service
	gitSvc         *ops.GitService
	slaSvc         *SLAService
	dsSvc          *datasource.Service
	poolMgr        *driver.PoolManager
	encryptionKey  string
	permSvc        *security.Service
	approvalEngine *ApprovalEngine
}

// New creates a Service from its dependencies.
//
// Everything is fixed at construction: the returned Service has no setters, so
// no code path can swap a collaborator out from under an in-flight ticket.
func New(deps Deps) *Service {
	return &Service{
		database:       deps.DB,
		client:         deps.DB.Client(),
		auditSvc:       auditlog.OrDiscard(deps.Audit),
		notifySvc:      deps.Notify,
		gitSvc:         deps.Git,
		slaSvc:         deps.SLA,
		dsSvc:          deps.Datasource,
		poolMgr:        deps.PoolManager,
		encryptionKey:  deps.EncryptionKey,
		permSvc:        deps.Permission,
		approvalEngine: deps.ApprovalEngine,
	}
}

// ticketColumns is the canonical column list for ticket reads.
//
// It must stay in the exact order scanTicket assigns its destinations. Keeping
// the two adjacent and referencing this constant everywhere is what prevents
// the drift that previously broke the scheduler (a stale 17-column SELECT fed
// into a 20-destination scan) and silently disabled the post-approval SQL
// integrity check (sql_hash written but never selected back).
const ticketColumns = `id, submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, ` +
	`status, risk_level, ai_review_result, sql_type, affected_tables, reviewer_id, review_comment, sql_hash, ` +
	`scheduled_at, executed_at, revision, created_at, updated_at`

// scanTicket scans a single ticket row from a sql.Rows or sql.Row.
// The query must select exactly ticketColumns, in that order.
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
		&t.ReviewerID, &t.ReviewComment, &t.SQLHash,
		&scheduledAt, &executedAt,
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

// casTicketStatus atomically moves a ticket from `from` to `to` and reports
// whether a row matched. `extra` adds column assignments to the same statement.
//
// Every approval-side transition must go through a compare-and-swap.
// Read-then-write leaves a window where two approvers both observe
// PENDING_APPROVAL and both write, producing two decisions for one ticket.
//
// ent's predicate-based Update() returns the affected row count, which is the
// compare and the swap in one statement — UpdateOneID cannot express this.
func casTicketStatus(
	ctx context.Context,
	tickets *ent.TicketClient,
	ticketID int64,
	from, to model.TicketStatus,
	now time.Time,
	extra func(*ent.TicketUpdate) *ent.TicketUpdate,
) (bool, error) {
	upd := tickets.Update().
		Where(entTicket.IDEQ(int(ticketID)), entTicket.StatusEQ(string(from))).
		SetStatus(string(to)).
		SetUpdatedAt(now)
	if extra != nil {
		upd = extra(upd)
	}
	affected, err := upd.Save(ctx)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// populateTicketNames fills in user names for submitter and reviewer.
func (s *Service) populateTicketNames(ctx context.Context, t *model.Ticket) {
	t.SubmitterName = s.lookupUsername(ctx, t.SubmitterID)
	t.ReviewerName = s.lookupUsername(ctx, t.ReviewerID)
}

// lookupUsername fetches the username for a given user ID.
func (s *Service) lookupUsername(ctx context.Context, userID int64) string {
	if userID == 0 {
		return ""
	}
	u, err := s.client.User.Get(ctx, int(userID))
	if err != nil {
		return ""
	}
	return u.Username
}

// CreateTicket creates a new ticket.
//
// The risk level is always derived server-side from the submitted SQL. It is
// deliberately not a parameter: risk drives approval-policy matching, including
// auto-approval, so accepting it from the submitter would let them pick their
// own approval path.
func (s *Service) CreateTicket(ctx context.Context, submitterID int64, submitterRole string, datasourceID int64, database, sqlContent, dbType, changeReason string) (*model.Ticket, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrTicketSQLRequired
	}
	if datasourceID == 0 {
		return nil, ErrTicketDatasourceRequired
	}

	summary := auditlog.Summarize(sqlContent)
	if dbType == "" {
		dbType = "mysql"
	}

	// MongoDB collection-level permission check
	if dbType == "mongodb" && s.permSvc != nil {
		if err := s.checkMongoPermission(ctx, submitterRole, datasourceID, sqlContent); err != nil {
			return nil, err
		}
	}

	// Parse the SQL and derive the risk level. Both are server-side facts.
	analysis := NewSQLAnalyzer().Analyze(sqlContent)
	tablesJSON := affectedTablesToJSON(analysis.AffectedTables)
	riskLevel := NewRiskEvaluator().Evaluate(analysis).Level

	now := time.Now()
	saved, err := s.client.Ticket.Create().
		SetSubmitterID(submitterID).
		SetDatasourceID(datasourceID).
		SetDatabase(database).
		SetSQLContent(sqlContent).
		SetSQLSummary(summary).
		SetDbType(dbType).
		SetChangeReason(changeReason).
		SetStatus(string(model.TicketStatusSubmitted)).
		SetRiskLevel(riskLevel).
		SetSQLType(analysis.SQLType).
		SetAffectedTables(tablesJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建工单失败: %w", err)
	}

	id := int64(saved.ID)

	s.auditSvc.Write(ctx, auditlog.Record{
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
		SQLType:        analysis.SQLType,
		AffectedTables: tablesJSON,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.populateTicketNames(ctx, t)

	s.applyApprovalPolicy(ctx, t)

	// Send notification for ticket creation
	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketCreated(ctx, t)
	}

	return t, nil
}

// applyApprovalPolicy matches and applies the approval policy for a ticket that
// has just entered SUBMITTED, updating t.Status in place.
//
// Shared by creation and resubmission: a resubmitted ticket carries new SQL and
// therefore a new risk level, so it must be routed through policy matching
// again rather than inheriting the previous revision's approval chain.
//
// Matching or application failure is logged, not returned: the ticket stays in
// SUBMITTED for manual review rather than being rejected outright.
func (s *Service) applyApprovalPolicy(ctx context.Context, t *model.Ticket) {
	if s.approvalEngine == nil {
		return
	}

	policy, err := s.approvalEngine.MatchPolicy(ctx, t)
	if err != nil {
		log.Printf("ticket: match approval policy failed for ticket %d: %v", t.ID, err)
		return
	}

	result, err := s.approvalEngine.ApplyPolicy(ctx, t.ID, policy, t.SubmitterID)
	if err != nil {
		log.Printf("ticket: apply approval policy failed for ticket %d: %v", t.ID, err)
		return
	}

	if result.AutoApproved {
		t.Status = model.TicketStatusApproved
	} else {
		t.Status = model.TicketStatusPendingApproval
	}
	log.Printf("ticket: approval policy applied for ticket %d, auto_approved=%v, policy_id=%d",
		t.ID, result.AutoApproved, result.PolicyID)
}

// GetTicket retrieves a ticket by ID with populated user names.
func (s *Service) GetTicket(ctx context.Context, id int64) (*model.Ticket, error) {
	t, err := scanTicket(s.database.DB.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT %s FROM tickets WHERE id = ?`, ticketColumns), id,
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

// GetTicketForActor applies the resource ownership boundary for ticket reads.
// Submitters and assigned reviewers can read a ticket; DBA/Admin can read all.
func (s *Service) GetTicketForActor(ctx context.Context, id, userID int64, role string) (*model.Ticket, error) {
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == "admin" || role == "dba" || t.SubmitterID == userID || t.ReviewerID == userID {
		return t, nil
	}
	return nil, ErrNoPermission
}

// ListTickets retrieves a paginated list of tickets with filtering.
func (s *Service) ListTickets(ctx context.Context, page, pageSize int, status, datasourceIDStr, submitterIDStr, riskLevel, keyword, scope string, currentUserID int64, currentRole string) ([]model.Ticket, int64, error) {
	p := sqlutil.ParsePagination(page, pageSize)

	var filters []sqlutil.FilterClause
	if currentRole != "admin" && currentRole != "dba" {
		// Non-governance roles can never widen this boundary with query params.
		filters = append(filters, sqlutil.FilterClause{Condition: "submitter_id = ?", Args: []interface{}{currentUserID}})
		submitterIDStr = ""
	}
	if status != "" {
		filters = append(filters, sqlutil.FilterClause{Condition: "status = ?", Args: []interface{}{status}})
	}
	if datasourceIDStr != "" {
		filters = append(filters, sqlutil.FilterClause{Condition: "datasource_id = ?", Args: []interface{}{datasourceIDStr}})
	}
	if submitterIDStr != "" {
		filters = append(filters, sqlutil.FilterClause{Condition: "submitter_id = ?", Args: []interface{}{submitterIDStr}})
	}
	if riskLevel != "" {
		filters = append(filters, sqlutil.FilterClause{Condition: "risk_level = ?", Args: []interface{}{riskLevel}})
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		filters = append(filters, sqlutil.FilterClause{Condition: "(sql_content LIKE ? OR change_reason LIKE ?)", Args: []interface{}{like, like}})
	}
	if scope == "mine" {
		filters = append(filters, sqlutil.FilterClause{Condition: "submitter_id = ?", Args: []interface{}{currentUserID}})
	}
	if scope == "pending" {
		filters = append(filters, sqlutil.FilterClause{Condition: "status IN (?, ?, ?)", Args: []interface{}{model.TicketStatusSubmitted, model.TicketStatusAIReviewed, model.TicketStatusPendingApproval}})
	}

	whereClause, args := sqlutil.BuildWhereClause(filters)

	// Count total
	var total int64
	countSQL := sqlutil.PaginatedCountSQL("tickets", whereClause)
	if err := s.database.DB.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计工单失败: %w", err)
	}

	// Query page
	querySQL := fmt.Sprintf(
		`SELECT %s FROM tickets %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		ticketColumns, whereClause,
	)
	queryArgs := sqlutil.AppendLimitArgs(args, p)

	rows, err := s.database.DB.QueryContext(ctx, querySQL, queryArgs...)
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
			// A scan failure here means schema drift or corruption, not a
			// per-row data issue. Dropping the row silently is how the
			// scheduler's column mismatch stayed hidden — make it visible.
			log.Printf("ticket: scan ticket row failed: %v", err)
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
func (s *Service) ApproveTicket(ctx context.Context, ticketID, reviewerID int64, reviewerRole, comment string) (*model.Ticket, error) {
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
	// Pin the SQL body that is being approved, so executeTicket can refuse a
	// ticket whose statement changed afterwards.
	sqlHash := sha256Hash(t.SQLContent)
	swapped, err := casTicketStatus(ctx, s.client.Ticket, ticketID,
		model.TicketStatusPendingApproval, model.TicketStatusApproved, now,
		func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetReviewerID(reviewerID).SetReviewComment(comment).SetSQLHash(sqlHash)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("审批工单失败: %w", err)
	}
	if !swapped {
		// Another approver decided this ticket between the read and the write.
		return nil, ErrInvalidStatusTransition
	}

	s.auditSvc.Write(ctx, auditlog.Record{
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
		s.notifySvc.NotifyTicketApproved(ctx, t)
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
func (s *Service) RejectTicket(ctx context.Context, ticketID, reviewerID int64, reviewerRole, reason string) (*model.Ticket, error) {
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
	swapped, err := casTicketStatus(ctx, s.client.Ticket, ticketID,
		model.TicketStatusPendingApproval, model.TicketStatusRejected, now,
		func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetReviewerID(reviewerID).SetReviewComment(reason)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("驳回工单失败: %w", err)
	}
	if !swapped {
		// Another approver decided this ticket between the read and the write.
		return nil, ErrInvalidStatusTransition
	}

	s.auditSvc.Write(ctx, auditlog.Record{
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
		s.notifySvc.NotifyTicketRejected(ctx, t)
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
func (s *Service) CancelTicket(ctx context.Context, ticketID, operatorID int64, operatorRole, reason string) (*model.Ticket, error) {
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
	_, err = s.client.Ticket.UpdateOneID(int(ticketID)).
		SetStatus(string(model.TicketStatusCancelled)).
		SetReviewComment(reason).
		ClearScheduledAt().
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("取消工单失败: %w", err)
	}

	s.auditSvc.Write(ctx, auditlog.Record{
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
func (s *Service) ExecuteTicket(ctx context.Context, ticketID, operatorID int64, operatorRole, operatorName string) (*model.Ticket, error) {
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
func (s *Service) executeTicket(ctx context.Context, t *model.Ticket, operatorID int64) (*model.Ticket, error) {
	if s.dsSvc == nil || s.poolMgr == nil {
		return nil, ErrTicketExecUnavailable
	}

	// Idempotent: APPROVED/SCHEDULED → EXECUTING
	// RAW_SQL: atomic CAS (Compare-And-Swap) with WHERE status IN — ent UpdateOneID does not
	// support conditional WHERE on current row values. Keep raw SQL for atomicity.
	now := time.Now()
	result, err := s.database.DB.ExecContext(ctx,
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
	_, err = s.client.Ticket.UpdateOneID(int(t.ID)).
		SetStatus(string(model.TicketStatusDone)).
		SetExecutedAt(now).
		ClearScheduledAt().
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("更新工单状态失败: %w", err)
	}

	// Compute SQL hash for audit
	sqlHash := sha256Hash(t.SQLContent)

	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:          operatorID,
		Action:          "ticket_execute",
		DatasourceID:    t.DatasourceID,
		Database:        t.Database,
		SQLContent:      t.SQLContent,
		SQLSummary:      t.SQLSummary,
		AffectedRows:    totalRowsAffected(execResults),
		ExecutionTimeMs: totalDurationMs(execResults),
		TicketID:        t.ID,
	})

	t.Status = model.TicketStatusDone
	t.ExecutedAt = &now
	t.ScheduledAt = nil
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	_ = sqlHash // logged via audit

	if s.notifySvc != nil {
		s.notifySvc.NotifyTicketExecuted(ctx, t)
	}

	return t, nil
}

// failTicket transitions a ticket to FAILED status and writes audit log.
func (s *Service) failTicket(ctx context.Context, t *model.Ticket, operatorID int64, errMsg string) error {
	now := time.Now()
	_, err := s.client.Ticket.UpdateOneID(int(t.ID)).
		SetStatus(string(model.TicketStatusFailed)).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("设置失败状态失败: %w (原始错误: %s)", err, errMsg)
	}

	s.auditSvc.Write(ctx, auditlog.Record{
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
		s.notifySvc.NotifyTicketFailed(ctx, t, errMsg)
	}

	return fmt.Errorf("工单执行失败: %s", errMsg)
}

// ScheduleTicket sets a ticket to be executed at the specified time.
// Only the submitter or dba/admin can schedule, and only when the ticket is APPROVED.
func (s *Service) ScheduleTicket(ctx context.Context, ticketID, operatorID int64, operatorRole string, scheduledAt time.Time) (*model.Ticket, error) {
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
	_, err = s.client.Ticket.UpdateOneID(int(ticketID)).
		SetStatus(string(model.TicketStatusScheduled)).
		SetScheduledAt(scheduledAt).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("设置定时执行失败: %w", err)
	}

	s.auditSvc.Write(ctx, auditlog.Record{
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
		s.notifySvc.NotifyTicketScheduled(ctx, t)
	}

	return t, nil
}

// CancelSchedule cancels a scheduled ticket execution, returning it to APPROVED status.
func (s *Service) CancelSchedule(ctx context.Context, ticketID, operatorID int64, operatorRole string) (*model.Ticket, error) {
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
	_, err = s.client.Ticket.UpdateOneID(int(ticketID)).
		SetStatus(string(model.TicketStatusApproved)).
		ClearScheduledAt().
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("取消定时执行失败: %w", err)
	}

	s.auditSvc.Write(ctx, auditlog.Record{
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
func (s *Service) BatchApprove(ctx context.Context, ticketIDs []int64, reviewerID int64, reviewerRole, comment string) (*BatchResponse, error) {
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
func (s *Service) BatchReject(ctx context.Context, ticketIDs []int64, reviewerID int64, reviewerRole, reason string) (*BatchResponse, error) {
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

// checkMongoPermission validates that the user has permission to perform the MongoDB operation.
// It parses the MongoDB command body, extracts the collection and operation type,
// and checks collection-level permission via Casbin.
func (s *Service) checkMongoPermission(ctx context.Context, role string, datasourceID int64, sqlContent string) error {
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

	act := security.MongoOpToCasbinAct(mongoResult.Operation)
	dom := authz.DatasourceDomain(datasourceID)

	allowed, err := s.permSvc.Enforce(role, dom, mongoResult.Collection, act)
	if err != nil {
		return fmt.Errorf("MongoDB权限校验失败: %w", err)
	}
	if !allowed {
		return fmt.Errorf("没有集合 %s 的 %s 权限", mongoResult.Collection, act)
	}

	return nil
}
