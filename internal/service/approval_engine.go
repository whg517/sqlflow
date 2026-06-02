package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entApprovalPolicy "github.com/whg517/sqlflow/internal/db/ent/approvalpolicy"
	entApprovalRecord "github.com/whg517/sqlflow/internal/db/ent/approvalrecord"
	"github.com/whg517/sqlflow/internal/model"
)

// ApprovalEngine manages configurable approval policies.
type ApprovalEngine struct {
	database  *db.DB
	client    *ent.Client
	notifySvc *NotifyService
}

// NewApprovalEngine creates a new ApprovalEngine.
func NewApprovalEngine(database *db.DB) *ApprovalEngine {
	return &ApprovalEngine{database: database, client: database.Client()}
}

// SetNotifyService sets the notification service for sending alerts.
func (e *ApprovalEngine) SetNotifyService(notifySvc *NotifyService) {
	e.notifySvc = notifySvc
}

// PolicyCondition defines matching criteria for a policy.
type PolicyCondition struct {
	RiskLevels  []string `json:"risk_levels,omitempty"`
	SQLTypes    []string `json:"sql_types,omitempty"`
	Environments []string `json:"environments,omitempty"`
	Databases   []string `json:"databases,omitempty"`
}

// ApprovalChainStage defines a single stage in the approval chain.
type ApprovalChainStage struct {
	Role                   string `json:"role"`
	AutoSkipSameSubmitter  bool   `json:"auto_skip_same_submitter"`
}

// --- Policy CRUD ---

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
func (e *ApprovalEngine) UpdatePolicy(ctx context.Context, id int64, name, description string, enabled bool, priority int, conditions, approvalChain string, autoApproveEnabled bool, autoApproveReason string, isDefault bool) (*model.ApprovalPolicy, error) {
	_, err := e.client.ApprovalPolicy.UpdateOneID(int(id)).
		SetName(name).
		SetDescription(description).
		SetEnabled(enabled).
		SetPriority(priority).
		SetConditions(conditions).
		SetApprovalChain(approvalChain).
		SetAutoApproveEnabled(autoApproveEnabled).
		SetAutoApproveReason(autoApproveReason).
		SetIsDefault(isDefault).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
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

// --- Policy Matching ---

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

		// Write auto-approve record
		now := time.Now()
		_, err := e.client.ApprovalRecord.Create().
			SetTicketID(ticketID).
			SetPolicyID(policy.ID).
			SetStage(0).
			SetTotalStages(0).
			SetApproverRole("").
			SetAction("auto_approved").
			SetAutoApproved(true).
			SetAutoReason(result.AutoReason).
			SetCreatedAt(now).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("写入自动审批记录失败: %w", err)
		}

		// Update ticket
		_, err = e.client.Ticket.UpdateOneID(int(ticketID)).
			SetPolicyID(policy.ID).
			SetCurrentStage(0).
			SetTotalStages(0).
			SetAutoApproved(true).
			SetAutoApproveReason(result.AutoReason).
			SetStatus("APPROVED").
			SetUpdatedAt(now).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("更新工单自动审批状态失败: %w", err)
		}

		return result, nil
	}

	// Not auto-approved: set up multi-stage approval
	now := time.Now()
	_, err := e.client.Ticket.UpdateOneID(int(ticketID)).
		SetPolicyID(policy.ID).
		SetCurrentStage(1).
		SetTotalStages(len(chain)).
		SetAutoApproved(false).
		SetStatus("PENDING_APPROVAL").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("更新工单审批阶段失败: %w", err)
	}

	// Notify approvers about pending approval
	if e.notifySvc != nil {
		t, _ := getTicketForNotify(ctx, e.database.DB, ticketID)
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

	// Write approval record
	now := time.Now()
	savedRecord, err := e.client.ApprovalRecord.Create().
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
		return nil, fmt.Errorf("写入审批记录失败: %w", err)
	}
	recordID := int64(savedRecord.ID)

	if action == "rejected" {
		// Reject the ticket
		_, err = e.client.Ticket.UpdateOneID(int(ticketID)).
			SetStatus("REJECTED").
			SetCurrentStage(0).
			SetTotalStages(0).
			SetReviewerID(approverID).
			SetReviewComment(comment).
			SetUpdatedAt(now).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("拒绝工单失败: %w", err)
		}

		// Notify submitter about rejection
		if e.notifySvc != nil {
			t, _ := getTicketForNotify(ctx, e.database.DB, ticketID)
			if t != nil {
				t.ReviewerID = approverID
				t.ReviewComment = comment
				e.notifySvc.NotifyTicketRejected(ctx, t)
			}
		}
	} else {
		// Approve: advance to next stage or fully approve
		nextStage := currentStage + 1
		if nextStage > totalStages {
			// All stages done — approved
			_, err = e.client.Ticket.UpdateOneID(int(ticketID)).
				SetStatus("APPROVED").
				SetCurrentStage(nextStage).
				SetTotalStages(totalStages).
				SetReviewerID(approverID).
				SetReviewComment(comment).
				SetUpdatedAt(now).
				Save(ctx)
		} else {
			// Advance to next stage
			_, err = e.client.Ticket.UpdateOneID(int(ticketID)).
				SetCurrentStage(nextStage).
				SetUpdatedAt(now).
				Save(ctx)
		}
		if err != nil {
			return nil, fmt.Errorf("更新工单审批状态失败: %w", err)
		}

		// Notify when all stages approved
		if nextStage > totalStages && e.notifySvc != nil {
			t, _ := getTicketForNotify(ctx, e.database.DB, ticketID)
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
func (e *ApprovalEngine) GetApprovalHistory(ctx context.Context, ticketID int64) ([]model.ApprovalRecord, error) {
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

// getTicketForNotify fetches minimal ticket fields needed for notification.
// RAW_SQL: helper for notifications, only needs a few fields — not worth ent query.
func getTicketForNotify(ctx context.Context, db *sql.DB, ticketID int64) (*model.Ticket, error) {
	t := &model.Ticket{ID: ticketID}
	err := db.QueryRowContext(ctx,
		`SELECT submitter_id, datasource_id, database, sql_summary, risk_level, status, created_at, updated_at
			 FROM tickets WHERE id = ?`, ticketID,
	).Scan(&t.SubmitterID, &t.DatasourceID, &t.Database, &t.SQLSummary, &t.RiskLevel, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
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
