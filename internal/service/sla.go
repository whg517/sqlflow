package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// SLATicketStatus holds SLA status info for a single ticket.
type SLATicketStatus struct {
	TicketID              int64      `json:"ticket_id"`
	SLADeadline           *time.Time `json:"sla_deadline,omitempty"`
	SLAStatus             string     `json:"sla_status"` // normal | warning | breached
	TimeRemainingSeconds float64    `json:"time_remaining_seconds"`
}

// slaTicketRow is a minimal ticket representation used by SLA checks.
// SLAService only needs these fields — it never touches full ticket scan.
type slaTicketRow struct {
	ID           int64
	RiskLevel    string
	ReviewerID   int64
	CreatedAt    time.Time
	SLADeadline  *time.Time
	SLAStatus    string
}

// SLAService manages SLA configuration, deadline computation, and notifications.
// It is a single-instance service with explicit lifecycle management.
// All public methods are safe for concurrent use (database-level serialization).
type SLAService struct {
	db        *sql.DB
	notifySvc *NotifyService
}

// NewSLAService creates a new SLAService.
// notifySvc may be nil; notification methods will be no-ops until it is set.
func NewSLAService(db *sql.DB, notifySvc *NotifyService) *SLAService {
	return &SLAService{
		db:        db,
		notifySvc: notifySvc,
	}
}

// ---------------------------------------------------------------------------
// SLA Config CRUD
// ---------------------------------------------------------------------------

// ListConfigs returns all SLA configurations.
func (s *SLAService) ListConfigs(ctx context.Context) ([]*model.SLAConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, priority, timeout_minutes, reminder_percent, escalate_to_role, escalate_to_user, enabled, created_at, updated_at
		 FROM sla_config ORDER BY timeout_minutes ASC`)
	if err != nil {
		return nil, fmt.Errorf("list sla configs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var configs []*model.SLAConfig
	for rows.Next() {
		c := &model.SLAConfig{}
		var enabled int
		if err := rows.Scan(&c.ID, &c.Priority, &c.TimeoutMinutes, &c.ReminderPercent,
			&c.EscalateToRole, &c.EscalateToUser, &enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan sla config: %w", err)
		}
		c.Enabled = enabled == 1
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// GetConfig returns the SLA config for a given priority.
func (s *SLAService) GetConfig(ctx context.Context, priority string) (*model.SLAConfig, error) {
	c := &model.SLAConfig{}
	var enabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, priority, timeout_minutes, reminder_percent, escalate_to_role, escalate_to_user, enabled, created_at, updated_at
		 FROM sla_config WHERE priority = ?`, priority).Scan(
		&c.ID, &c.Priority, &c.TimeoutMinutes, &c.ReminderPercent,
		&c.EscalateToRole, &c.EscalateToUser, &enabled, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get sla config: %w", err)
	}
	c.Enabled = enabled == 1
	return c, nil
}

// CreateConfig creates a new SLA configuration.
func (s *SLAService) CreateConfig(ctx context.Context, cfg *model.SLAConfig) (*model.SLAConfig, error) {
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	now := time.Now()
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO sla_config (priority, timeout_minutes, reminder_percent, escalate_to_role, escalate_to_user, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		cfg.Priority, cfg.TimeoutMinutes, cfg.ReminderPercent, cfg.EscalateToRole, cfg.EscalateToUser, enabled, now, now)
	if err != nil {
		return nil, fmt.Errorf("create sla config: %w", err)
	}
	id, _ := result.LastInsertId()
	cfg.ID = id
	cfg.CreatedAt = now
	cfg.UpdatedAt = now
	return cfg, nil
}

// UpdateConfig updates an existing SLA configuration.
func (s *SLAService) UpdateConfig(ctx context.Context, id int64, cfg *model.SLAConfig) error {
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	now := time.Now()
	_, err := s.db.ExecContext(ctx,
		`UPDATE sla_config SET priority = ?, timeout_minutes = ?, reminder_percent = ?, escalate_to_role = ?, escalate_to_user = ?, enabled = ?, updated_at = ?
		 WHERE id = ?`,
		cfg.Priority, cfg.TimeoutMinutes, cfg.ReminderPercent, cfg.EscalateToRole, cfg.EscalateToUser, enabled, now, id)
	if err != nil {
		return fmt.Errorf("update sla config: %w", err)
	}
	return nil
}

// DeleteConfig deletes an SLA configuration.
func (s *SLAService) DeleteConfig(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sla_config WHERE id = ?`, id)
	if err != nil {
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
	var deadline sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT sla_deadline FROM tickets WHERE id = ?`, ticketID).Scan(&deadline)
	if err != nil {
		return fmt.Errorf("check sla deadline: %w", err)
	}
	if deadline.Valid {
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
	_, err = s.db.ExecContext(ctx,
		`UPDATE tickets SET sla_deadline = ?, sla_status = 'normal', updated_at = ? WHERE id = ?`,
		dl, time.Now(), ticketID)
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
// It does NOT depend on TicketService.scanTicket or full ticket rows.
func (s *SLAService) CheckSLA(ctx context.Context) error {
	now := time.Now()

	// Query only the minimal fields needed for SLA checking.
	// This is intentionally separate from TicketService.scanTicket —
	// SLAService owns its own query to maintain clear responsibility boundaries.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, risk_level, reviewer_id, created_at, sla_deadline, sla_status
		 FROM tickets
		 WHERE status = ? AND sla_deadline IS NOT NULL`,
		model.TicketStatusPendingApproval,
	)
	if err != nil {
		return fmt.Errorf("sla check query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tickets, err := s.scanActiveTicketRows(rows)
	if err != nil {
		return fmt.Errorf("sla scan tickets: %w", err)
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
			// Escalation: limit 1 per day — idempotent via sla_action_log
			dedupKey := fmt.Sprintf("%d:escalate:%s", t.ID, now.Format("2006-01-02"))
			if ok, _ := s.tryRecordAction(ctx, t.ID, "escalate", dedupKey, approverName, cfg.ID); ok {
				s.sendEscalation(ctx, t, cfg, approverName)
			}
			// Update status to breached
			if t.SLAStatus != "breached" {
				_, _ = s.db.ExecContext(ctx, `UPDATE tickets SET sla_status = 'breached', updated_at = ? WHERE id = ?`, now, t.ID)
			}

		case percent >= float64(cfg.ReminderPercent):
			// Reminder: limit 3 per day — idempotent via sla_action_log with hour granularity
			dedupKey := fmt.Sprintf("%d:reminder:%s:%02d", t.ID, now.Format("2006-01-02"), now.Hour())
			if ok, _ := s.tryRecordAction(ctx, t.ID, "reminder", dedupKey, approverName, cfg.ID); ok {
				s.sendReminder(ctx, t, cfg, percent, approverName)
			}
			// Update status to warning
			if t.SLAStatus != "warning" && t.SLAStatus != "breached" {
				_, _ = s.db.ExecContext(ctx, `UPDATE tickets SET sla_status = 'warning', updated_at = ? WHERE id = ?`, now, t.ID)
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

	placeholders := make([]string, len(ticketIDs))
	args := make([]interface{}, len(ticketIDs))
	for i, id := range ticketIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(
		`SELECT id, sla_deadline, sla_status FROM tickets WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch sla status query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64]*SLATicketStatus, len(ticketIDs))
	now := time.Now()
	for rows.Next() {
		var id int64
		var deadline sql.NullTime
		var status string
		if err := rows.Scan(&id, &deadline, &status); err != nil {
			return nil, fmt.Errorf("scan batch sla status: %w", err)
		}
		st := &SLATicketStatus{TicketID: id, SLAStatus: status}
		if deadline.Valid {
			st.SLADeadline = &deadline.Time
			st.TimeRemainingSeconds = deadline.Time.Sub(now).Seconds()
		}
		result[id] = st
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Notification history
// ---------------------------------------------------------------------------

// ListNotifications returns paginated SLA action log records.
func (s *SLAService) ListNotifications(ctx context.Context, page, pageSize int) ([]*model.SLANotification, int64, error) {
	p := ParsePagination(page, pageSize)

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_action_log`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count sla action log: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ticket_id, action_type, dedup_key, notified_user, created_at, sla_config_id
		 FROM sla_action_log ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		p.PageSize, p.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list sla action log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notifications []*model.SLANotification
	for rows.Next() {
		n := &model.SLANotification{}
		if err := rows.Scan(&n.ID, &n.TicketID, &n.NotificationType, &n.Stage, &n.NotifiedUser, &n.NotifiedAt, &n.SLAConfigID); err != nil {
			return nil, 0, fmt.Errorf("scan sla action log: %w", err)
		}
		notifications = append(notifications, n)
	}
	return notifications, total, rows.Err()
}

// ---------------------------------------------------------------------------
// Clear SLA (called on approve/reject)
// ---------------------------------------------------------------------------

// ClearTicketSLA clears the SLA deadline and status for a ticket.
func (s *SLAService) ClearTicketSLA(ctx context.Context, ticketID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tickets SET sla_deadline = NULL, sla_status = 'normal', updated_at = ? WHERE id = ?`,
		time.Now(), ticketID)
	if err != nil {
		return fmt.Errorf("clear ticket sla: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// scanActiveTicketRows scans minimal ticket fields from SLA's own query.
// This is intentionally separate from TicketService.scanTicket —
// SLAService owns only what it needs.
func (s *SLAService) scanActiveTicketRows(rows *sql.Rows) ([]slaTicketRow, error) {
	var tickets []slaTicketRow
	for rows.Next() {
		var t slaTicketRow
		var deadline sql.NullTime
		err := rows.Scan(&t.ID, &t.RiskLevel, &t.ReviewerID, &t.CreatedAt, &deadline, &t.SLAStatus)
		if err != nil {
			return nil, err
		}
		if deadline.Valid {
			t.SLADeadline = &deadline.Time
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (s *SLAService) lookupUsername(ctx context.Context, userID int64) string {
	if userID == 0 {
		return ""
	}
	var username string
	if err := s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, userID).Scan(&username); err != nil {
		return ""
	}
	return username
}

// tryRecordAction attempts to insert a deduplicated action log entry.
// Returns true if the record was inserted (not a duplicate).
// This provides scheduling idempotency — the same action on the same ticket
// within the same dedup window will not be recorded twice.
func (s *SLAService) tryRecordAction(ctx context.Context, ticketID int64, actionType, dedupKey, notifiedUser string, configID int64) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO sla_action_log (ticket_id, action_type, dedup_key, notified_user, created_at, sla_config_id)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(dedup_key) DO NOTHING`,
		ticketID, actionType, dedupKey, notifiedUser, time.Now(), configID,
	)
	if err != nil {
		log.Printf("sla: record action failed: %v", err)
		return false, err
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
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
