package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/mask"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

// ---------------------------------------------------------------------------
// Shared integration test helpers
// ---------------------------------------------------------------------------

// setupIntegrationDB creates a fully migrated SQLite database for integration tests.
func setupIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "integration_test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open integration db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate integration db: %v", err)
	}
	return database.DB
}

// seedIntegrationUser creates a user and returns the ID.
func seedIntegrationUser(t *testing.T, testDB *sql.DB, username, role string) int64 {
	t.Helper()
	result, err := testDB.Exec(
		`INSERT INTO users (username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		username, "$2a$10$testhash", role,
	)
	if err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
	id, _ := result.LastInsertId()
	return id
}

// seedIntegrationDatasource creates a datasource and returns the ID.
func seedIntegrationDatasource(t *testing.T, testDB *sql.DB, name string) int64 {
	t.Helper()
	result, err := testDB.Exec(
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, status, created_at, updated_at) VALUES (?, 'mysql', 'localhost', 3306, 'root', '', 'active', datetime('now'), datetime('now'))`,
		name,
	)
	if err != nil {
		t.Fatalf("seed datasource %s: %v", name, err)
	}
	id, _ := result.LastInsertId()
	return id
}

// setIntegrationTicketStatus directly sets a ticket's status in the DB.
func setIntegrationTicketStatus(t *testing.T, testDB *sql.DB, ticketID int64, status model.TicketStatus) {
	t.Helper()
	_, err := testDB.Exec(`UPDATE tickets SET status = ?, updated_at = datetime('now') WHERE id = ?`, status, ticketID)
	if err != nil {
		t.Fatalf("setTicketStatus(%d, %s): %v", ticketID, status, err)
	}
}

// ---------------------------------------------------------------------------
// Integration Test 1: Full Ticket Lifecycle with Audit Trail
// Verifies: Ticket state machine + audit logging + permission checks
// ---------------------------------------------------------------------------

func TestIntegration_TicketLifecycleWithAudit(t *testing.T) {
	testDB := setupIntegrationDB(t)
	auditSvc := NewAuditService(testDB, 100, 50*time.Millisecond)
	defer auditSvc.Close()

	notifySvc := NewNotifyService("", "") // disabled
	ticketSvc := NewTicketService(testDB, auditSvc, notifySvc)

	devID := seedIntegrationUser(t, testDB, "developer1", "developer")
	dbaID := seedIntegrationUser(t, testDB, "dba1", "dba")
	dsID := seedIntegrationDatasource(t, testDB, "prod-mysql")

	t.Run("full_approve_lifecycle", func(t *testing.T) {
		// Step 1: Developer creates ticket
		ticket, err := ticketSvc.CreateTicket(context.Background(),
			devID, dsID, "appdb",
			"ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
			"mysql", "add phone field", "medium",
			`{"risk":"medium","decision":"ticket"}`,
		)
		if err != nil {
			t.Fatalf("CreateTicket: %v", err)
		}
		if ticket.Status != model.TicketStatusSubmitted {
			t.Fatalf("initial status = %s, want SUBMITTED", ticket.Status)
		}

		// Step 2: Simulate AI review -> PENDING_APPROVAL
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

		// Step 3: DBA approves
		ticket, err = ticketSvc.ApproveTicket(context.Background(),ticket.ID, dbaID, "dba", "LGTM")
		if err != nil {
			t.Fatalf("ApproveTicket: %v", err)
		}
		if ticket.Status != model.TicketStatusApproved {
			t.Fatalf("approved status = %s, want APPROVED", ticket.Status)
		}
		if ticket.ReviewerName != "dba1" {
			t.Errorf("ReviewerName = %s, want dba1", ticket.ReviewerName)
		}

		// Step 4: Developer executes
		ticket, err = ticketSvc.ExecuteTicket(context.Background(),ticket.ID, devID, "developer", "developer1")
		if err != nil {
			t.Fatalf("ExecuteTicket: %v", err)
		}
		if ticket.Status != model.TicketStatusDone {
			t.Fatalf("final status = %s, want DONE", ticket.Status)
		}
		if ticket.ExecutedAt == nil {
			t.Error("ExecutedAt should be set")
		}

		// Verify audit trail was written for each step
		time.Sleep(300 * time.Millisecond)

		rows, err := testDB.Query(
			`SELECT action FROM audit_logs WHERE sql_content LIKE '%ALTER TABLE users%' ORDER BY created_at ASC`,
		)
		if err != nil {
			t.Fatalf("query audit logs: %v", err)
		}
		defer rows.Close()

		var actions []string
		for rows.Next() {
			var action string
			if err := rows.Scan(&action); err != nil {
				t.Fatalf("scan action: %v", err)
			}
			actions = append(actions, action)
		}

		// Should have at least: ticket_create, ticket_approve, ticket_execute
		expectedActions := map[string]bool{
			"ticket_create":  false,
			"ticket_approve": false,
			"ticket_execute": false,
		}
		for _, a := range actions {
			if _, ok := expectedActions[a]; ok {
				expectedActions[a] = true
			}
		}
		for a, found := range expectedActions {
			if !found {
				t.Errorf("missing audit action %q in trail, got %v", a, actions)
			}
		}
	})

	t.Run("reject_lifecycle", func(t *testing.T) {
		ticket, err := ticketSvc.CreateTicket(context.Background(),
			devID, dsID, "appdb",
			"DELETE FROM logs WHERE created_at < '2024-01-01'",
			"mysql", "cleanup old logs", "high", "",
		)
		if err != nil {
			t.Fatalf("CreateTicket: %v", err)
		}

		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

		ticket, err = ticketSvc.RejectTicket(context.Background(),ticket.ID, dbaID, "dba", "too risky without limit")
		if err != nil {
			t.Fatalf("RejectTicket: %v", err)
		}
		if ticket.Status != model.TicketStatusRejected {
			t.Fatalf("status = %s, want REJECTED", ticket.Status)
		}

		// Rejected ticket cannot be executed
		_, err = ticketSvc.ExecuteTicket(context.Background(),ticket.ID, devID, "developer", "developer1")
		if err != ErrTicketNotExecutable {
			t.Errorf("ExecuteTicket on rejected ticket: error = %v, want ErrTicketNotExecutable", err)
		}
	})

	t.Run("cancel_lifecycle", func(t *testing.T) {
		ticket, err := ticketSvc.CreateTicket(context.Background(),
			devID, dsID, "appdb",
			"UPDATE users SET status = 1",
			"mysql", "bulk update", "high", "",
		)
		if err != nil {
			t.Fatalf("CreateTicket: %v", err)
		}

		// Developer cancels own ticket
		ticket, err = ticketSvc.CancelTicket(context.Background(),ticket.ID, devID, "developer", "changed my mind")
		if err != nil {
			t.Fatalf("CancelTicket: %v", err)
		}
		if ticket.Status != model.TicketStatusCancelled {
			t.Fatalf("status = %s, want CANCELLED", ticket.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// Integration Test 2: Permission Checks across Ticket Operations
// Verifies: Role-based access control for approve/reject/execute/cancel
// ---------------------------------------------------------------------------

func TestIntegration_PermissionChecks(t *testing.T) {
	testDB := setupIntegrationDB(t)
	ticketSvc := NewTicketService(testDB, nil, nil)

	devID := seedIntegrationUser(t, testDB, "dev_perm", "developer")
	dev2ID := seedIntegrationUser(t, testDB, "dev2_perm", "developer")
	dbaID := seedIntegrationUser(t, testDB, "dba_perm", "dba")
	adminID := seedIntegrationUser(t, testDB, "admin_perm", "admin")
	dsID := seedIntegrationDatasource(t, testDB, "perm-db")

	t.Run("developer_cannot_approve", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "medium", "")
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

		_, err := ticketSvc.ApproveTicket(context.Background(),ticket.ID, devID, "developer", "ok")
		if err != ErrNoPermission {
			t.Errorf("developer approve: error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("developer_cannot_reject", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "medium", "")
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

		_, err := ticketSvc.RejectTicket(context.Background(),ticket.ID, devID, "developer", "bad")
		if err != ErrNoPermission {
			t.Errorf("developer reject: error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("other_developer_cannot_execute", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "medium", "")
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusApproved)

		_, err := ticketSvc.ExecuteTicket(context.Background(),ticket.ID, dev2ID, "developer", "dev2_perm")
		if err != ErrNoPermission {
			t.Errorf("other dev execute: error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("other_developer_cannot_cancel", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "medium", "")

		_, err := ticketSvc.CancelTicket(context.Background(),ticket.ID, dev2ID, "developer", "cancel")
		if err != ErrNoPermission {
			t.Errorf("other dev cancel: error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("dba_can_approve_and_execute", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "medium", "")
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

		result, err := ticketSvc.ApproveTicket(context.Background(),ticket.ID, dbaID, "dba", "ok")
		if err != nil {
			t.Fatalf("dba approve: %v", err)
		}
		if result.Status != model.TicketStatusApproved {
			t.Errorf("status = %s, want APPROVED", result.Status)
		}

		result, err = ticketSvc.ExecuteTicket(context.Background(),ticket.ID, dbaID, "dba", "dba_perm")
		if err != nil {
			t.Fatalf("dba execute: %v", err)
		}
		if result.Status != model.TicketStatusDone {
			t.Errorf("status = %s, want DONE", result.Status)
		}
	})

	t.Run("admin_can_approve_and_execute", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "medium", "")
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

		result, err := ticketSvc.ApproveTicket(context.Background(),ticket.ID, adminID, "admin", "ok")
		if err != nil {
			t.Fatalf("admin approve: %v", err)
		}
		if result.Status != model.TicketStatusApproved {
			t.Errorf("status = %s, want APPROVED", result.Status)
		}

		result, err = ticketSvc.ExecuteTicket(context.Background(),ticket.ID, adminID, "admin", "admin_perm")
		if err != nil {
			t.Fatalf("admin execute: %v", err)
		}
		if result.Status != model.TicketStatusDone {
			t.Errorf("status = %s, want DONE", result.Status)
		}
	})

	t.Run("dba_can_cancel_any_ticket", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "medium", "")
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

		result, err := ticketSvc.CancelTicket(context.Background(),ticket.ID, dbaID, "dba", "not needed anymore")
		if err != nil {
			t.Fatalf("dba cancel: %v", err)
		}
		if result.Status != model.TicketStatusCancelled {
			t.Errorf("status = %s, want CANCELLED", result.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// Integration Test 3: SQL Parsing + AI Review + Ticket Creation
// Verifies: SQL parser -> AI review -> ticket creation pipeline
// ---------------------------------------------------------------------------

func TestIntegration_SQLParsingToTicketCreation(t *testing.T) {
	testDB := setupIntegrationDB(t)
	aiSvc := NewAIReviewService(testDB, "openai", "test-model", "", "", 5*time.Second)
	ticketSvc := NewTicketService(testDB, nil, nil)

	devID := seedIntegrationUser(t, testDB, "dev_sql", "developer")
	dsID := seedIntegrationDatasource(t, testDB, "sql-test-db")

	tests := []struct {
		name        string
		sql         string
		dbType      string
		wantBlocked bool
		wantRisk    string
	}{
		{
			name:        "safe select",
			sql:         "SELECT * FROM users LIMIT 10",
			dbType:      "mysql",
			wantBlocked: false,
			wantRisk:    "low",
		},
		{
			name:        "drop database blocked",
			sql:         "DROP DATABASE production",
			dbType:      "mysql",
			wantBlocked: true,
			wantRisk:    "high",
		},
		{
			name:        "update without where blocked",
			sql:         "UPDATE users SET active = 0",
			dbType:      "mysql",
			wantBlocked: true,
			wantRisk:    "high",
		},
		{
			name:        "insert needs ticket",
			sql:         "INSERT INTO users (name, email) VALUES ('test', 'test@example.com')",
			dbType:      "mysql",
			wantBlocked: false,
			wantRisk:    "medium",
		},
		{
			name:        "safe mongodb find",
			sql:         `{"operation": "find", "collection": "users", "filter": {"active": true}}`,
			dbType:      "mongodb",
			wantBlocked: false,
			wantRisk:    "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Parse SQL
			parseResult, err := sqlparser.ParseSQL(tt.sql, tt.dbType)
			if err != nil {
				t.Fatalf("ParseSQL: %v", err)
			}

			if parseResult.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v (reason: %s)", parseResult.IsBlocked, tt.wantBlocked, parseResult.BlockReason)
			}

			// Step 2: AI Review (static only since no API key)
			aiReq := &AIReviewRequest{
				SQL:         tt.sql,
				DBType:      tt.dbType,
				DatasourceID: dsID,
				Database:    "testdb",
				UserID:      devID,
				Operation:   parseResult.Operation,
				Tables:      parseResult.Tables,
				ParseResult: parseResult,
			}
			aiResult, err := aiSvc.Review(context.Background(), aiReq)
			if err != nil {
				t.Fatalf("AI Review: %v", err)
			}

			if tt.wantBlocked {
				if aiResult.Decision != DecisionBlocked {
					t.Errorf("Decision = %v, want blocked", aiResult.Decision)
				}
				// Blocked SQL should NOT create a ticket
				return
			}

			// Step 3: Create ticket for non-blocked SQL
			aiReviewJSON, _ := json.Marshal(aiResult)
			ticket, err := ticketSvc.CreateTicket(context.Background(),
				devID, dsID, "testdb", tt.sql, tt.dbType,
				"integration test", aiResult.RiskLevel, string(aiReviewJSON),
			)
			if err != nil {
				t.Fatalf("CreateTicket: %v", err)
			}
			if ticket.Status != model.TicketStatusSubmitted {
				t.Errorf("ticket status = %s, want SUBMITTED", ticket.Status)
			}
			if ticket.RiskLevel != aiResult.RiskLevel {
				t.Errorf("ticket risk = %s, want %s", ticket.RiskLevel, aiResult.RiskLevel)
			}

			// Step 4: Verify ticket can be retrieved
			got, err := ticketSvc.GetTicket(context.Background(),ticket.ID)
			if err != nil {
				t.Fatalf("GetTicket: %v", err)
			}
			if got.SQLContent != tt.sql {
				t.Errorf("SQLContent mismatch")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration Test 4: Mask Rules + Data Masking Pipeline
// Verifies: Mask rule CRUD + mask.ApplyToRows end-to-end
// ---------------------------------------------------------------------------

func TestIntegration_MaskRulesWithDataMasking(t *testing.T) {
	testDB := setupIntegrationDB(t)
	maskRuleSvc := NewMaskRuleService(testDB, nil, nil)

	dsID := seedIntegrationDatasource(t, testDB, "mask-test-db")

	t.Run("create_rules_and_apply_masking", func(t *testing.T) {
		// Create mask rules for different field types
		rules := []struct {
			table  string
			field  string
			mtype  string
			regex  string
			tmpl   string
		}{
			{"users", "phone", "phone", "", ""},
			{"users", "email", "email", "", ""},
			{"users", "name", "name", "", ""},
			{"orders", "card_no", "bank_card", "", ""},
			{"orders", "amount", "full", "", ""},
		}

		for _, r := range rules {
			_, err := maskRuleSvc.CreateMaskRule(context.Background(),0, dsID, "appdb", r.table, r.field, r.mtype, r.regex, r.tmpl)
			if err != nil {
				t.Fatalf("CreateMaskRule(%s.%s): %v", r.table, r.field, err)
			}
		}

		// List all rules
		allRules, total, err := maskRuleSvc.ListMaskRules(context.Background(),1, 10, "", "", "")
		if err != nil {
			t.Fatalf("ListMaskRules: %v", err)
		}
		if total != 5 {
			t.Errorf("total rules = %d, want 5", total)
		}
		if len(allRules) != 5 {
			t.Errorf("len(rules) = %d, want 5", len(allRules))
		}

		// Match rules for "users" table
		matchedRules := make([]mask.Rule, 0)
		for _, r := range allRules {
			if r.TableName == "users" || r.TableName == "*" {
				matchedRules = append(matchedRules, mask.Rule{
					Field:      r.Field,
					MaskType:   mask.MaskType(r.MaskType),
					TableName:  r.TableName,
				})
			}
		}

		// Apply masking to user rows
		rows := []map[string]interface{}{
			{"phone": "13812341234", "email": "zhangsan@example.com", "name": "张三", "age": 30},
			{"phone": "13987654321", "email": "lisi@test.com", "name": "李四", "age": 25},
		}

		maskedFields := mask.ApplyToRows(rows, matchedRules)
		if len(maskedFields) == 0 {
			t.Error("expected some fields to be masked")
		}

		// Verify phone masking
		if rows[0]["phone"] != "138****1234" {
			t.Errorf("phone[0] = %v, want 138****1234", rows[0]["phone"])
		}
		if rows[1]["phone"] != "139****4321" {
			t.Errorf("phone[1] = %v, want 139****4321", rows[1]["phone"])
		}

		// Verify email masking
		email0, ok := rows[0]["email"].(string)
		if !ok || email0 == "zhangsan@example.com" {
			t.Errorf("email[0] should be masked, got %v", rows[0]["email"])
		}

		// Verify name masking
		if rows[0]["name"] != "张*" {
			t.Errorf("name[0] = %v, want 张*", rows[0]["name"])
		}

		// Verify age NOT masked (no rule for it)
		if rows[0]["age"] != 30 {
			t.Errorf("age[0] should not be masked, got %v", rows[0]["age"])
		}
	})

	t.Run("custom_regex_mask_rule", func(t *testing.T) {
		_, err := maskRuleSvc.CreateMaskRule(context.Background(),0, dsID, "appdb", "products", "serial_no", "custom", `(\w{3})\w+(\w{3})`, "$1***$2")
		if err != nil {
			t.Fatalf("CreateMaskRule custom: %v", err)
		}

		rule := mask.Rule{Field: "serial_no", MaskType: mask.MaskCustom, CustomRegex: `(\w{3})\w+(\w{3})`, CustomTemplate: "$1***$2", TableName: "products"}
		rows := []map[string]interface{}{
			{"serial_no": "ABCDEFGH123"},
		}
		mask.ApplyToRows(rows, []mask.Rule{rule})

		if rows[0]["serial_no"] == "ABCDEFGH123" {
			t.Error("serial_no should be masked")
		}
	})

	t.Run("update_and_delete_mask_rule", func(t *testing.T) {
		rule, err := maskRuleSvc.CreateMaskRule(context.Background(),0, dsID, "appdb", "test_table", "field1", "full", "", "")
		if err != nil {
			t.Fatalf("CreateMaskRule: %v", err)
		}

		// Update
		updated, err := maskRuleSvc.UpdateMaskRule(context.Background(),0, rule.ID, "", "", "phone", "", "")
		if err != nil {
			t.Fatalf("UpdateMaskRule: %v", err)
		}
		if updated.MaskType != "phone" {
			t.Errorf("MaskType = %q, want phone", updated.MaskType)
		}

		// Delete
		err = maskRuleSvc.DeleteMaskRule(context.Background(),0, rule.ID)
		if err != nil {
			t.Fatalf("DeleteMaskRule: %v", err)
		}

		_, err = maskRuleSvc.GetMaskRule(context.Background(),rule.ID)
		if err != ErrMaskRuleNotFound {
			t.Errorf("GetMaskRule after delete: error = %v, want ErrMaskRuleNotFound", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Integration Test 5: Sensitive Tables + AI Review Integration
// Verifies: Sensitive table detection affects AI review risk level
// ---------------------------------------------------------------------------

func TestIntegration_SensitiveTableAffectsAIReview(t *testing.T) {
	testDB := setupIntegrationDB(t)

	dsID := seedIntegrationDatasource(t, testDB, "sensitive-db")

	// Create a mask rule that marks "users" as sensitive via mask_rules table
	// (AI review checks mask_rules for sensitive table detection)
	_, err := testDB.Exec(
		`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type, created_at, updated_at) VALUES (?, '', 'users', 'phone', 'phone', datetime('now'), datetime('now'))`,
		dsID,
	)
	if err != nil {
		t.Fatalf("insert mask rule: %v", err)
	}

	aiSvc := NewAIReviewService(testDB, "openai", "test-model", "", "", 5*time.Second)

	t.Run("select_on_sensitive_table_upgraded_risk", func(t *testing.T) {
		req := &AIReviewRequest{
			SQL:          "SELECT * FROM users LIMIT 10",
			DBType:       "mysql",
			DatasourceID: dsID,
			Operation:    sqlparser.OpSelect,
			Tables:       []string{"users"},
			ParseResult: &sqlparser.SQLParseResult{
				DBType:    "mysql",
				Operation: sqlparser.OpSelect,
				Tables:    []string{"users"},
				RiskLevel: sqlparser.RiskLow,
				Warnings:  make([]string, 0),
			},
		}

		result, err := aiSvc.Review(context.Background(), req)
		if err != nil {
			t.Fatalf("Review: %v", err)
		}

		// Sensitive table should upgrade risk from low to medium
		if result.RiskLevel != AIRiskMedium {
			t.Errorf("risk = %v, want %v (sensitive table should upgrade risk)", result.RiskLevel, AIRiskMedium)
		}
		if result.Decision != DecisionConfirm {
			t.Errorf("decision = %v, want %v", result.Decision, DecisionConfirm)
		}
	})
}

// ---------------------------------------------------------------------------
// Integration Test 6: Audit Service + Ticket Service Integration
// Verifies: Audit logs are correctly written for all ticket operations
// ---------------------------------------------------------------------------

func TestIntegration_AuditServiceBatchWithTicketOps(t *testing.T) {
	testDB := setupIntegrationDB(t)

	// Use AuditService's batch writer
	auditSvc := NewAuditService(testDB, 3, 50*time.Millisecond)

	// Write audit records through the batch service
	for i := 0; i < 7; i++ {
		auditSvc.Write(context.Background(),AuditRecord{
			UserID:     1,
			Action:     "query_execute",
			SQLContent: fmt.Sprintf("SELECT %d", i),
			SQLSummary: fmt.Sprintf("SELECT %d", i),
		})
	}

	// Close triggers flush
	auditSvc.Close()

	// Verify all records were persisted
	var count int64
	if err := testDB.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE action = 'query_execute'").Scan(&count); err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count != 7 {
		t.Errorf("audit log count = %d, want 7", count)
	}

	// Now verify audit logs from ticket operations
	seedIntegrationUser(t, testDB, "audit_user", "developer")
	dsID := seedIntegrationDatasource(t, testDB, "audit-db")

	ticketAuditSvc := NewAuditService(testDB, 100, 50*time.Millisecond)
	defer ticketAuditSvc.Close()
	ticketSvc := NewTicketService(testDB, ticketAuditSvc, nil)
	devID := seedIntegrationUser(t, testDB, "audit_dev", "developer")

	_, err := ticketSvc.CreateTicket(context.Background(),devID, dsID, "mydb", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	// Wait for async audit log
	time.Sleep(300 * time.Millisecond)

	var ticketAuditCount int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE action = 'ticket_create'").Scan(&ticketAuditCount); err != nil {
		t.Fatalf("count ticket audit: %v", err)
	}
	if ticketAuditCount != 1 {
		t.Errorf("ticket_create audit count = %d, want 1", ticketAuditCount)
	}
}

// ---------------------------------------------------------------------------
// Integration Test 7: SQL Parser Risk Assessment End-to-End
// Verifies: Various SQL types produce correct risk/block decisions
// ---------------------------------------------------------------------------

func TestIntegration_SQLParserRiskAssessment(t *testing.T) {
	tests := []struct {
		name        string
		sql         string
		dbType      string
		wantBlocked bool
		wantRisk    sqlparser.RiskLevel
	}{
		// Safe operations
		{"safe_select", "SELECT id, name FROM users WHERE active = 1 LIMIT 50", "mysql", false, sqlparser.RiskLow},
		{"safe_select_join", "SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id LIMIT 100", "mysql", false, sqlparser.RiskLow},
		{"safe_insert", "INSERT INTO users (name) VALUES ('test')", "mysql", false, sqlparser.RiskMedium},
		{"safe_update_with_where", "UPDATE users SET name = 'new' WHERE id = 1", "mysql", false, sqlparser.RiskMedium},
		{"safe_delete_with_where", "DELETE FROM logs WHERE id = 1", "mysql", false, sqlparser.RiskMedium},

		// Blocked operations
		{"drop_database", "DROP DATABASE production", "mysql", true, sqlparser.RiskHigh},
		{"drop_table", "DROP TABLE users", "mysql", true, sqlparser.RiskHigh},
		{"truncate", "TRUNCATE TABLE users", "mysql", true, sqlparser.RiskHigh},
		{"update_no_where", "UPDATE users SET active = 0", "mysql", true, sqlparser.RiskHigh},
		{"delete_no_where", "DELETE FROM users", "mysql", true, sqlparser.RiskHigh},

		// MongoDB
		{"mongo_safe_find", `{"operation": "find", "collection": "users", "filter": {"active": true}}`, "mongodb", false, sqlparser.RiskLow},
		{"mongo_blocked_update_empty", `{"operation": "update", "collection": "users", "multi": true, "filter": {}, "update": {"$set": {"x": 1}}}`, "mongodb", true, sqlparser.RiskHigh},
		{"mongo_blocked_dangerous_stage", `{"operation": "aggregate", "collection": "users", "pipeline": [{"$out": "backup"}]}`, "mongodb", true, sqlparser.RiskHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sqlparser.ParseSQL(tt.sql, tt.dbType)
			if err != nil {
				t.Fatalf("ParseSQL: %v", err)
			}
			if result.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v (reason: %s)", result.IsBlocked, tt.wantBlocked, result.BlockReason)
			}
			if result.RiskLevel != tt.wantRisk {
				t.Errorf("RiskLevel = %v, want %v", result.RiskLevel, tt.wantRisk)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration Test 8: Notification + Ticket Lifecycle
// Verifies: Notifications are sent at each ticket lifecycle stage
// ---------------------------------------------------------------------------

func TestIntegration_NotificationWithTicketLifecycle(t *testing.T) {
	testDB := setupIntegrationDB(t)

	var mu sync.Mutex
	var notifications []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req dingTalkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Markdown != nil {
			mu.Lock()
			notifications = append(notifications, req.Markdown.Title)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	notifySvc := NewNotifyService(server.URL, "test-secret")
	auditSvc := NewAuditService(testDB, 100, 50*time.Millisecond)
	defer auditSvc.Close()
	ticketSvc := NewTicketService(testDB, auditSvc, notifySvc)

	devID := seedIntegrationUser(t, testDB, "notify_dev", "developer")
	dbaID := seedIntegrationUser(t, testDB, "notify_dba", "dba")
	dsID := seedIntegrationDatasource(t, testDB, "notify-db")

	// Create ticket -> notification sent
	ticket, err := ticketSvc.CreateTicket(context.Background(),
		devID, dsID, "mydb",
		"ALTER TABLE users ADD COLUMN age INT",
		"mysql", "add age field", "medium", "",
	)
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	// Move to PENDING_APPROVAL
	setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

	// Approve -> notification sent
	ticket, err = ticketSvc.ApproveTicket(context.Background(),ticket.ID, dbaID, "dba", "approved")
	if err != nil {
		t.Fatalf("ApproveTicket: %v", err)
	}

	// Execute -> notification sent
	ticket, err = ticketSvc.ExecuteTicket(context.Background(),ticket.ID, devID, "developer", "notify_dev")
	if err != nil {
		t.Fatalf("ExecuteTicket: %v", err)
	}

	// Wait for async notifications
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(notifications) < 3 {
		t.Errorf("expected at least 3 notifications, got %d: %v", len(notifications), notifications)
	}
}

// ---------------------------------------------------------------------------
// Integration Test 9: AI Review with Mock LLM -> Ticket Decision
// Verifies: Full AI review pipeline produces correct ticket decisions
// ---------------------------------------------------------------------------

func TestIntegration_AIReviewWithMockLLMToDecision(t *testing.T) {
	testDB := setupIntegrationDB(t)

	tests := []struct {
		name         string
		aiResponse   string
		sql          string
		operation    sqlparser.OperationType
		wantDecision ReviewDecision
		wantRisk     string
	}{
		{
			name:         "low_risk_auto_execute",
			aiResponse:   `{"risk_level": "low", "risk_score": 10, "summary": "safe select", "suggestions": [], "impact_analysis": "none", "rollback_sql": ""}`,
			sql:          "SELECT * FROM users LIMIT 10",
			operation:    sqlparser.OpSelect,
			wantDecision: DecisionExecute,
			wantRisk:     AIRiskLow,
		},
		{
			name:         "high_risk_requires_ticket",
			aiResponse:   `{"risk_level": "high", "risk_score": 85, "summary": "dangerous DDL", "suggestions": ["backup first"], "impact_analysis": "table lock", "rollback_sql": "ALTER TABLE users DROP COLUMN phone"}`,
			sql:          "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
			operation:    sqlparser.OpDDL,
			wantDecision: DecisionTicket,
			wantRisk:     AIRiskHigh,
		},
		{
			name:         "medium_risk_needs_confirm",
			aiResponse:   `{"risk_level": "medium", "risk_score": 45, "summary": "update with where", "suggestions": ["verify where clause"], "impact_analysis": "modifies data", "rollback_sql": ""}`,
			sql:          "UPDATE users SET name = 'test' WHERE id = 1",
			operation:    sqlparser.OpUpdate,
			wantDecision: DecisionTicket,
			wantRisk:     AIRiskMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aiResp := chatCompletionsResponse{
				ID: "test-id",
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{
					{Message: chatMessage{Role: "assistant", Content: tt.aiResponse}},
				},
				Model: "test-model",
			}

			handler := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(aiResp)
			}

			server := httptest.NewServer(http.HandlerFunc(handler))
			defer server.Close()

			svc := NewAIReviewService(testDB, "openai", "test-model", "test-api-key", server.URL, 5*time.Second)
			svc.client = server.Client()
			svc.client.Timeout = 5 * time.Second

			req := &AIReviewRequest{
				SQL:       tt.sql,
				DBType:    "mysql",
				Operation: tt.operation,
				Tables:    []string{"users"},
				ParseResult: &sqlparser.SQLParseResult{
					DBType:    "mysql",
					Operation: tt.operation,
					Tables:    []string{"users"},
					RiskLevel: sqlparser.RiskLow,
					Warnings:  make([]string, 0),
				},
			}

			result, err := svc.Review(context.Background(), req)
			if err != nil {
				t.Fatalf("Review: %v", err)
			}

			if result.Decision != tt.wantDecision {
				t.Errorf("Decision = %v, want %v", result.Decision, tt.wantDecision)
			}
			if result.RiskLevel != tt.wantRisk {
				t.Errorf("RiskLevel = %v, want %v", result.RiskLevel, tt.wantRisk)
			}
			if result.ReviewSource != "ai" {
				t.Errorf("ReviewSource = %v, want ai", result.ReviewSource)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration Test 10: Masking + Audit Log Integration
// Verifies: Desensitized fields are recorded in audit logs
// ---------------------------------------------------------------------------

func TestIntegration_MaskingAndAuditLog(t *testing.T) {
	testDB := setupIntegrationDB(t)
	auditSvc := NewAuditService(testDB, 100, 50*time.Millisecond)

	// Setup mask rules
	maskRules := []mask.Rule{
		{Field: "phone", MaskType: mask.MaskPhone, TableName: "users"},
		{Field: "email", MaskType: mask.MaskEmail, TableName: "users"},
	}

	// Simulate a query result that needs masking
	rows := []map[string]interface{}{
		{"phone": "13812341234", "email": "test@example.com", "name": "张三"},
	}

	maskedFields := mask.ApplyToRows(rows, maskRules)

	// Write audit log with masked fields info
	auditSvc.Write(context.Background(),AuditRecord{
		UserID:            1,
		Action:            "query_execute",
		SQLContent:        "SELECT phone, email, name FROM users",
		DesensitizedFields: fmt.Sprintf("%v", maskedFields),
	})

	auditSvc.Close()

	// Verify audit log has masking info
	var desensitized string
	err := testDB.QueryRow(
		"SELECT desensitized_fields FROM audit_logs WHERE action = 'query_execute' LIMIT 1",
	).Scan(&desensitized)
	if err != nil {
		t.Fatalf("scan desensitized_fields: %v", err)
	}
	if desensitized == "" {
		t.Error("expected desensitized_fields to be recorded")
	}

	// Verify data was actually masked
	if rows[0]["phone"] != "138****1234" {
		t.Errorf("phone not masked: %v", rows[0]["phone"])
	}
}

// ---------------------------------------------------------------------------
// Integration Test 11: State Machine Completeness
// Verifies: Every valid state transition is testable end-to-end
// ---------------------------------------------------------------------------

func TestIntegration_StateMachineCompleteness(t *testing.T) {
	testDB := setupIntegrationDB(t)
	ticketSvc := NewTicketService(testDB, nil, nil)

	devID := seedIntegrationUser(t, testDB, "sm_dev", "developer")
	_ = seedIntegrationUser(t, testDB, "sm_dba", "dba")
	dsID := seedIntegrationDatasource(t, testDB, "sm-db")

	t.Run("submitted_to_ai_reviewed", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")
		if !CanTransition(model.TicketStatusSubmitted, model.TicketStatusAIReviewed) {
			t.Error("SUBMITTED -> AI_REVIEWED should be valid")
		}
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusAIReviewed)
		got, _ := ticketSvc.GetTicket(context.Background(),ticket.ID)
		if got.Status != model.TicketStatusAIReviewed {
			t.Errorf("status = %s, want AI_REVIEWED", got.Status)
		}
	})

	t.Run("ai_reviewed_to_pending_approval", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusAIReviewed)
		if !CanTransition(model.TicketStatusAIReviewed, model.TicketStatusPendingApproval) {
			t.Error("AI_REVIEWED -> PENDING_APPROVAL should be valid")
		}
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)
		got, _ := ticketSvc.GetTicket(context.Background(),ticket.ID)
		if got.Status != model.TicketStatusPendingApproval {
			t.Errorf("status = %s, want PENDING_APPROVAL", got.Status)
		}
	})

	t.Run("approved_to_executing_to_done", func(t *testing.T) {
		ticket, _ := ticketSvc.CreateTicket(context.Background(),devID, dsID, "db", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")
		setIntegrationTicketStatus(t, testDB, ticket.ID, model.TicketStatusApproved)

		// APPROVED -> EXECUTING is a valid transition
		if !CanTransition(model.TicketStatusApproved, model.TicketStatusExecuting) {
			t.Error("APPROVED -> EXECUTING should be valid")
		}

		// ExecuteTicket goes directly to DONE
		result, err := ticketSvc.ExecuteTicket(context.Background(),ticket.ID, devID, "developer", "sm_dev")
		if err != nil {
			t.Fatalf("ExecuteTicket: %v", err)
		}
		if result.Status != model.TicketStatusDone {
			t.Errorf("status = %s, want DONE", result.Status)
		}

		// DONE is terminal
		if CanTransition(model.TicketStatusDone, model.TicketStatusSubmitted) {
			t.Error("DONE -> SUBMITTED should not be valid")
		}
		if CanTransition(model.TicketStatusDone, model.TicketStatusCancelled) {
			t.Error("DONE -> CANCELLED should not be valid")
		}
	})

	t.Run("terminal_states_no_transitions", func(t *testing.T) {
		terminals := []model.TicketStatus{
			model.TicketStatusDone,
			model.TicketStatusRejected,
			model.TicketStatusCancelled,
		}
		allStatuses := []model.TicketStatus{
			model.TicketStatusSubmitted,
			model.TicketStatusAIReviewed,
			model.TicketStatusPendingApproval,
			model.TicketStatusApproved,
			model.TicketStatusExecuting,
			model.TicketStatusDone,
			model.TicketStatusRejected,
			model.TicketStatusCancelled,
		}
		for _, terminal := range terminals {
			for _, target := range allStatuses {
				if CanTransition(terminal, target) {
					t.Errorf("terminal state %s should have no transitions, but can transition to %s", terminal, target)
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Integration Test 12: Concurrent Ticket Operations
// Verifies: Multiple ticket operations don't cause data races
// ---------------------------------------------------------------------------

func TestIntegration_ConcurrentTicketOperations(t *testing.T) {
	testDB := setupIntegrationDB(t)
	ticketSvc := NewTicketService(testDB, nil, nil)

	devID := seedIntegrationUser(t, testDB, "concurrent_dev", "developer")
	dbaID := seedIntegrationUser(t, testDB, "concurrent_dba", "dba")
	dsID := seedIntegrationDatasource(t, testDB, "concurrent-db")

	const numTickets = 10
	var wg sync.WaitGroup
	ticketIDs := make([]int64, numTickets)
	errors := make([]error, numTickets)

	// Create tickets concurrently
	for i := 0; i < numTickets; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ticket, err := ticketSvc.CreateTicket(context.Background(),
				devID, dsID, "db",
				fmt.Sprintf("ALTER TABLE t ADD COLUMN col_%d INT", idx),
				"mysql", fmt.Sprintf("test %d", idx), "medium", "",
			)
			if err != nil {
				errors[idx] = err
				return
			}
			ticketIDs[idx] = ticket.ID
		}(i)
	}
	wg.Wait()

	// Verify all tickets created
	for i, err := range errors {
		if err != nil {
			t.Errorf("ticket %d creation failed: %v", i, err)
		}
	}

	// List and verify count
	tickets, total, err := ticketSvc.ListTickets(context.Background(),1, 100, "", "", "", "", "", "", devID, "developer")
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if total != numTickets {
		t.Errorf("total = %d, want %d", total, numTickets)
	}
	if len(tickets) != numTickets {
		t.Errorf("len(tickets) = %d, want %d", len(tickets), numTickets)
	}

	// Approve all concurrently
	for i := range ticketIDs {
		setIntegrationTicketStatus(t, testDB, ticketIDs[i], model.TicketStatusPendingApproval)
	}

	for i := range ticketIDs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := ticketSvc.ApproveTicket(context.Background(),ticketIDs[idx], dbaID, "dba", fmt.Sprintf("approved %d", idx))
			if err != nil {
				errors[idx] = err
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("ticket %d approval failed: %v", i, err)
		}
	}
}
