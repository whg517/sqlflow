package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/db/ent"
	entApprovalPolicy "github.com/whg517/sqlflow/internal/db/ent/approvalpolicy"
	entUser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/model"
)

// TogglePolicy enables or disables a policy.
func (e *ApprovalEngine) TogglePolicy(ctx context.Context, id int64) (*model.ApprovalPolicy, error) {
	p, err := e.client.ApprovalPolicy.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("策略不存在: %d", id)
		}
		return nil, fmt.Errorf("查询策略失败: %w", err)
	}

	_, err = e.client.ApprovalPolicy.UpdateOneID(int(id)).
		SetEnabled(!p.Enabled).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("切换策略状态失败: %w", err)
	}
	return e.GetPolicy(ctx, id)
}

// ReorderPolicies updates priorities for multiple policies in batch.
// priorities is a map of policy ID -> new priority.
func (e *ApprovalEngine) ReorderPolicies(ctx context.Context, priorities map[int64]int) error {
	if len(priorities) == 0 {
		return fmt.Errorf("优先级列表不能为空")
	}

	for id, priority := range priorities {
		_, err := e.client.ApprovalPolicy.UpdateOneID(int(id)).
			SetPriority(priority).
			SetUpdatedAt(time.Now()).
			Save(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return fmt.Errorf("策略不存在: %d", id)
			}
			return fmt.Errorf("更新策略优先级失败 (id=%d): %w", id, err)
		}
	}
	return nil
}

// ListPoliciesFiltered returns policies with optional filters.
// enabledFilter: "" = all, "true" = enabled only, "false" = disabled only
// page/pageSize: pagination (1-based page)
// sort: "priority" (default) or "created_at"
func (e *ApprovalEngine) ListPoliciesFiltered(ctx context.Context, enabledFilter, sort string, page, pageSize int) ([]model.ApprovalPolicy, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := e.client.ApprovalPolicy.Query()

	switch enabledFilter {
	case "true":
		query = query.Where(entApprovalPolicy.EnabledEQ(true))
	case "false":
		query = query.Where(entApprovalPolicy.EnabledEQ(false))
	}

	// Count total
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("查询策略总数失败: %w", err)
	}

	// Sort
	switch sort {
	case "created_at":
		query = query.Order(ent.Desc(entApprovalPolicy.FieldCreatedAt))
	default:
		query = query.Order(ent.Desc(entApprovalPolicy.FieldPriority), ent.Asc(entApprovalPolicy.FieldID))
	}

	// Paginate
	offset := (page - 1) * pageSize
	query = query.Offset(offset).Limit(pageSize)

	policies, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("查询策略列表失败: %w", err)
	}

	out := make([]model.ApprovalPolicy, 0, len(policies))
	for _, p := range policies {
		out = append(out, *entApprovalPolicyToModel(p))
	}
	return out, total, nil
}

// ApproverInfo holds approver display info for the policy management UI.
type ApproverInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// GetApprovers returns all users who can serve as approvers (admin + dba roles),
// plus virtual role entries for chain configuration.
func (e *ApprovalEngine) GetApprovers(ctx context.Context) ([]ApproverInfo, error) {
	users, err := e.client.User.Query().
		Where(entUser.RoleIn("admin", "dba")).
		Order(ent.Asc(entUser.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询审批人列表失败: %w", err)
	}

	out := make([]ApproverInfo, 0, len(users)+2)

	// Add role-based entries first (for chain config reference)
	out = append(out, ApproverInfo{ID: 0, Username: "admin", Role: "admin"})
	out = append(out, ApproverInfo{ID: 0, Username: "dba", Role: "dba"})

	for _, u := range users {
		out = append(out, ApproverInfo{
			ID:       int64(u.ID),
			Username: u.Username,
			Role:     u.Role,
		})
	}

	return out, nil
}

// ApprovalStageDetail represents one stage in the approval chain for a ticket.
type ApprovalStageDetail struct {
	Stage        int    `json:"stage"`
	Role         string `json:"role"`
	AutoSkipSame bool   `json:"auto_skip_same_submitter"`
	Status       string `json:"status"` // "pending", "approved", "rejected", "skipped", "auto_approved"
	ApproverID   int64  `json:"approver_id,omitempty"`
	ApproverName string `json:"approver_name,omitempty"`
	Comment      string `json:"comment,omitempty"`
	Action       string `json:"action,omitempty"`
	AutoApproved bool   `json:"auto_approved,omitempty"`
}

// ApprovalChainDetail holds the full approval chain for a ticket.
type ApprovalChainDetail struct {
	TicketID     int64                 `json:"ticket_id"`
	PolicyID     int64                 `json:"policy_id"`
	PolicyName   string                `json:"policy_name"`
	AutoApproved bool                  `json:"auto_approved"`
	CurrentStage int                   `json:"current_stage"`
	TotalStages  int                   `json:"total_stages"`
	Stages       []ApprovalStageDetail `json:"stages"`
}

// GetApprovalChainDetail returns the full approval chain detail for a ticket.
func (e *ApprovalEngine) GetApprovalChainDetail(ctx context.Context, ticketID int64) (*ApprovalChainDetail, error) {
	tk, err := e.client.Ticket.Get(ctx, int(ticketID))
	if err != nil {
		return nil, fmt.Errorf("查询工单失败: %w", err)
	}

	detail := &ApprovalChainDetail{
		TicketID:     ticketID,
		AutoApproved: tk.AutoApproved,
		CurrentStage: tk.CurrentStage,
		TotalStages:  tk.TotalStages,
	}

	var policyID int64
	if tk.PolicyID != nil {
		policyID = *tk.PolicyID
	}
	detail.PolicyID = policyID

	if policyID == 0 {
		return detail, nil
	}

	policy, err := e.GetPolicy(ctx, policyID)
	if err != nil {
		return detail, nil
	}
	detail.PolicyName = policy.Name

	var chain []ApprovalChainStage
	if err := json.Unmarshal([]byte(policy.ApprovalChain), &chain); err != nil {
		return detail, nil
	}

	// Get approval records for this ticket
	records, _ := e.GetApprovalHistory(ctx, ticketID)

	stages := make([]ApprovalStageDetail, 0, len(chain))
	for i, stage := range chain {
		sd := ApprovalStageDetail{
			Stage:        i + 1,
			Role:         stage.Role,
			AutoSkipSame: stage.AutoSkipSameSubmitter,
			Status:       "pending",
		}

		// Find matching record
		for _, r := range records {
			if r.Stage == i+1 {
				sd.Action = r.Action
				sd.ApproverID = r.ApproverID
				sd.ApproverName = r.ApproverName
				sd.Comment = r.Comment
				sd.AutoApproved = r.AutoApproved
				switch r.Action {
				case "approved":
					sd.Status = "approved"
				case "rejected":
					sd.Status = "rejected"
				case "auto_approved":
					sd.Status = "auto_approved"
				}
				break
			}
		}

		// Determine stage status based on ticket progress
		if tk.AutoApproved {
			if sd.Status == "pending" {
				sd.Status = "skipped"
			}
		} else if i+1 < tk.CurrentStage && sd.Status == "pending" {
			sd.Status = "approved"
		}

		stages = append(stages, sd)
	}

	detail.Stages = stages
	return detail, nil
}
