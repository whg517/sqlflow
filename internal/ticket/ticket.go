package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/db/ent/predicate"
	entTicket "github.com/whg517/sqlflow/internal/db/ent/ticket"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/notify"
	"github.com/whg517/sqlflow/internal/ops"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"
)

var (
	// ErrTicketNotFound indicates the ticket does not exist.
	ErrTicketNotFound = errors.New("工单不存在")
	// ErrInvalidStatusTransition indicates an invalid state transition.
	ErrInvalidStatusTransition = errors.New("无效的工单状态变更")
	// ErrTicketAlreadyProcessed indicates the ticket has already been processed.
	// ErrSelfApproval means the actor deciding this ticket is the one who
	// submitted it. Separating author from approver is the platform's reason for
	// existing, so it is refused on every decision path rather than left to a
	// policy that might not be configured.
	ErrSelfApproval = errors.New("不能审批自己提交的工单")
	// ErrTicketExecutionConflict means another actor moved the ticket out of
	// EXECUTING while this execution was running — the reclaim sweep, in
	// practice. The outcome of this run was not recorded.
	ErrTicketExecutionConflict = errors.New("工单已被其他操作改变状态，本次执行结果未被记录")
	ErrTicketAlreadyProcessed  = errors.New("工单已处理，无法重复操作")
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
	// ErrTooManyStatements bounds one ticket, because each statement costs a
	// parse and nothing else stops a pasted schema dump.
	ErrTooManyStatements = errors.New("工单语句数量超出上限")
	// ErrTicketSQLUnanalyzable indicates the body could not be decomposed into
	// statements, so nothing can be said about what it would do. Refusing is the
	// only honest answer: a body that cannot be read has not been shown to be
	// safe.
	ErrTicketSQLUnanalyzable = errors.New("无法解析工单语句，请检查 SQL 是否完整")
	// ErrTicketSQLRequired indicates the SQL content is required.
	ErrTicketSQLRequired = errors.New("SQL内容不能为空")
	// ErrTicketDatasourceRequired indicates the datasource is required.
	ErrTicketDatasourceRequired = errors.New("数据源不能为空")

	// ErrTicketDatasourceNotFound is returned when the referenced datasource is
	// gone. Reading the type made this reachable: nothing used to verify the
	// datasource existed, so a ticket could be filed against any integer.
	ErrTicketDatasourceNotFound = errors.New("数据源不存在")
	// ErrTicketDatasourceDisabled mirrors the query path: a datasource an
	// operator took out of service should not accumulate queued changes.
	ErrTicketDatasourceDisabled = errors.New("数据源已禁用")
	// ErrTicketInternalDatasource keeps the platform's own store admin-only,
	// which ExecuteQuery already did and this path did not.
	ErrTicketInternalDatasource = errors.New("SQLFlow 元数据库仅允许管理员提交变更")
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
	// ErrApprovalEngineUnavailable means a ticket carries an approval chain but
	// the engine that walks it was never wired, so no one can decide it. Failing
	// is the only honest answer: approving without the engine would skip the
	// chain, which is the defect this route was changed to close.
	ErrApprovalEngineUnavailable = errors.New("审批引擎不可用，无法处理分阶段审批")
)

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
// entTicketToModel converts a generated ticket to the API shape.
//
// It replaces a hand-kept column list and a positional scanner that had to stay
// in the same order: the list was maintained in four places before it was
// consolidated, and a mismatch between the two showed up as a scan error at
// runtime rather than a compile error.
func entTicketToModel(t *ent.Ticket) *model.Ticket {
	return &model.Ticket{
		ID:             int64(t.ID),
		SubmitterID:    t.SubmitterID,
		DatasourceID:   t.DatasourceID,
		Database:       t.Database,
		SQLContent:     t.SQLContent,
		SQLSummary:     t.SQLSummary,
		DBType:         t.DbType,
		ChangeReason:   t.ChangeReason,
		Status:         model.TicketStatus(t.Status),
		RiskLevel:      t.RiskLevel,
		AIReviewResult: t.AiReviewResult,
		SQLType:        t.SQLType,
		AffectedTables: t.AffectedTables,
		ReviewerID:     t.ReviewerID,
		ReviewComment:  t.ReviewComment,
		SQLHash:        t.SQLHash,
		ScheduledAt:    t.ScheduledAt,
		ExecutedAt:     t.ExecutedAt,
		Revision:       t.Revision,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,

		// The approval chain's position. These were declared on the model and
		// on the wire but never filled in, so every read reported stage 0 of 0
		// no matter what the row said — which is precisely why ApproveTicket
		// could not tell a two-stage ticket from an unstaged one.
		CurrentStage: t.CurrentStage,
		TotalStages:  t.TotalStages,
		PolicyID:     derefInt64(t.PolicyID),
	}
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
// Neither the risk level nor the database type is a parameter, and for the same
// reason: both select which checks run. Risk drives approval-policy matching
// including auto-approval, and the type selects the MongoDB collection-level
// check — a submitter who claimed "mysql" for a MongoDB datasource, or who
// simply omitted the field and took the old "mysql" default, skipped that check
// and got the ticket into the approval queue. The server reads the type from
// the datasource row, which is the only thing that actually knows it.
func (s *Service) CreateTicket(ctx context.Context, submitterID int64, submitterRole string, datasourceID int64, database, sqlContent, changeReason string) (*model.Ticket, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrTicketSQLRequired
	}
	if datasourceID == 0 {
		return nil, ErrTicketDatasourceRequired
	}

	target, err := s.checkDatasourceAccess(ctx, datasourceID, submitterRole)
	if err != nil {
		return nil, err
	}

	// A ticket records the scope its change will land in, so it may only record
	// one the executor can reach. The field is free text on the submit form and
	// nothing checked it: a ticket could name prod while the datasource's
	// connection was pinned to staging, and the approver would read prod. The
	// scope is the datasource's; naming another is refused here rather than
	// discovered at execution.
	scope, err := datasource.ResolveQueryScope(target.database, database)
	if err != nil {
		return nil, err
	}

	summary := auditlog.Summarize(sqlContent)

	// Decompose the body and derive risk from every statement it contains.
	//
	// The grade used to come from the first keyword while the executor split on
	// ';', so prefixing a DROP with "SELECT 1;" moved the ticket from critical to
	// low and it still ran the DROP.
	plan, err := planTicketSQL(target.dbType, sqlContent)
	if err != nil {
		s.auditSvc.Write(ctx, auditlog.Record{
			UserID:       submitterID,
			Action:       "ticket_create_rejected",
			DatasourceID: datasourceID,
			Database:     scope,
			SQLContent:   sqlContent,
			SQLSummary:   summary,
			ErrorMessage: err.Error(),
		})
		return nil, fmt.Errorf("%w: %v", ErrTicketSQLUnanalyzable, err)
	}
	analysis := plan.Analysis
	tablesJSON := affectedTablesToJSON(analysis.AffectedTables)
	riskLevel := NewRiskEvaluator().Evaluate(analysis).Level

	now := time.Now()
	saved, err := s.client.Ticket.Create().
		SetSubmitterID(submitterID).
		SetDatasourceID(datasourceID).
		SetDatabase(scope).
		SetSQLContent(sqlContent).
		SetSQLSummary(summary).
		SetDbType(target.dbType).
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
		Database:       scope,
		SQLContent:     sqlContent,
		SQLSummary:     summary,
		DBType:         target.dbType,
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
	row, err := s.client.Ticket.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("获取工单失败: %w", err)
	}
	t := entTicketToModel(row)

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

	var preds []predicate.Ticket
	if currentRole != "admin" && currentRole != "dba" {
		// Non-governance roles can never widen this boundary with query params:
		// the submitter filter is appended here and the caller's value is
		// discarded, so a crafted submitter_id cannot reach another user's
		// tickets.
		preds = append(preds, entTicket.SubmitterIDEQ(currentUserID))
		submitterIDStr = ""
	}
	if status != "" {
		preds = append(preds, entTicket.StatusEQ(status))
	}
	if datasourceIDStr != "" {
		id, err := strconv.ParseInt(datasourceIDStr, 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("无效的数据源ID: %s", datasourceIDStr)
		}
		preds = append(preds, entTicket.DatasourceIDEQ(id))
	}
	if submitterIDStr != "" {
		id, err := strconv.ParseInt(submitterIDStr, 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("无效的提交人ID: %s", submitterIDStr)
		}
		preds = append(preds, entTicket.SubmitterIDEQ(id))
	}
	if riskLevel != "" {
		preds = append(preds, entTicket.RiskLevelEQ(riskLevel))
	}
	if keyword != "" {
		// ContainsFold renders as ILIKE and escapes the pattern itself, so a
		// keyword of "%" no longer matches every ticket.
		preds = append(preds, entTicket.Or(
			entTicket.SQLContentContainsFold(keyword),
			entTicket.ChangeReasonContainsFold(keyword),
		))
	}
	if scope == "mine" {
		preds = append(preds, entTicket.SubmitterIDEQ(currentUserID))
	}
	if scope == "pending" {
		preds = append(preds, entTicket.StatusIn(
			string(model.TicketStatusSubmitted),
			string(model.TicketStatusPendingApproval),
		))
	}

	q := s.client.Ticket.Query().Where(preds...)

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("统计工单失败: %w", err)
	}

	rows, err := q.
		Order(ent.Desc(entTicket.FieldCreatedAt)).
		Limit(p.PageSize).
		Offset(p.Offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("查询工单列表失败: %w", err)
	}

	tickets := make([]model.Ticket, 0, len(rows))
	for _, r := range rows {
		tickets = append(tickets, *entTicketToModel(r))
	}

	for i := range tickets {
		s.populateTicketNames(ctx, &tickets[i])
	}

	// Populate git links if service is available (batch query to avoid N+1)
	if s.gitSvc != nil {
		s.gitSvc.BatchPopulateGitLinks(ctx, tickets)
	}

	return tickets, int64(total), nil
}

// ApproveTicket approves a ticket. Only dba/admin can approve.
func (s *Service) ApproveTicket(ctx context.Context, ticketID, reviewerID int64, reviewerRole, comment string) (*model.Ticket, error) {
	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	if err := refuseSelfDecision(t, reviewerID); err != nil {
		return nil, err
	}

	if reviewerRole != "admin" && reviewerRole != "dba" {
		return nil, ErrNoPermission
	}

	if t.Status != model.TicketStatusPendingApproval {
		return nil, ErrInvalidStatusTransition
	}

	// A ticket governed by an approval chain is decided one stage at a time, and
	// this route does not get to shortcut it.
	//
	// It used to: this function names neither CurrentStage nor TotalStages, so a
	// dba facing the chain [security, dba] finalized it in a single call and
	// stage 1 never happened — while the chain view went on rendering two stages
	// as though both had been walked. This is the route the production approve
	// button calls, and RequireScope is a no-op for JWT sessions, so the role
	// check above was the only thing in front of it.
	if t.TotalStages > 0 {
		return s.approveThroughChain(ctx, t, reviewerID, reviewerRole, comment)
	}

	now := time.Now()
	// Pin the SQL body that is being approved, so executeTicket can refuse a
	// ticket whose statement changed afterwards.
	sqlHash := sha256Hash(t.SQLContent)
	swapped, err := applyTransition(ctx, s.client.Ticket, ticketID, now, transition{
		From: []model.TicketStatus{model.TicketStatusPendingApproval},
		To:   model.TicketStatusApproved,
		Extra: func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetReviewerID(reviewerID).SetReviewComment(comment).SetSQLHash(sqlHash)
		},
	})
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

// approveThroughChain records one stage's approval through the engine.
//
// The engine is the only path that knows which role the current stage expects
// and that guards on the stage the approver actually saw, so the decision is
// entirely its own. What stays here is what the engine has no collaborator for:
// the audit trail, and releasing the SLA clock once the chain is finished.
func (s *Service) approveThroughChain(ctx context.Context, t *model.Ticket, reviewerID int64, reviewerRole, comment string) (*model.Ticket, error) {
	if s.approvalEngine == nil {
		return nil, ErrApprovalEngineUnavailable
	}

	if _, err := s.approvalEngine.ProcessApproval(ctx, t.ID, reviewerID, reviewerRole, "approved", comment); err != nil {
		return nil, err
	}

	updated, err := s.GetTicket(ctx, t.ID)
	if err != nil {
		return nil, err
	}

	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     reviewerID,
		Action:     "ticket_approve",
		SQLContent: updated.SQLContent,
		SQLSummary: updated.SQLSummary,
		TicketID:   updated.ID,
	})

	// Only the final stage finishes the approval. An intermediate one leaves the
	// ticket waiting, and its deadline has to keep running with it. The engine
	// has already notified for whichever of the two happened.
	if updated.Status == model.TicketStatusApproved && s.slaSvc != nil {
		if err := s.slaSvc.ClearTicketSLA(ctx, updated.ID); err != nil {
			log.Printf("ticket: clear SLA on approve failed: %v", err)
		}
	}

	return updated, nil
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

	if err := refuseSelfDecision(t, reviewerID); err != nil {
		return nil, err
	}

	if reviewerRole != "admin" && reviewerRole != "dba" {
		return nil, ErrNoPermission
	}

	if t.Status != model.TicketStatusPendingApproval {
		return nil, ErrInvalidStatusTransition
	}

	// A rejection ends a ticket as finally as an approval does, so it goes
	// through the chain for the same reason ApproveTicket does.
	//
	// This route did not: any admin or dba could reject outright and clear
	// current_stage/total_stages, which made every stage role a chain declared
	// bypassable from one side. A two-stage chain [security, dba] was enforced
	// on the way to APPROVED and ignored on the way to REJECTED.
	if t.TotalStages > 0 {
		return s.rejectThroughChain(ctx, t, reviewerID, reviewerRole, reason)
	}

	now := time.Now()
	swapped, err := applyTransition(ctx, s.client.Ticket, ticketID, now, transition{
		From: []model.TicketStatus{model.TicketStatusPendingApproval},
		To:   model.TicketStatusRejected,
		Extra: func(u *ent.TicketUpdate) *ent.TicketUpdate {
			// Clearing the stage counters is what stops a decided ticket being
			// walked forward again: the staged route guards on current_stage,
			// and a rejected ticket that kept stage 1 still matched it.
			return u.SetReviewerID(reviewerID).SetReviewComment(reason).
				SetCurrentStage(0).SetTotalStages(0)
		},
	})
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

// cancellableStatuses are the states a ticket may be cancelled from.
//
// It is a slice rather than a set because it is used twice — once to reject the
// request with a useful error, and once as the compare-and-swap predicate that
// actually enforces it. Two lists would be two things to keep in step.
var cancellableStatuses = []model.TicketStatus{
	model.TicketStatusSubmitted,
	model.TicketStatusPendingApproval,
	model.TicketStatusApproved,
	model.TicketStatusScheduled,
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

	if !slices.Contains(cancellableStatuses, t.Status) {
		return nil, ErrTicketNotCancellable
	}

	// The same set guards the write, not just the read.
	//
	// This was a bare UpdateOneID: the status was checked against a value read
	// several round-trips earlier and then overwritten unconditionally. A cancel
	// racing an execute therefore returned 200 and wrote a ticket_cancel audit
	// record while the statement ran, and the row flipped back to DONE when the
	// execution finished — so the cancel was not merely late, it was lost, and
	// the user was told it had worked.
	now := time.Now()
	swapped, err := applyTransition(ctx, s.client.Ticket, ticketID, now, transition{
		From: cancellableStatuses,
		To:   model.TicketStatusCancelled,
		Extra: func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetReviewComment(reason).ClearScheduledAt().
				SetCurrentStage(0).SetTotalStages(0)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("取消工单失败: %w", err)
	}
	if !swapped {
		return nil, ErrTicketNotCancellable
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

	return s.executeTicket(ctx, t, operatorID, executableByOperator)
}

// executeTicket performs the actual SQL execution with hash verification,
// timeout control, and idempotent status transition.
//
// `from` is the set of states this caller may execute out of, and it differs by
// caller — see executableByOperator and executableByScheduler.
func (s *Service) executeTicket(ctx context.Context, t *model.Ticket, operatorID int64, from []model.TicketStatus) (*model.Ticket, error) {
	if s.dsSvc == nil || s.poolMgr == nil {
		return nil, ErrTicketExecUnavailable
	}

	// Idempotent: → EXECUTING, by compare-and-swap. Two schedulers reaching the
	// same due ticket both arrive here; the predicate is what makes exactly one
	// of them execute it.
	now := time.Now()
	applied, err := applyTransition(ctx, s.client.Ticket, t.ID, now, transition{
		From: from,
		To:   model.TicketStatusExecuting,
	})
	if err != nil {
		return nil, fmt.Errorf("状态更新失败: %w", err)
	}
	if !applied {
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

	execResults, execErr := s.executeSQL(execCtx, ds, t.DBType, t.Database, t.SQLContent)

	// Record execution results
	for i, r := range execResults {
		s.recordExecutionResult(ctx, t.ID, i, r)
	}

	if execErr != nil {
		return nil, s.failTicket(ctx, t, operatorID, sanitizeErrMsg(execErr.Error()))
	}

	// Success: EXECUTING → DONE.
	//
	// Guarded like every other move. As a bare write it would clobber whatever
	// the row said, which is how a cancel that landed mid-execution disappeared.
	now = time.Now()
	if err = s.concludeExecution(ctx, t.ID, now, model.TicketStatusDone,
		func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetExecutedAt(now).ClearScheduledAt()
		}); err != nil {
		return nil, err
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
	if err := s.concludeExecution(ctx, t.ID, now, model.TicketStatusFailed, nil); err != nil {
		return fmt.Errorf("%w (原始错误: %s)", err, errMsg)
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
	swapped, err := applyTransition(ctx, s.client.Ticket, ticketID, now, transition{
		From: []model.TicketStatus{model.TicketStatusApproved},
		To:   model.TicketStatusScheduled,
		Extra: func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetScheduledAt(scheduledAt)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("设置定时执行失败: %w", err)
	}
	if !swapped {
		return nil, ErrTicketNotSchedulable
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
	swapped, err := applyTransition(ctx, s.client.Ticket, ticketID, now, transition{
		From: []model.TicketStatus{model.TicketStatusScheduled},
		To:   model.TicketStatusApproved,
		Extra: func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.ClearScheduledAt()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("取消定时执行失败: %w", err)
	}
	if !swapped {
		// The scheduler picked it up between the read and the write. Saying the
		// schedule was cancelled would be a lie: the run is already under way.
		return nil, ErrTicketNotScheduled
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

// ticketTarget holds the server-owned facts about the datasource a ticket is
// filed against.
//
// Both are server-owned for the same reason: each one selects which checks run
// or where the change lands, so neither may come from the submitter.
type ticketTarget struct {
	// dbType selects the parser, the risk grade and the collection-level check.
	dbType string
	// database is the datasource's own database — the only scope its connection
	// can reach.
	database string
}

// checkDatasourceAccess decides whether this submitter may file against this
// datasource at all, and returns the target's server-owned facts.
//
// These are the gates ExecuteQuery already applies. The ticket path applied
// none of them, so a developer could file DROP TABLE tickets against SQLFlow's
// own metadata database — every ticket, approval record, audit row and
// encrypted credential lives there — and have it queued as an ordinary change.
// A gate on one entrance says nothing about another.
//
// What it deliberately does not check is whether the submitter already holds
// the permission the statement needs. A developer is read-only by default and
// files a ticket precisely because they lack it; that is the workflow. The
// approval step is the control for what may be done inside a datasource the
// submitter is allowed to use.
func (s *Service) checkDatasourceAccess(ctx context.Context, datasourceID int64, role string) (ticketTarget, error) {
	ds, err := s.client.DataSource.Get(ctx, int(datasourceID))
	if err != nil {
		if ent.IsNotFound(err) {
			return ticketTarget{}, ErrTicketDatasourceNotFound
		}
		return ticketTarget{}, fmt.Errorf("读取数据源失败: %w", err)
	}
	if ds.Status == "disabled" {
		return ticketTarget{}, ErrTicketDatasourceDisabled
	}
	if isInternalDatasource(ds.ExtraConfig) && role != "admin" {
		return ticketTarget{}, ErrTicketInternalDatasource
	}
	return ticketTarget{dbType: ds.Type, database: ds.Database}, nil
}

// isInternalDatasource reports whether the row is SQLFlow's own metadata
// database, which marks itself with system=true in extra_config.
func isInternalDatasource(extraConfig string) bool {
	if extraConfig == "" {
		return false
	}
	var extra struct {
		System bool `json:"system"`
	}
	return json.Unmarshal([]byte(extraConfig), &extra) == nil && extra.System
}

// concludeExecution moves a ticket out of EXECUTING and refuses to overwrite a
// decision another actor already made.
//
// Both ends of an execution used to discard applyTransition's first result with
// `_`, against its stated contract that a false return is a conflict rather
// than a success. The reachable case is the reclaim sweep: it moves a ticket
// whose lease expired to FAILED without coordinating with an executor that is
// still running, so when that executor finished its swap matched zero rows —
// and the caller was told DONE while the row said FAILED and the audit trail
// recorded a successful execution. Reporting the conflict is what keeps the
// answer, the row and the audit record talking about the same ticket.
func (s *Service) concludeExecution(
	ctx context.Context,
	ticketID int64,
	now time.Time,
	to model.TicketStatus,
	extra func(*ent.TicketUpdate) *ent.TicketUpdate,
) error {
	applied, err := applyTransition(ctx, s.client.Ticket, ticketID, now, transition{
		From:  []model.TicketStatus{model.TicketStatusExecuting},
		To:    to,
		Extra: extra,
	})
	if err != nil {
		return fmt.Errorf("更新工单状态失败: %w", err)
	}
	if !applied {
		return ErrTicketExecutionConflict
	}
	return nil
}

// refuseSelfDecision keeps the author of a change out of its approval.
//
// Neither decision path checked this. A submitter who also holds dba, or one
// whose own role happens to name a chain stage, could approve their own
// high-risk change — which is the single thing the ticket workflow exists to
// prevent. It is a rule rather than a policy knob because a policy that is not
// configured is a rule that does not apply.
func refuseSelfDecision(t *model.Ticket, actorID int64) error {
	if t.SubmitterID == actorID {
		return ErrSelfApproval
	}
	return nil
}

// rejectThroughChain hands a chained ticket's rejection to the engine, which is
// the only thing that knows which role owns the current stage.
func (s *Service) rejectThroughChain(ctx context.Context, t *model.Ticket, reviewerID int64, reviewerRole, reason string) (*model.Ticket, error) {
	if s.approvalEngine == nil {
		return nil, ErrApprovalEngineUnavailable
	}
	if _, err := s.approvalEngine.ProcessApproval(ctx, t.ID, reviewerID, reviewerRole, "rejected", reason); err != nil {
		return nil, err
	}

	updated, err := s.GetTicket(ctx, t.ID)
	if err != nil {
		return nil, err
	}

	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     reviewerID,
		Action:     "ticket_reject",
		SQLContent: updated.SQLContent,
		SQLSummary: updated.SQLSummary,
		TicketID:   updated.ID,
	})
	return updated, nil
}
