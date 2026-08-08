package ticket

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/db/ent"
	entticketrevision "github.com/whg517/sqlflow/internal/db/ent/ticketrevision"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
)

// ResubmitTicket resubmits a rejected ticket with optional SQL and reason changes.
// The ticket must be in REJECTED status. Only the original submitter can resubmit.
func (s *Service) ResubmitTicket(ctx context.Context, ticketID, submitterID int64, submitterRole, sqlContent, changeReason string) (*model.Ticket, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrTicketSQLRequired
	}

	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	if t.Status != model.TicketStatusRejected {
		return nil, ErrTicketNotResubmittable
	}

	if t.SubmitterID != submitterID {
		return nil, ErrNoPermission
	}

	// A resubmission is a new proposal, not an edit: re-derive every
	// server-owned fact from the new SQL. Carrying the previous revision's
	// analysis forward would show approvers the impact of the SQL they already
	// rejected. Done before the transaction opens — these are pure functions,
	// but the platform pool holds a single connection, so no query may run
	// while the transaction is open.
	// A resubmission re-enters the queue, so it re-enters through the same
	// gates: the datasource may have been disabled, or the submitter's role
	// changed, since the ticket was first filed.
	target, err := s.checkDatasourceAccess(ctx, t.DatasourceID, submitterRole)
	if err != nil {
		return nil, err
	}
	plan, err := planTicketSQL(target.dbType, sqlContent)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTicketSQLUnanalyzable, err)
	}
	analysis := plan.Analysis
	riskLevel := NewRiskEvaluator().Evaluate(analysis).Level
	tablesJSON := affectedTablesToJSON(analysis.AffectedTables)

	now := time.Now()
	summary := auditlog.Summarize(sqlContent)
	newRevision := t.Revision + 1

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("重提工单失败: %w", err)
	}

	// Snapshot the rejected version and advance the ticket as one unit: a
	// snapshot without the update would leave a duplicate revision, and an
	// update without the snapshot would lose the rejected history.
	if _, err := tx.TicketRevision.Create().
		SetTicketID(t.ID).
		SetRevision(t.Revision).
		SetSQLContent(t.SQLContent).
		SetSQLSummary(t.SQLSummary).
		SetChangeReason(t.ChangeReason).
		SetRiskLevel(t.RiskLevel).
		SetAiReviewResult(t.AIReviewResult).
		SetReviewerID(t.ReviewerID).
		SetReviewComment(t.ReviewComment).
		SetStatus(string(t.Status)).
		SetCreatedAt(t.UpdatedAt).
		Save(ctx); err != nil {
		return nil, rollbackOn(tx, fmt.Errorf("保存历史版本失败: %w", err))
	}

	swapped, err := applyTransition(ctx, tx.Ticket, ticketID, now, transition{
		From: []model.TicketStatus{model.TicketStatusRejected},
		To:   model.TicketStatusSubmitted,
		Extra: func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetSQLContent(sqlContent).
				SetSQLSummary(summary).
				SetChangeReason(changeReason).
				SetRiskLevel(riskLevel).
				SetSQLType(analysis.SQLType).
				SetAffectedTables(tablesJSON).
				SetAiReviewResult("").
				SetReviewerID(0).
				SetReviewComment("").
				// The hash pins an approved SQL body; this revision has not
				// been approved, so carrying it over would let the integrity
				// check pass against a stale approval.
				SetSQLHash("").
				SetRevision(newRevision)
		},
	})
	if err != nil {
		return nil, rollbackOn(tx, fmt.Errorf("重提工单失败: %w", err))
	}
	if !swapped {
		// The ticket left REJECTED between the read and the write.
		return nil, rollbackOn(tx, ErrTicketNotResubmittable)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("重提工单失败: %w", err)
	}

	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     submitterID,
		Action:     "ticket_resubmit",
		SQLContent: sqlContent,
		SQLSummary: summary,
	})

	t.SQLContent = sqlContent
	t.SQLSummary = summary
	t.ChangeReason = changeReason
	t.Status = model.TicketStatusSubmitted
	t.RiskLevel = riskLevel
	t.SQLType = analysis.SQLType
	t.AffectedTables = tablesJSON
	t.AIReviewResult = ""
	t.ReviewerID = 0
	t.ReviewComment = ""
	t.SQLHash = ""
	t.Revision = newRevision
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	// Re-run policy matching on the new revision, outside the transaction:
	// ApplyPolicy writes, and the single-connection pool cannot serve it while
	// the transaction is open. Matching failure leaves the ticket in SUBMITTED
	// for manual review, same as ticket creation.
	s.applyApprovalPolicy(ctx, t)

	return t, nil
}

// rollbackOn rolls back tx and returns cause, folding in any rollback failure.
func rollbackOn(tx *ent.Tx, cause error) error {
	if rbErr := tx.Rollback(); rbErr != nil {
		return fmt.Errorf("%w (rollback failed: %v)", cause, rbErr)
	}
	return cause
}

// ListRevisions returns the revision history for a ticket.
func (s *Service) ListRevisions(ctx context.Context, ticketID int64) ([]model.TicketRevision, error) {
	rows, err := s.client.TicketRevision.Query().
		Where(entticketrevision.TicketIDEQ(ticketID)).
		Order(ent.Asc(entticketrevision.FieldRevision)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询历史版本失败: %w", err)
	}

	revisions := make([]model.TicketRevision, 0, len(rows))
	for _, r := range rows {
		revisions = append(revisions, model.TicketRevision{
			ID:             int64(r.ID),
			TicketID:       r.TicketID,
			Revision:       r.Revision,
			SQLContent:     r.SQLContent,
			SQLSummary:     r.SQLSummary,
			ChangeReason:   r.ChangeReason,
			RiskLevel:      r.RiskLevel,
			AIReviewResult: r.AiReviewResult,
			ReviewerID:     r.ReviewerID,
			ReviewComment:  r.ReviewComment,
			Status:         model.TicketStatus(r.Status),
			CreatedAt:      r.CreatedAt,
		})
	}

	// Now populate user names (requires additional queries)
	for i := range revisions {
		revisions[i].ReviewerName = s.lookupUsername(ctx, revisions[i].ReviewerID)
	}

	return revisions, nil
}
