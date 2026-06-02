package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// ResubmitTicket resubmits a rejected ticket with optional SQL and reason changes.
// The ticket must be in REJECTED status. Only the original submitter can resubmit.
func (s *TicketService) ResubmitTicket(ctx context.Context, ticketID, submitterID int64, sqlContent, changeReason string) (*model.Ticket, error) {
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

	// Snapshot the current (rejected) version into ticket_revisions
	_, err = s.client.TicketRevision.Create().
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
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("保存历史版本失败: %w", err)
	}

	// Update the ticket: new SQL, incremented revision, reset to SUBMITTED
	now := time.Now()
	summary := truncateSQL(sqlContent)
	newRevision := t.Revision + 1

	_, err = s.client.Ticket.UpdateOneID(int(ticketID)).
		SetSQLContent(sqlContent).
		SetSQLSummary(summary).
		SetChangeReason(changeReason).
		SetStatus(string(model.TicketStatusSubmitted)).
		SetRiskLevel("").
		SetAiReviewResult("").
		SetReviewerID(0).
		SetReviewComment("").
		SetRevision(newRevision).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("重提工单失败: %w", err)
	}

	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     submitterID,
		Action:     "ticket_resubmit",
		SQLContent: sqlContent,
		SQLSummary: summary,
	})

	t.SQLContent = sqlContent
	t.SQLSummary = summary
	t.ChangeReason = changeReason
	t.Status = model.TicketStatusSubmitted
	t.RiskLevel = ""
	t.AIReviewResult = ""
	t.ReviewerID = 0
	t.ReviewComment = ""
	t.Revision = newRevision
	t.UpdatedAt = now
	s.populateTicketNames(ctx, t)

	return t, nil
}

// ListRevisions returns the revision history for a ticket.
func (s *TicketService) ListRevisions(ctx context.Context, ticketID int64) ([]model.TicketRevision, error) {
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT id, ticket_id, revision, sql_content, sql_summary, change_reason, risk_level, ai_review_result, reviewer_id, review_comment, status, created_at
		 FROM ticket_revisions WHERE ticket_id = ? ORDER BY revision ASC`,
		ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询历史版本失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Read all rows first before populating names, since MaxOpenConns(1)
	// means the rows cursor holds the only connection.
	revisions := make([]model.TicketRevision, 0)
	for rows.Next() {
		var rev model.TicketRevision
		if err := rows.Scan(
			&rev.ID, &rev.TicketID, &rev.Revision, &rev.SQLContent,
			&rev.SQLSummary, &rev.ChangeReason, &rev.RiskLevel,
			&rev.AIReviewResult, &rev.ReviewerID, &rev.ReviewComment,
			&rev.Status, &rev.CreatedAt,
		); err != nil {
			continue
		}
		revisions = append(revisions, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历历史版本失败: %w", err)
	}

	// Now populate user names (requires additional queries)
	for i := range revisions {
		revisions[i].ReviewerName = s.lookupUsername(ctx, revisions[i].ReviewerID)
	}

	return revisions, nil
}
