package ticket

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/notify"
	"github.com/whg517/sqlflow/internal/testutil"
)

func setupSLATestDB(t *testing.T) *db.DB {
	t.Helper()
	return testutil.NewDB(t)
}

func createTestUserForSLA(t *testing.T, ctx context.Context, d *sql.DB, username string) int64 {
	t.Helper()
	var id int64
	if err := d.QueryRowContext(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES ($1, $2, $3) RETURNING id`,
		username, "$2a$10$hash", "dba").Scan(&id); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return id
}

func createTestDatasourceForSLA(t *testing.T, ctx context.Context, d *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := d.QueryRowContext(ctx,
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, database) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		"test-ds", "mysql", "localhost", 3306, "root", "encrypted", testutil.DatasourceDatabase).Scan(&id); err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	return id
}

func createTestTicketForSLA(t *testing.T, ctx context.Context, d *sql.DB, submitterID, dsID int64, status string, createdAt time.Time, slaDeadline *time.Time) int64 {
	t.Helper()
	var id int64
	if err := d.QueryRowContext(ctx,
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, status, risk_level, created_at, updated_at, sla_deadline, sla_status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		submitterID, dsID, "testdb", "SELECT 1", "SELECT 1", "mysql", status, "medium", createdAt, createdAt, slaDeadline, "normal").Scan(&id); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	return id
}

func TestSLAService_AutoReject_Breached(t *testing.T) {
	ctx := context.Background()
	d := setupSLATestDB(t)
	notifySvc := notify.NewService(notify.Deps{DB: d})
	slaSvc := NewSLAService(d, notifySvc)

	submitterID := createTestUserForSLA(t, ctx, d.DB, "submitter1")
	dsID := createTestDatasourceForSLA(t, ctx, d.DB)

	// Create SLA config with auto_reject_enabled = true, timeout = 60 minutes
	_, err := d.ExecContext(ctx,
		`INSERT INTO sla_config (priority, timeout_minutes, reminder_percent, escalate_to_role, auto_reject_enabled, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"medium", 60, 80, "admin", true, true, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("create sla config: %v", err)
	}

	// Create a PENDING_APPROVAL ticket created 70 minutes ago (past deadline)
	createdAt := time.Now().Add(-70 * time.Minute)
	deadline := createdAt.Add(60 * time.Minute)
	ticketID := createTestTicketForSLA(t, ctx, d.DB, submitterID, dsID, string(model.TicketStatusPendingApproval), createdAt, &deadline)

	// Run SLA check
	if err := slaSvc.CheckSLA(ctx); err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}

	// Verify ticket was auto-rejected
	var status string
	err = d.QueryRowContext(ctx, `SELECT status FROM tickets WHERE id = $1`, ticketID).Scan(&status)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if status != string(model.TicketStatusRejected) {
		t.Errorf("ticket status = %q, want %q", status, model.TicketStatusRejected)
	}

	// Verify review_comment mentions auto-reject
	var comment string
	err = d.QueryRowContext(ctx, `SELECT review_comment FROM tickets WHERE id = $1`, ticketID).Scan(&comment)
	if err != nil {
		t.Fatalf("query comment: %v", err)
	}
	if comment == "" {
		t.Error("review_comment should not be empty")
	}

	// Verify sla_status = breached
	var slaStatus string
	err = d.QueryRowContext(ctx, `SELECT sla_status FROM tickets WHERE id = $1`, ticketID).Scan(&slaStatus)
	if err != nil {
		t.Fatalf("query sla_status: %v", err)
	}
	if slaStatus != "breached" {
		t.Errorf("sla_status = %q, want %q", slaStatus, "breached")
	}

	// Verify action was logged
	var actionCount int
	err = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_action_log WHERE ticket_id = $1 AND action_type = 'auto_reject'`, ticketID).Scan(&actionCount)
	if err != nil {
		t.Fatalf("query action log: %v", err)
	}
	if actionCount != 1 {
		t.Errorf("action_count = %d, want 1", actionCount)
	}
}

func TestSLAService_AutoReject_Idempotent(t *testing.T) {
	ctx := context.Background()
	d := setupSLATestDB(t)
	slaSvc := NewSLAService(d, nil)

	submitterID := createTestUserForSLA(t, ctx, d.DB, "submitter2")
	dsID := createTestDatasourceForSLA(t, ctx, d.DB)

	_, err := d.ExecContext(ctx,
		`INSERT INTO sla_config (priority, timeout_minutes, reminder_percent, escalate_to_role, auto_reject_enabled, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"medium", 60, 80, "admin", true, true, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("create sla config: %v", err)
	}

	createdAt := time.Now().Add(-70 * time.Minute)
	deadline := createdAt.Add(60 * time.Minute)
	ticketID := createTestTicketForSLA(t, ctx, d.DB, submitterID, dsID, string(model.TicketStatusPendingApproval), createdAt, &deadline)

	// Run CheckSLA twice — should be idempotent
	if err := slaSvc.CheckSLA(ctx); err != nil {
		t.Fatalf("CheckSLA 1: %v", err)
	}
	if err := slaSvc.CheckSLA(ctx); err != nil {
		t.Fatalf("CheckSLA 2: %v", err)
	}

	// Verify only one auto_reject action logged
	var actionCount int
	err = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_action_log WHERE ticket_id = $1 AND action_type = 'auto_reject'`, ticketID).Scan(&actionCount)
	if err != nil {
		t.Fatalf("query action log: %v", err)
	}
	if actionCount != 1 {
		t.Errorf("action_count = %d, want 1 (idempotent)", actionCount)
	}
}

func TestSLAService_NoAutoReject_WhenDisabled(t *testing.T) {
	ctx := context.Background()
	d := setupSLATestDB(t)
	slaSvc := NewSLAService(d, nil)

	submitterID := createTestUserForSLA(t, ctx, d.DB, "submitter3")
	dsID := createTestDatasourceForSLA(t, ctx, d.DB)

	// auto_reject_enabled = false (default)
	_, err := d.ExecContext(ctx,
		`INSERT INTO sla_config (priority, timeout_minutes, reminder_percent, escalate_to_role, auto_reject_enabled, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"medium", 60, 80, "admin", false, true, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("create sla config: %v", err)
	}

	createdAt := time.Now().Add(-70 * time.Minute)
	deadline := createdAt.Add(60 * time.Minute)
	ticketID := createTestTicketForSLA(t, ctx, d.DB, submitterID, dsID, string(model.TicketStatusPendingApproval), createdAt, &deadline)

	if err := slaSvc.CheckSLA(ctx); err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}

	// Ticket should still be PENDING_APPROVAL (not auto-rejected)
	var status string
	err = d.QueryRowContext(ctx, `SELECT status FROM tickets WHERE id = $1`, ticketID).Scan(&status)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if status != string(model.TicketStatusPendingApproval) {
		t.Errorf("ticket status = %q, want %q (not auto-rejected)", status, model.TicketStatusPendingApproval)
	}

	// But escalation should be logged (the existing behavior)
	var actionCount int
	err = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_action_log WHERE ticket_id = $1 AND action_type = 'escalate'`, ticketID).Scan(&actionCount)
	if err != nil {
		t.Fatalf("query action log: %v", err)
	}
	if actionCount != 1 {
		t.Errorf("escalate action_count = %d, want 1", actionCount)
	}
}

func TestSLAService_NoAutoReject_WhenNotBreached(t *testing.T) {
	ctx := context.Background()
	d := setupSLATestDB(t)
	slaSvc := NewSLAService(d, nil)

	submitterID := createTestUserForSLA(t, ctx, d.DB, "submitter4")
	dsID := createTestDatasourceForSLA(t, ctx, d.DB)

	_, err := d.ExecContext(ctx,
		`INSERT INTO sla_config (priority, timeout_minutes, reminder_percent, escalate_to_role, auto_reject_enabled, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"medium", 60, 80, "admin", true, true, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("create sla config: %v", err)
	}

	// Ticket created 30 minutes ago (not yet breached, 60min timeout)
	createdAt := time.Now().Add(-30 * time.Minute)
	deadline := createdAt.Add(60 * time.Minute)
	ticketID := createTestTicketForSLA(t, ctx, d.DB, submitterID, dsID, string(model.TicketStatusPendingApproval), createdAt, &deadline)

	if err := slaSvc.CheckSLA(ctx); err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}

	// Ticket should still be PENDING_APPROVAL
	var status string
	err = d.QueryRowContext(ctx, `SELECT status FROM tickets WHERE id = $1`, ticketID).Scan(&status)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if status != string(model.TicketStatusPendingApproval) {
		t.Errorf("ticket status = %q, want %q (not breached)", status, model.TicketStatusPendingApproval)
	}

	// No auto_reject action should be logged
	var actionCount int
	err = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_action_log WHERE ticket_id = $1 AND action_type = 'auto_reject'`, ticketID).Scan(&actionCount)
	if err != nil {
		t.Fatalf("query action log: %v", err)
	}
	if actionCount != 0 {
		t.Errorf("auto_reject action_count = %d, want 0", actionCount)
	}
}

func TestSLAService_ConfigCRUD_WithAutoReject(t *testing.T) {
	ctx := context.Background()
	d := setupSLATestDB(t)
	slaSvc := NewSLAService(d, nil)

	// Create config with auto_reject_enabled
	cfg := &model.SLAConfig{
		Priority:          "high",
		TimeoutMinutes:    120,
		ReminderPercent:   80,
		EscalateToRole:    "admin",
		AutoRejectEnabled: true,
		Enabled:           true,
	}
	created, err := slaSvc.CreateConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}
	if !created.AutoRejectEnabled {
		t.Error("AutoRejectEnabled should be true")
	}

	// GetConfig should return auto_reject_enabled
	got, err := slaSvc.GetConfig(ctx, "high")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if !got.AutoRejectEnabled {
		t.Error("GetConfig: AutoRejectEnabled should be true")
	}

	// ListConfigs should include auto_reject_enabled
	configs, err := slaSvc.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	found := false
	for _, c := range configs {
		if c.Priority == "high" && c.AutoRejectEnabled {
			found = true
		}
	}
	if !found {
		t.Error("ListConfigs: high priority with AutoRejectEnabled not found")
	}

	// UpdateConfig should update auto_reject_enabled
	cfg.AutoRejectEnabled = false
	if err := slaSvc.UpdateConfig(ctx, created.ID, cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	updated, err := slaSvc.GetConfig(ctx, "high")
	if err != nil {
		t.Fatalf("GetConfig after update: %v", err)
	}
	if updated.AutoRejectEnabled {
		t.Error("AutoRejectEnabled should be false after update")
	}
}
