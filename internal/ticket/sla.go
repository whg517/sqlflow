package ticket

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entslaactionlog "github.com/whg517/sqlflow/internal/db/ent/slaactionlog"
	entslaconfig "github.com/whg517/sqlflow/internal/db/ent/slaconfig"
	entticket "github.com/whg517/sqlflow/internal/db/ent/ticket"
	entuser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/notify"
)

// SLATicketStatus holds SLA status info for a single ticket.
type SLATicketStatus struct {
	TicketID             int64      `json:"ticket_id"`
	SLADeadline          *time.Time `json:"sla_deadline,omitempty"`
	SLAStatus            string     `json:"sla_status"` // normal | warning | breached
	TimeRemainingSeconds float64    `json:"time_remaining_seconds"`
}

// slaTicketRow is a minimal ticket representation used by SLA checks.
// SLAService only needs these fields — it never touches full ticket scan.
type slaTicketRow struct {
	ID          int64
	SubmitterID int64
	RiskLevel   string
	ReviewerID  int64
	SQLSummary  string
	CreatedAt   time.Time
	SLADeadline *time.Time
	SLAStatus   string
}

// SLAService manages SLA configuration, deadline computation, and notifications.
// It is a single-instance service with explicit lifecycle management.
// All public methods are safe for concurrent use (database-level serialization).
type SLAService struct {
	database  *db.DB
	client    *ent.Client
	notifySvc *notify.Service
}

// NewSLAService creates a new SLAService.
// notifySvc may be nil; notification methods will be no-ops until it is set.
func NewSLAService(database *db.DB, notifySvc *notify.Service) *SLAService {
	return &SLAService{
		database:  database,
		client:    database.Client(),
		notifySvc: notifySvc,
	}
}

// ---------------------------------------------------------------------------
// SLA Config CRUD
// ---------------------------------------------------------------------------

// entSLAConfigToModel converts a generated SLA config to the API shape.
func entSLAConfigToModel(c *ent.SLAConfig) *model.SLAConfig {
	return &model.SLAConfig{
		ID:                int64(c.ID),
		Priority:          c.Priority,
		TimeoutMinutes:    c.TimeoutMinutes,
		ReminderPercent:   c.ReminderPercent,
		EscalateToRole:    c.EscalateToRole,
		EscalateToUser:    c.EscalateToUser,
		AutoRejectEnabled: c.AutoRejectEnabled,
		Enabled:           c.Enabled,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}
}

// ListConfigs returns all SLA configurations.
func (s *SLAService) ListConfigs(ctx context.Context) ([]*model.SLAConfig, error) {
	rows, err := s.client.SLAConfig.Query().
		Order(ent.Asc(entslaconfig.FieldTimeoutMinutes)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sla configs: %w", err)
	}
	configs := make([]*model.SLAConfig, 0, len(rows))
	for _, c := range rows {
		configs = append(configs, entSLAConfigToModel(c))
	}
	return configs, nil
}

// GetConfig returns the SLA config for a given priority, or nil when none is
// configured for it.
//
// A missing config is not an error: most priorities have no SLA, and the
// callers treat "no config" as "no deadline to enforce".
func (s *SLAService) GetConfig(ctx context.Context, priority string) (*model.SLAConfig, error) {
	c, err := s.client.SLAConfig.Query().
		Where(entslaconfig.PriorityEQ(priority)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get sla config: %w", err)
	}
	return entSLAConfigToModel(c), nil
}

// CreateConfig creates a new SLA configuration.
func (s *SLAService) CreateConfig(ctx context.Context, cfg *model.SLAConfig) (*model.SLAConfig, error) {
	now := time.Now()
	created, err := s.client.SLAConfig.Create().
		SetPriority(cfg.Priority).
		SetTimeoutMinutes(cfg.TimeoutMinutes).
		SetReminderPercent(cfg.ReminderPercent).
		SetEscalateToRole(cfg.EscalateToRole).
		SetEscalateToUser(cfg.EscalateToUser).
		SetAutoRejectEnabled(cfg.AutoRejectEnabled).
		SetEnabled(cfg.Enabled).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create sla config: %w", err)
	}
	return entSLAConfigToModel(created), nil
}

// UpdateConfig updates an existing SLA configuration.
func (s *SLAService) UpdateConfig(ctx context.Context, id int64, cfg *model.SLAConfig) error {
	err := s.client.SLAConfig.UpdateOneID(int(id)).
		SetPriority(cfg.Priority).
		SetTimeoutMinutes(cfg.TimeoutMinutes).
		SetReminderPercent(cfg.ReminderPercent).
		SetEscalateToRole(cfg.EscalateToRole).
		SetEscalateToUser(cfg.EscalateToUser).
		SetAutoRejectEnabled(cfg.AutoRejectEnabled).
		SetEnabled(cfg.Enabled).
		SetUpdatedAt(time.Now()).
		Exec(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("update sla config: %w", err)
	}
	return nil
}

// DeleteConfig deletes an SLA configuration.
func (s *SLAService) DeleteConfig(ctx context.Context, id int64) error {
	err := s.client.SLAConfig.DeleteOneID(int(id)).Exec(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("delete sla config: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// SLA Deadline Management
// ---------------------------------------------------------------------------

// SetSLADeadline sets the SLA deadline for a ticket based on its risk_level/priority.
// If the deadline is already set, it is not changed.
func (s *SLAService) SetSLADeadline(ctx context.Context, ticketID int64, riskLevel string) error {
	// Check current deadline
	t, err := s.client.Ticket.Get(ctx, int(ticketID))
	if err != nil {
		return fmt.Errorf("check sla deadline: %w", err)
	}
	if t.SLADeadline != nil {
		return nil // already set
	}

	// Look up SLA config by priority (risk_level maps to priority)
	priority := strings.ToLower(riskLevel)
	if priority == "" {
		priority = "medium" // default
	}

	cfg, err := s.GetConfig(ctx, priority)
	if err != nil {
		return fmt.Errorf("lookup sla config: %w", err)
	}
	if cfg == nil || !cfg.Enabled {
		return nil // no matching config
	}

	dl := time.Now().Add(time.Duration(cfg.TimeoutMinutes) * time.Minute)
	err = s.client.Ticket.UpdateOneID(int(ticketID)).
		SetSLADeadline(dl).
		SetSLAStatus("normal").
		SetUpdatedAt(time.Now()).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("set sla deadline: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// SLA Check (called by scheduler)
// ---------------------------------------------------------------------------

// CheckSLA scans all PENDING_APPROVAL tickets with SLA deadlines and sends
// reminder/escalation notifications as needed.
//
// Architecture: SLAService independently queries only the fields it needs
// (id, risk_level, reviewer_id, created_at, sla_deadline, sla_status).
// It does NOT depend on Service.scanTicket or full ticket rows.
func (s *SLAService) CheckSLA(ctx context.Context) error {
	now := time.Now()

	// Query only the minimal fields needed for SLA checking.
	// This is intentionally separate from Service.scanTicket —
	// SLAService owns its own query to maintain clear responsibility boundaries.
	rows, err := s.client.Ticket.Query().
		Where(
			entticket.StatusEQ(string(model.TicketStatusPendingApproval)),
			entticket.SLADeadlineNotNil(),
		).
		All(ctx)
	if err != nil {
		return fmt.Errorf("sla check query: %w", err)
	}
	tickets := make([]slaTicketRow, 0, len(rows))
	for _, t := range rows {
		row := slaTicketRow{
			ID:          int64(t.ID),
			SubmitterID: t.SubmitterID,
			RiskLevel:   t.RiskLevel,
			ReviewerID:  t.ReviewerID,
			SQLSummary:  t.SQLSummary,
			CreatedAt:   t.CreatedAt,
			SLAStatus:   t.SLAStatus,
		}
		row.SLADeadline = t.SLADeadline
		tickets = append(tickets, row)
	}

	for _, t := range tickets {
		if t.SLADeadline == nil {
			continue
		}

		// Look up SLA config
		priority := strings.ToLower(t.RiskLevel)
		if priority == "" {
			priority = "medium"
		}
		cfg, err := s.GetConfig(ctx, priority)
		if err != nil {
			log.Printf("sla: lookup config for ticket #%d: %v", t.ID, err)
			continue
		}
		if cfg == nil || !cfg.Enabled {
			continue
		}

		totalDuration := time.Duration(cfg.TimeoutMinutes) * time.Minute
		elapsed := now.Sub(t.CreatedAt)
		percent := float64(elapsed) / float64(totalDuration) * 100

		// Lookup approver name
		approverName := s.lookupUsername(ctx, t.ReviewerID)

		switch {
		case percent >= 100:
			// Auto-reject: if enabled, reject the ticket and notify submitter
			if cfg.AutoRejectEnabled {
				dedupKey := fmt.Sprintf("%d:auto_reject", t.ID)
				if ok, _ := s.tryRecordAction(ctx, t.ID, "auto_reject", dedupKey, "", cfg.ID); ok {
					s.autoRejectTicket(ctx, t, cfg)
				}
			} else {
				// Escalation: limit 1 per day — idempotent via sla_action_log
				dedupKey := fmt.Sprintf("%d:escalate:%s", t.ID, now.Format("2006-01-02"))
				if ok, _ := s.tryRecordAction(ctx, t.ID, "escalate", dedupKey, approverName, cfg.ID); ok {
					s.sendEscalation(ctx, t, cfg, approverName)
				}
			}
			// Update status to breached
			if t.SLAStatus != "breached" {
				_ = s.setSLAStatus(ctx, t.ID, "breached", now)
			}

		case percent >= float64(cfg.ReminderPercent):
			// Reminder: limit 3 per day — idempotent via sla_action_log with hour granularity
			dedupKey := fmt.Sprintf("%d:reminder:%s:%02d", t.ID, now.Format("2006-01-02"), now.Hour())
			if ok, _ := s.tryRecordAction(ctx, t.ID, "reminder", dedupKey, approverName, cfg.ID); ok {
				s.sendReminder(ctx, t, cfg, percent, approverName)
			}
			// Update status to warning
			if t.SLAStatus != "warning" && t.SLAStatus != "breached" {
				_ = s.setSLAStatus(ctx, t.ID, "warning", now)
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Batch SLA status queries
// ---------------------------------------------------------------------------

// GetTicketSLAStatuses returns SLA status for a batch of tickets.
func (s *SLAService) GetTicketSLAStatuses(ctx context.Context, ticketIDs []int64) (map[int64]*SLATicketStatus, error) {
	if len(ticketIDs) == 0 {
		return map[int64]*SLATicketStatus{}, nil
	}

	rows, err := s.client.Ticket.Query().
		Where(entticket.IDIn(toEntIDs(ticketIDs)...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("batch sla status query: %w", err)
	}

	result := make(map[int64]*SLATicketStatus, len(ticketIDs))
	now := time.Now()
	for _, t := range rows {
		st := &SLATicketStatus{TicketID: int64(t.ID), SLAStatus: t.SLAStatus}
		if t.SLADeadline != nil {
			st.SLADeadline = t.SLADeadline
			st.TimeRemainingSeconds = t.SLADeadline.Sub(now).Seconds()
		}
		result[int64(t.ID)] = st
	}
	return result, nil
}

// derefInt64 reads through a nullable column, treating NULL as zero.
func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// toEntIDs narrows the domain's int64 ids to the int ent generates for the
// primary key.
func toEntIDs(ids []int64) []int {
	out := make([]int, len(ids))
	for i, id := range ids {
		out[i] = int(id)
	}
	return out
}

// setSLAStatus records the ticket's SLA state after a check.
//
// Not compare-and-swap: this is an advisory field the scheduler recomputes on
// every pass, not a state machine transition. Losing a race here means the next
// pass sets it, sixty seconds later.
func (s *SLAService) setSLAStatus(ctx context.Context, ticketID int64, status string, at time.Time) error {
	return s.client.Ticket.UpdateOneID(int(ticketID)).
		SetSLAStatus(status).
		SetUpdatedAt(at).
		Exec(ctx)

}

// ---------------------------------------------------------------------------
// Notification history
// ---------------------------------------------------------------------------

// ListNotifications returns paginated SLA action log records.
func (s *SLAService) ListNotifications(ctx context.Context, page, pageSize int) ([]*model.SLANotification, int64, error) {
	p := sqlutil.ParsePagination(page, pageSize)

	total, err := s.client.SLAActionLog.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count sla action log: %w", err)
	}

	rows, err := s.client.SLAActionLog.Query().
		Order(ent.Desc(entslaactionlog.FieldCreatedAt)).
		Limit(p.PageSize).
		Offset(p.Offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list sla action log: %w", err)
	}

	notifications := make([]*model.SLANotification, 0, len(rows))
	for _, r := range rows {
		notifications = append(notifications, &model.SLANotification{
			ID:               int64(r.ID),
			TicketID:         r.TicketID,
			NotificationType: r.ActionType,
			Stage:            r.DedupKey,
			NotifiedUser:     r.NotifiedUser,
			NotifiedAt:       r.CreatedAt,
			SLAConfigID:      derefInt64(r.SLAConfigID),
		})
	}
	return notifications, int64(total), nil
}

// ---------------------------------------------------------------------------
// Clear SLA (called on approve/reject)
// ---------------------------------------------------------------------------

// ClearTicketSLA clears the SLA deadline and status for a ticket.
func (s *SLAService) ClearTicketSLA(ctx context.Context, ticketID int64) error {
	err := s.client.Ticket.UpdateOneID(int(ticketID)).
		ClearSLADeadline().
		SetSLAStatus("normal").
		SetUpdatedAt(time.Now()).
		Exec(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("clear ticket sla: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (s *SLAService) lookupUsername(ctx context.Context, userID int64) string {
	if userID == 0 {
		return ""
	}
	username, err := s.client.User.Query().
		Where(entuser.IDEQ(int(userID))).
		Select(entuser.FieldUsername).
		String(ctx)
	if err != nil {
		// The name only decorates a notification; a lookup failure must not stop
		// the SLA action it belongs to.
		return ""
	}
	return username
}

// tryRecordAction attempts to insert a deduplicated action log entry.
// Returns true if the record was inserted (not a duplicate).
// This provides scheduling idempotency — the same action on the same ticket
// within the same dedup window will not be recorded twice.
func (s *SLAService) tryRecordAction(ctx context.Context, ticketID int64, actionType, dedupKey, notifiedUser string, configID int64) (bool, error) {
	// The unique constraint on dedup_key is what makes this idempotent, and the
	// insert is what tests it: two schedulers racing on the same ticket both
	// reach here, and exactly one insert lands. Checking for an existing row
	// first would let both pass the check before either wrote.
	affected, err := s.client.SLAActionLog.Create().
		SetTicketID(ticketID).
		SetActionType(actionType).
		SetDedupKey(dedupKey).
		SetNotifiedUser(notifiedUser).
		SetCreatedAt(time.Now()).
		SetSLAConfigID(configID).
		OnConflictColumns(entslaactionlog.FieldDedupKey).
		DoNothing().
		ID(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("sla: record action failed: %v", err)
		return false, err
	}
	// DO NOTHING returns no row when the key already existed, which ent surfaces
	// as ErrNoRows — that is the duplicate, not a failure.
	return err == nil && affected > 0, nil
}

func (s *SLAService) sendReminder(ctx context.Context, t slaTicketRow, cfg *model.SLAConfig, percent float64, approverName string) {
	if s.notifySvc == nil {
		return
	}
	elapsedHours := time.Since(t.CreatedAt).Hours()
	slaHours := float64(cfg.TimeoutMinutes) / 60.0

	s.notifySvc.NotifySLAReminderRaw(t.ID, fmt.Sprintf("%.1f", elapsedHours), fmt.Sprintf("%.1f", slaHours), approverName, percent)
}

func (s *SLAService) sendEscalation(ctx context.Context, t slaTicketRow, cfg *model.SLAConfig, approverName string) {
	if s.notifySvc == nil {
		return
	}
	slaHours := float64(cfg.TimeoutMinutes) / 60.0

	s.notifySvc.NotifySLAEscalateRaw(t.ID, fmt.Sprintf("%.1f", slaHours), approverName)
}

// autoRejectTicket rejects a ticket due to SLA breach and notifies the submitter.
// This is idempotent — the dedup_key ensures it only runs once per ticket.
func (s *SLAService) autoRejectTicket(ctx context.Context, t slaTicketRow, cfg *model.SLAConfig) {
	now := time.Now()

	// The same compare-and-swap the approval paths use. The dedup key stops the
	// scheduler running this twice, but it does not stop a human approving the
	// ticket in the same moment — only the status predicate does.
	comment := fmt.Sprintf("审批超时自动拒绝 (SLA: %s, 超时 %d 分钟)", cfg.Priority, cfg.TimeoutMinutes)
	applied, err := casTicketStatus(ctx, s.client.Ticket, t.ID,
		model.TicketStatusPendingApproval, model.TicketStatusRejected, now,
		func(u *ent.TicketUpdate) *ent.TicketUpdate {
			return u.SetReviewerID(0).SetReviewComment(comment)
		})
	if err != nil {
		log.Printf("sla: auto-reject ticket #%d failed: %v", t.ID, err)
		return
	}
	if !applied {
		return // already decided by someone else
	}

	log.Printf("sla: ticket #%d auto-rejected (SLA breach, priority=%s, timeout=%dm)", t.ID, cfg.Priority, cfg.TimeoutMinutes)

	// Notify submitter about auto-rejection
	if s.notifySvc != nil {
		submitterName := s.lookupUsername(ctx, t.SubmitterID)
		s.notifySvc.NotifySLAAutoReject(t.ID, cfg.Priority, cfg.TimeoutMinutes, submitterName, t.SQLSummary)
	}
}
