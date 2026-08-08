package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entApprovalPolicy "github.com/whg517/sqlflow/internal/db/ent/approvalpolicy"
	entApprovalRecord "github.com/whg517/sqlflow/internal/db/ent/approvalrecord"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/notify"
)

// ErrApprovalStageConflict indicates the approval stage moved between the read
// and the write — another approver already decided this stage.
var ErrApprovalStageConflict = errors.New("该审批阶段已被其他审批人处理")

// pinnedSQLHash returns the hash of a ticket's current SQL body. Every
// transition into APPROVED must record it, so executeTicket can detect a
// statement that changed after the approval decision.
func (e *ApprovalEngine) pinnedSQLHash(ctx context.Context, ticketID int64) (string, error) {
	tk, err := e.client.Ticket.Get(ctx, int(ticketID))
	if err != nil {
		return "", fmt.Errorf("查询工单失败: %w", err)
	}
	return sha256Hash(tk.SQLContent), nil
}

// ApprovalEngine manages configurable approval policies.
type ApprovalEngine struct {
	database  *db.DB
	client    *ent.Client
	notifySvc *notify.Service
}

// NewApprovalEngine creates an ApprovalEngine.
//
// notifySvc may be nil, in which case policy decisions are applied silently.
func NewApprovalEngine(database *db.DB, notifySvc *notify.Service) *ApprovalEngine {
	return &ApprovalEngine{database: database, client: database.Client(), notifySvc: notifySvc}
}

// PolicyCondition defines matching criteria for a policy.
type PolicyCondition struct {
	RiskLevels   []string `json:"risk_levels,omitempty"`
	SQLTypes     []string `json:"sql_types,omitempty"`
	Environments []string `json:"environments,omitempty"`
	Databases    []string `json:"databases,omitempty"`
}

// ApprovalChainStage defines a single stage in the approval chain.
type ApprovalChainStage struct {
	Role                  string `json:"role"`
	AutoSkipSameSubmitter bool   `json:"auto_skip_same_submitter"`
}

// --- security.Policy CRUD ---

// CreatePolicy creates a new approval policy.
func (e *ApprovalEngine) CreatePolicy(ctx context.Context, name, description string, conditions, approvalChain string, autoApproveEnabled bool, autoApproveReason string, isDefault bool, priority int) (*model.ApprovalPolicy, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("策略名称不能为空")
	}

	now := time.Now()
	saved, err := e.client.ApprovalPolicy.Create().
		SetName(name).
		SetDescription(description).
		SetEnabled(true).
		SetPriority(priority).
		SetConditions(conditions).
		SetApprovalChain(approvalChain).
		SetAutoApproveEnabled(autoApproveEnabled).
		SetAutoApproveReason(autoApproveReason).
		SetIsDefault(isDefault).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return nil, fmt.Errorf("策略名称已存在: %s", name)
		}
		return nil, fmt.Errorf("创建策略失败: %w", err)
	}

	return &model.ApprovalPolicy{
		ID:                 int64(saved.ID),
		Name:               saved.Name,
		Description:        strPtrValue(saved.Description),
		Enabled:            saved.Enabled,
		Priority:           saved.Priority,
		Conditions:         saved.Conditions,
		ApprovalChain:      saved.ApprovalChain,
		AutoApproveEnabled: saved.AutoApproveEnabled,
		AutoApproveReason:  strPtrValue(saved.AutoApproveReason),
		IsDefault:          saved.IsDefault,
		CreatedAt:          saved.CreatedAt,
		UpdatedAt:          saved.UpdatedAt,
	}, nil
}

// GetPolicy retrieves a policy by ID.
func (e *ApprovalEngine) GetPolicy(ctx context.Context, id int64) (*model.ApprovalPolicy, error) {
	p, err := e.client.ApprovalPolicy.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("策略不存在: %d", id)
		}
		return nil, fmt.Errorf("查询策略失败: %w", err)
	}
	return entApprovalPolicyToModel(p), nil
}

// ListPolicies lists all policies ordered by priority.
func (e *ApprovalEngine) ListPolicies(ctx context.Context) ([]model.ApprovalPolicy, error) {
	policies, err := e.client.ApprovalPolicy.Query().
		Order(ent.Desc(entApprovalPolicy.FieldPriority), ent.Asc(entApprovalPolicy.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询策略列表失败: %w", err)
	}

	out := make([]model.ApprovalPolicy, 0, len(policies))
	for _, p := range policies {
		out = append(out, *entApprovalPolicyToModel(p))
	}
	return out, nil
}

// UpdatePolicy updates an existing policy.
// PolicyUpdate names the fields a caller wants changed.
//
// Pointers, so that "not mentioned" and "set to the zero value" are different
// things. They were the same thing before, and every partial update in the
// admin screen wrote zeros over the rest of the policy: the enable toggle and
// the reorder arrows blanked the approval chain and were rejected outright, and
// the edit sheet — which never sends is_default — silently cleared the default
// policy on every save.
type PolicyUpdate struct {
	Name               *string
	Description        *string
	Enabled            *bool
	Priority           *int
	Conditions         *string
	ApprovalChain      *string
	AutoApproveEnabled *bool
	AutoApproveReason  *string
	IsDefault          *bool
}

// UpdatePolicy applies only the fields the caller named.
func (e *ApprovalEngine) UpdatePolicy(ctx context.Context, id int64, upd PolicyUpdate) (*model.ApprovalPolicy, error) {
	q := e.client.ApprovalPolicy.UpdateOneID(int(id)).SetUpdatedAt(time.Now())
	if upd.Name != nil {
		q = q.SetName(*upd.Name)
	}
	if upd.Description != nil {
		q = q.SetDescription(*upd.Description)
	}
	if upd.Enabled != nil {
		q = q.SetEnabled(*upd.Enabled)
	}
	if upd.Priority != nil {
		q = q.SetPriority(*upd.Priority)
	}
	if upd.Conditions != nil {
		q = q.SetConditions(*upd.Conditions)
	}
	if upd.ApprovalChain != nil {
		q = q.SetApprovalChain(*upd.ApprovalChain)
	}
	if upd.AutoApproveEnabled != nil {
		q = q.SetAutoApproveEnabled(*upd.AutoApproveEnabled)
	}
	if upd.AutoApproveReason != nil {
		q = q.SetAutoApproveReason(*upd.AutoApproveReason)
	}
	if upd.IsDefault != nil {
		q = q.SetIsDefault(*upd.IsDefault)
	}

	if _, err := q.Save(ctx); err != nil {
		return nil, fmt.Errorf("更新策略失败: %w", err)
	}
	return e.GetPolicy(ctx, id)
}

// DeletePolicy deletes a policy by ID.
func (e *ApprovalEngine) DeletePolicy(ctx context.Context, id int64) error {
	err := e.client.ApprovalPolicy.DeleteOneID(int(id)).Exec(ctx)
	if err != nil {
		return fmt.Errorf("删除策略失败: %w", err)
	}
	return nil
}

// --- security.Policy Matching ---

// MatchPolicy finds the best matching policy for a ticket.
func (e *ApprovalEngine) MatchPolicy(ctx context.Context, ticket *model.Ticket) (*model.ApprovalPolicy, error) {
	policies, err := e.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}

	for i := range policies {
		p := &policies[i]
		if !p.Enabled {
			continue
		}
		if e.policyMatches(p, ticket) {
			return p, nil
		}
	}

	// Return default policy if exists
	for i := range policies {
		p := &policies[i]
		if p.IsDefault {
			return p, nil
		}
	}

	return nil, fmt.Errorf("无匹配审批策略")
}

func (e *ApprovalEngine) policyMatches(policy *model.ApprovalPolicy, ticket *model.Ticket) bool {
	var cond PolicyCondition
	if err := json.Unmarshal([]byte(policy.Conditions), &cond); err != nil {
		log.Printf("approval_engine: parse conditions for policy %d: %v", policy.ID, err)
		return false
	}

	// Empty conditions = match all
	if len(cond.RiskLevels) == 0 && len(cond.SQLTypes) == 0 && len(cond.Environments) == 0 && len(cond.Databases) == 0 {
		return true
	}

	// Risk level match
	if len(cond.RiskLevels) > 0 && !containsAny(cond.RiskLevels, ticket.RiskLevel) {
		return false
	}

	// SQL type match — ticket.SQLType populated by sql_analyzer on creation
	if len(cond.SQLTypes) > 0 {
		sqlType := ticket.SQLType
		if sqlType == "" {
			sqlType = "OTHER"
		}
		if !containsAny(cond.SQLTypes, sqlType) {
			return false
		}
	}

	// Database match
	if len(cond.Databases) > 0 && !containsAny(cond.Databases, ticket.Database) {
		return false
	}

	return true
}

// --- Approval Execution ---

// ApplyPolicy applies a matched policy to a ticket, potentially auto-approving.
func (e *ApprovalEngine) ApplyPolicy(ctx context.Context, ticketID int64, policy *model.ApprovalPolicy, submitterID int64) (*ApprovalApplyResult, error) {
	var chain []ApprovalChainStage
	if err := json.Unmarshal([]byte(policy.ApprovalChain), &chain); err != nil {
		return nil, fmt.Errorf("解析审批链失败: %w", err)
	}

	result := &ApprovalApplyResult{
		PolicyID:     policy.ID,
		TotalStages:  len(chain),
		AutoApproved: false,
	}

	// Check auto-approve
	if policy.AutoApproveEnabled {
		result.AutoApproved = true
		result.AutoReason = policy.AutoApproveReason
		if result.AutoReason == "" {
			result.AutoReason = "策略自动审批"
		}

		sqlHash, err := e.pinnedSQLHash(ctx, ticketID)
		if err != nil {
			return nil, err
		}

		// Move the ticket first, guarding on the state it is leaving.
		//
		// A bare UpdateOneID here would be a read-modify-write: two concurrent
		// applications — a resubmission racing a retry, say — would both write
		// an approval record and both claim to have approved. Invariant 2 exists
		// because that shape once let four concurrent approvals through.
		now := time.Now()
		moved, err := applyTransition(ctx, e.client.Ticket, ticketID, now, transition{
			From: []model.TicketStatus{model.TicketStatusSubmitted},
			To:   model.TicketStatusApproved,
			Extra: func(u *ent.TicketUpdate) *ent.TicketUpdate {
				return u.SetPolicyID(policy.ID).
					SetCurrentStage(0).
					SetTotalStages(0).
					SetAutoApproved(true).
					SetAutoApproveReason(result.AutoReason).
					SetSQLHash(sqlHash)
			},
		})
		if err != nil {
			return nil, fmt.Errorf("更新工单自动审批状态失败: %w", err)
		}
		if !moved {
			// Someone else applied a policy to this ticket. Not an error, but
			// this call decided nothing, so it must not leave a record saying it
			// did.
			return result, nil
		}

		// The record is written only by the caller that actually moved the
		// ticket, so the count of approval records matches the number of
		// decisions taken.
		if _, err := e.client.ApprovalRecord.Create().
			SetTicketID(ticketID).
			SetPolicyID(policy.ID).
			SetStage(0).
			SetTotalStages(0).
			SetApproverRole("").
			SetAction("auto_approved").
			SetAutoApproved(true).
			SetAutoReason(result.AutoReason).
			SetCreatedAt(now).
			Save(ctx); err != nil {
			return nil, fmt.Errorf("写入自动审批记录失败: %w", err)
		}

		result.Applied = true
		return result, nil
	}

	// Not auto-approved: set up multi-stage approval, guarded the same way.
	now := time.Now()
	moved, err := applyTransition(ctx, e.client.Ticket, ticketID, now, transition{
		From: []model.TicketStatus{model.TicketStatusSubmitted},
		To:   model.TicketStatusPendingApproval,
		Extra: func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetPolicyID(policy.ID).
				SetCurrentStage(1).
				SetTotalStages(len(chain)).
				SetAutoApproved(false)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("更新工单审批阶段失败: %w", err)
	}
	if !moved {
		return result, nil
	}
	result.Applied = true

	// Notify approvers about pending approval
	if e.notifySvc != nil {
		t, _ := getTicketForNotify(ctx, e.client, ticketID)
		if t != nil {
			e.notifySvc.NotifyTicketPendingApproval(ctx, t)
		}
	}

	return result, nil
}

// ApprovalApplyResult holds the result of applying a policy.
type ApprovalApplyResult struct {
	PolicyID     int64  `json:"policy_id"`
	TotalStages  int    `json:"total_stages"`
	AutoApproved bool   `json:"auto_approved"`
	AutoReason   string `json:"auto_reason,omitempty"`

	// Applied reports whether this call performed the transition.
	//
	// A policy application that finds the ticket already moved is not an error —
	// a resubmission or a retry can legitimately race — but the caller must be
	// able to tell the two apart, or a second caller would write a duplicate
	// approval record for a decision it did not make.
	Applied bool `json:"applied"`
}

// ProcessApproval handles an approval action at the current stage.
func (e *ApprovalEngine) ProcessApproval(ctx context.Context, ticketID, approverID int64, approverRole, action, comment string) (*model.ApprovalRecord, error) {
	if action != "approved" && action != "rejected" {
		return nil, fmt.Errorf("无效的审批动作: %s", action)
	}

	// Get current ticket state
	tk, err := e.client.Ticket.Get(ctx, int(ticketID))
	if err != nil {
		return nil, fmt.Errorf("查询工单失败: %w", err)
	}
	// The engine is a decision door too, and it had no self-approval check
	// either. Checking here rather than only in the Service wrappers is what
	// makes it hold for /engine-approve, which reaches this directly.
	if tk.SubmitterID == approverID {
		return nil, ErrSelfApproval
	}

	currentStage := tk.CurrentStage
	totalStages := tk.TotalStages
	var policyID int64
	if tk.PolicyID != nil {
		policyID = *tk.PolicyID
	}

	if currentStage == 0 && totalStages == 0 {
		return nil, fmt.Errorf("工单不在审批流程中")
	}

	// Get the chain for the current stage's expected role
	policy, err := e.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}

	var chain []ApprovalChainStage
	if err := json.Unmarshal([]byte(policy.ApprovalChain), &chain); err != nil {
		return nil, fmt.Errorf("解析审批链失败: %w", err)
	}

	if currentStage < 1 || currentStage > len(chain) {
		return nil, fmt.Errorf("无效的审批阶段: %d", currentStage)
	}

	expectedStage := chain[currentStage-1]
	if expectedStage.Role != "" && expectedStage.Role != approverRole && approverRole != "admin" {
		return nil, fmt.Errorf("当前阶段需要 %s 角色，你是 %s", expectedStage.Role, approverRole)
	}

	now := time.Now()
	nextStage := currentStage + 1

	// Move the ticket first, guarding on both the state it is leaving and the
	// stage this approver actually saw.
	//
	// Two approvers acting on the same stage would otherwise both write a record
	// and both advance the chain, turning one decision into two. The stage half
	// was already here; the status half was not, and its absence was the larger
	// hole — the predicate never mentioned status while the mutation wrote it,
	// and nothing reset current_stage when a ticket was cancelled or rejected.
	// A decided ticket therefore kept stage 1 and still matched, so it could be
	// walked to APPROVED after the fact. The approve branch writes a fresh
	// sql_hash too, so a ticket revived that way also passed the integrity check
	// the executor performs before it runs anything.
	tr := transition{
		From:    []model.TicketStatus{model.TicketStatusPendingApproval},
		AtStage: atStage(currentStage),
	}
	switch {
	case action == "rejected":
		tr.To = model.TicketStatusRejected
		tr.Extra = func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetCurrentStage(0).
				SetTotalStages(0).
				SetReviewerID(approverID).
				SetReviewComment(comment)
		}
	case nextStage > totalStages:
		// All stages done — approved. Pin the SQL body being approved so the
		// executor can refuse a statement that changed afterwards.
		tr.To = model.TicketStatusApproved
		tr.Extra = func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetCurrentStage(nextStage).
				SetTotalStages(totalStages).
				SetReviewerID(approverID).
				SetReviewComment(comment).
				SetSQLHash(sha256Hash(tk.SQLContent))
		}
	default:
		// An intermediate stage: the ticket stays PENDING_APPROVAL and only its
		// position in the chain moves.
		tr.To = model.TicketStatusPendingApproval
		tr.Extra = func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetCurrentStage(nextStage)
		}
	}

	// The move and the record it accounts for are one unit.
	//
	// They were two statements: the ticket advanced, then the approval record
	// was written. A failure in between left a decided ticket with no record of
	// who decided it, and the chain view reads those records — so the history
	// would show a stage nobody walked while the ticket sat approved. The
	// compare-and-swap keeps its meaning inside the transaction: a losing
	// caller still writes nothing.
	tx, err := e.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("开启审批事务失败: %w", err)
	}

	applied, err := applyTransition(ctx, tx.Ticket, ticketID, now, tr)
	if err != nil {
		return nil, rollbackOn(tx, fmt.Errorf("更新工单审批状态失败: %w", err))
	}
	if !applied {
		return nil, rollbackOn(tx, ErrApprovalStageConflict)
	}

	savedRecord, err := tx.ApprovalRecord.Create().
		SetTicketID(ticketID).
		SetPolicyID(policyID).
		SetStage(currentStage).
		SetTotalStages(totalStages).
		SetApproverRole(expectedStage.Role).
		SetApproverID(approverID).
		SetAction(action).
		SetComment(comment).
		SetAutoApproved(false).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, rollbackOn(tx, fmt.Errorf("写入审批记录失败: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交审批事务失败: %w", err)
	}
	recordID := int64(savedRecord.ID)

	if action == "rejected" {
		// Notify submitter about rejection
		if e.notifySvc != nil {
			t, _ := getTicketForNotify(ctx, e.client, ticketID)
			if t != nil {
				t.ReviewerID = approverID
				t.ReviewComment = comment
				e.notifySvc.NotifyTicketRejected(ctx, t)
			}
		}
	} else {
		// Notify when all stages approved
		if nextStage > totalStages && e.notifySvc != nil {
			t, _ := getTicketForNotify(ctx, e.client, ticketID)
			if t != nil {
				t.ReviewerID = approverID
				t.ReviewComment = comment
				e.notifySvc.NotifyTicketApproved(ctx, t)
			}
		}
	}

	return &model.ApprovalRecord{
		ID:           recordID,
		TicketID:     ticketID,
		PolicyID:     policyID,
		Stage:        currentStage,
		TotalStages:  totalStages,
		ApproverRole: expectedStage.Role,
		ApproverID:   approverID,
		Action:       action,
		Comment:      comment,
		CreatedAt:    now,
	}, nil
}

// GetApprovalHistory returns all approval records for a ticket.
func (e *ApprovalEngine) GetApprovalHistory(ctx context.Context, ticketID, userID int64, role string) ([]model.ApprovalRecord, error) {
	if err := e.mayReadTicket(ctx, ticketID, userID, role); err != nil {
		return nil, err
	}
	return e.approvalRecords(ctx, ticketID)
}

// approvalRecords reads the records without authorizing, for callers that have
// already crossed the boundary.
func (e *ApprovalEngine) approvalRecords(ctx context.Context, ticketID int64) ([]model.ApprovalRecord, error) {

	records, err := e.client.ApprovalRecord.Query().
		Where(entApprovalRecord.TicketIDEQ(ticketID)).
		Order(ent.Asc(entApprovalRecord.FieldStage), ent.Asc(entApprovalRecord.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询审批历史失败: %w", err)
	}

	out := make([]model.ApprovalRecord, 0, len(records))
	for _, r := range records {
		mr := model.ApprovalRecord{
			ID:           int64(r.ID),
			TicketID:     r.TicketID,
			PolicyID:     int64PtrValue(r.PolicyID),
			Stage:        r.Stage,
			TotalStages:  r.TotalStages,
			ApproverRole: r.ApproverRole,
			ApproverID:   int64PtrValue(r.ApproverID),
			ApproverName: strPtrValue(r.ApproverName),
			Action:       r.Action,
			Comment:      strPtrValue(r.Comment),
			AutoApproved: r.AutoApproved,
			AutoReason:   strPtrValue(r.AutoReason),
			CreatedAt:    r.CreatedAt,
		}
		out = append(out, mr)
	}
	return out, nil
}

// EnsureDefaultPolicy creates a default policy if none exists.
func (e *ApprovalEngine) EnsureDefaultPolicy(ctx context.Context) error {
	count, err := e.client.ApprovalPolicy.Query().
		Where(entApprovalPolicy.IsDefaultEQ(true)).
		Count(ctx)
	if err != nil {
		return fmt.Errorf("检查默认策略失败: %w", err)
	}
	if count > 0 {
		return nil
	}

	// Create default policy: all tickets require dba approval
	defaultConditions := `{}`
	defaultChain := `[{"role":"dba","auto_skip_same_submitter":true}]`
	_, err = e.CreatePolicy(ctx, "默认审批策略", "所有工单需 DBA 审批", defaultConditions, defaultChain, false, "", true, 0)
	return err
}

// containsAny checks if target is in the list (case-insensitive).
func containsAny(list []string, target string) bool {
	t := strings.ToLower(target)
	for _, s := range list {
		if strings.ToLower(s) == t {
			return true
		}
	}
	return false
}

// entApprovalPolicyToModel converts an ent ApprovalPolicy to a model ApprovalPolicy.
func entApprovalPolicyToModel(p *ent.ApprovalPolicy) *model.ApprovalPolicy {
	return &model.ApprovalPolicy{
		ID:                 int64(p.ID),
		Name:               p.Name,
		Description:        strPtrValue(p.Description),
		Enabled:            p.Enabled,
		Priority:           p.Priority,
		Conditions:         p.Conditions,
		ApprovalChain:      p.ApprovalChain,
		AutoApproveEnabled: p.AutoApproveEnabled,
		AutoApproveReason:  strPtrValue(p.AutoApproveReason),
		IsDefault:          p.IsDefault,
		CreatedAt:          p.CreatedAt,
		UpdatedAt:          p.UpdatedAt,
	}
}

// getTicketForNotify fetches the ticket a notification describes.
func getTicketForNotify(ctx context.Context, client *ent.Client, ticketID int64) (*model.Ticket, error) {
	t, err := client.Ticket.Get(ctx, int(ticketID))
	if err != nil {
		return nil, err
	}
	return entTicketToModel(t), nil
}

// strPtrValue dereferences a string pointer, returning "" for nil.
func strPtrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// int64PtrValue dereferences an int64 pointer, returning 0 for nil.
func int64PtrValue(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// mayReadTicket applies the ticket's own read boundary to what hangs off it.
//
// The chain and its history describe who decided a change and why, and they
// were readable by anyone authenticated while the ticket itself was not. The
// rule is GetTicketForActor's, restated here rather than imported because the
// engine has no Service — a cross-reference the other direction would be a
// cycle.
func (e *ApprovalEngine) mayReadTicket(ctx context.Context, ticketID, userID int64, role string) error {
	if role == "admin" || role == "dba" {
		return nil
	}
	tk, err := e.client.Ticket.Get(ctx, int(ticketID))
	if err != nil {
		return fmt.Errorf("查询工单失败: %w", err)
	}
	if tk.SubmitterID == userID || tk.ReviewerID == userID {
		return nil
	}
	return ErrNoPermission
}
