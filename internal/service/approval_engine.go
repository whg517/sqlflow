package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// ApprovalEngine manages configurable approval policies.
type ApprovalEngine struct {
	db        *sql.DB
	notifySvc *NotifyService
}

// NewApprovalEngine creates a new ApprovalEngine.
func NewApprovalEngine(db *sql.DB) *ApprovalEngine {
	return &ApprovalEngine{db: db}
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
	result, err := e.db.ExecContext(ctx,
		`INSERT INTO approval_policies (name, description, enabled, priority, conditions, approval_chain, auto_approve_enabled, auto_approve_reason, is_default, created_at, updated_at)
		 VALUES (?, ?, TRUE, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, description, priority, conditions, approvalChain, autoApproveEnabled, autoApproveReason, isDefault, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return nil, fmt.Errorf("策略名称已存在: %s", name)
		}
		return nil, fmt.Errorf("创建策略失败: %w", err)
	}

	id, _ := result.LastInsertId()
	return &model.ApprovalPolicy{
		ID:                 id,
		Name:               name,
		Description:        description,
		Enabled:            true,
		Priority:           priority,
		Conditions:         conditions,
		ApprovalChain:      approvalChain,
		AutoApproveEnabled: autoApproveEnabled,
		AutoApproveReason:  autoApproveReason,
		IsDefault:          isDefault,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

// GetPolicy retrieves a policy by ID.
func (e *ApprovalEngine) GetPolicy(ctx context.Context, id int64) (*model.ApprovalPolicy, error) {
	p := &model.ApprovalPolicy{}
	err := e.db.QueryRowContext(ctx,
		`SELECT id, name, description, enabled, priority, conditions, approval_chain, auto_approve_enabled, auto_approve_reason, is_default, created_at, updated_at
		 FROM approval_policies WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Enabled, &p.Priority, &p.Conditions, &p.ApprovalChain, &p.AutoApproveEnabled, &p.AutoApproveReason, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("策略不存在: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询策略失败: %w", err)
	}
	return p, nil
}

// ListPolicies lists all policies ordered by priority.
func (e *ApprovalEngine) ListPolicies(ctx context.Context) ([]model.ApprovalPolicy, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT id, name, description, enabled, priority, conditions, approval_chain, auto_approve_enabled, auto_approve_reason, is_default, created_at, updated_at
		 FROM approval_policies ORDER BY priority DESC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询策略列表失败: %w", err)
	}
	defer rows.Close()

	var policies []model.ApprovalPolicy
	for rows.Next() {
		var p model.ApprovalPolicy
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Enabled, &p.Priority, &p.Conditions, &p.ApprovalChain, &p.AutoApproveEnabled, &p.AutoApproveReason, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描策略失败: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, nil
}

// UpdatePolicy updates an existing policy.
func (e *ApprovalEngine) UpdatePolicy(ctx context.Context, id int64, name, description string, enabled bool, priority int, conditions, approvalChain string, autoApproveEnabled bool, autoApproveReason string, isDefault bool) (*model.ApprovalPolicy, error) {
	_, err := e.db.ExecContext(ctx,
		`UPDATE approval_policies SET name=?, description=?, enabled=?, priority=?, conditions=?, approval_chain=?, auto_approve_enabled=?, auto_approve_reason=?, is_default=?, updated_at=? WHERE id=?`,
		name, description, enabled, priority, conditions, approvalChain, autoApproveEnabled, autoApproveReason, isDefault, time.Now(), id,
	)
	if err != nil {
		return nil, fmt.Errorf("更新策略失败: %w", err)
	}
	return e.GetPolicy(ctx, id)
}

// DeletePolicy deletes a policy by ID.
func (e *ApprovalEngine) DeletePolicy(ctx context.Context, id int64) error {
	_, err := e.db.ExecContext(ctx, `DELETE FROM approval_policies WHERE id = ?`, id)
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
		_, err := e.db.ExecContext(ctx,
			`INSERT INTO approval_records (ticket_id, policy_id, stage, total_stages, approver_role, action, auto_approved, auto_reason, created_at)
			 VALUES (?, ?, 0, 0, '', 'auto_approved', TRUE, ?, ?)`,
			ticketID, policy.ID, result.AutoReason, now,
		)
		if err != nil {
			return nil, fmt.Errorf("写入自动审批记录失败: %w", err)
		}

		// Update ticket
		_, err = e.db.ExecContext(ctx,
			`UPDATE tickets SET policy_id=?, current_stage=0, total_stages=0, auto_approved=TRUE, auto_approve_reason=?, status='APPROVED', updated_at=? WHERE id=?`,
			policy.ID, result.AutoReason, now, ticketID,
		)
		if err != nil {
			return nil, fmt.Errorf("更新工单自动审批状态失败: %w", err)
		}

		return result, nil
	}

	// Not auto-approved: set up multi-stage approval
	now := time.Now()
	_, err := e.db.ExecContext(ctx,
		`UPDATE tickets SET policy_id=?, current_stage=1, total_stages=?, auto_approved=FALSE, status='PENDING_APPROVAL', updated_at=? WHERE id=?`,
		policy.ID, len(chain), now, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("更新工单审批阶段失败: %w", err)
	}

	// Notify approvers about pending approval
	if e.notifySvc != nil {
		t, _ := getTicketForNotify(ctx, e.db, ticketID)
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
	var currentStage, totalStages int
	var policyID int64
	err := e.db.QueryRowContext(ctx,
		`SELECT current_stage, total_stages, policy_id FROM tickets WHERE id = ?`, ticketID,
	).Scan(&currentStage, &totalStages, &policyID)
	if err != nil {
		return nil, fmt.Errorf("查询工单失败: %w", err)
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
	recordResult, err := e.db.ExecContext(ctx,
		`INSERT INTO approval_records (ticket_id, policy_id, stage, total_stages, approver_role, approver_id, action, comment, auto_approved, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, FALSE, ?)`,
		ticketID, policyID, currentStage, totalStages, expectedStage.Role, approverID, action, comment, now,
	)
	if err != nil {
		return nil, fmt.Errorf("写入审批记录失败: %w", err)
	}
	recordID, _ := recordResult.LastInsertId()

	if action == "rejected" {
		// Reject the ticket
		_, err = e.db.ExecContext(ctx,
			`UPDATE tickets SET status='REJECTED', current_stage=0, total_stages=0, reviewer_id=?, review_comment=?, updated_at=? WHERE id=?`,
			approverID, comment, now, ticketID,
		)
		if err != nil {
		return nil, fmt.Errorf("拒绝工单失败: %w", err)
		}

		// Notify submitter about rejection
		if e.notifySvc != nil {
			t, _ := getTicketForNotify(ctx, e.db, ticketID)
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
			_, err = e.db.ExecContext(ctx,
				`UPDATE tickets SET status='APPROVED', current_stage=?, total_stages=?, reviewer_id=?, review_comment=?, updated_at=? WHERE id=?`,
				nextStage, totalStages, approverID, comment, now, ticketID,
			)
		} else {
			// Advance to next stage
			_, err = e.db.ExecContext(ctx,
				`UPDATE tickets SET current_stage=?, updated_at=? WHERE id=?`,
				nextStage, now, ticketID,
			)
		}
		if err != nil {
			return nil, fmt.Errorf("更新工单审批状态失败: %w", err)
		}

		// Notify when all stages approved
		if nextStage > totalStages && e.notifySvc != nil {
			t, _ := getTicketForNotify(ctx, e.db, ticketID)
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
	rows, err := e.db.QueryContext(ctx,
		`SELECT id, ticket_id, policy_id, stage, total_stages, approver_role, approver_id, approver_name, action, comment, auto_approved, auto_reason, created_at
		 FROM approval_records WHERE ticket_id = ? ORDER BY stage ASC, id ASC`, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询审批历史失败: %w", err)
	}
	defer rows.Close()

	var records []model.ApprovalRecord
	for rows.Next() {
		var r model.ApprovalRecord
		if err := rows.Scan(&r.ID, &r.TicketID, &r.PolicyID, &r.Stage, &r.TotalStages, &r.ApproverRole, &r.ApproverID, &r.ApproverName, &r.Action, &r.Comment, &r.AutoApproved, &r.AutoReason, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描审批记录失败: %w", err)
		}
		records = append(records, r)
	}
	return records, nil
}

// EnsureDefaultPolicy creates a default policy if none exists.
func (e *ApprovalEngine) EnsureDefaultPolicy(ctx context.Context) error {
	var count int
	err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_policies WHERE is_default = TRUE`).Scan(&count)
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

// getTicketForNotify fetches minimal ticket fields needed for notification.
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
