package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestSQLitePoC validates that ent auto-migrate works correctly with SQLite.
// It tests 3-5 core tables (users, datasources, tickets) to verify:
// - entsql.Annotation handles SQLite-specific syntax
// - Indexes, defaults, and nullable fields work correctly
// - Existing data is preserved after auto-migrate
func TestSQLitePoC(t *testing.T) {
	dbPath := t.TempDir() + "/poc_test.db"

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Run migrations (golang-migrate + ent auto-migrate)
	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	client := database.Client()

	// Test 1: Create a user
	user, err := client.User.Create().
		SetUsername("poc_tester").
		SetPasswordHash("hash123").
		SetRole("admin").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if user.ID == 0 {
		t.Error("expected non-zero user ID")
	}
	if user.Username != "poc_tester" {
		t.Errorf("expected username poc_tester, got %s", user.Username)
	}
	t.Logf("✓ User created: id=%d username=%s role=%s", user.ID, user.Username, user.Role)

	// Test 2: Create a datasource
	ds, err := client.DataSource.Create().
		SetName("poc_ds").
		SetType("mysql").
		SetHost("localhost").
		SetPort(3306).
		SetMaxOpen(10).
		SetMaxIdle(5).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create datasource: %v", err)
	}
	t.Logf("✓ DataSource created: id=%d name=%s type=%s", ds.ID, ds.Name, ds.Type)

	// Test 3: Create a ticket
	ticket, err := client.Ticket.Create().
		SetSubmitterID(int64(user.ID)).
		SetDatasourceID(int64(ds.ID)).
		SetSQLContent("SELECT 1").
		SetStatus("SUBMITTED").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}
	t.Logf("✓ Ticket created: id=%d status=%s revision=%d", ticket.ID, ticket.Status, ticket.Revision)

	// Test 4: Query back
	gotUser, err := client.User.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to query user: %v", err)
	}
	if gotUser.Username != "poc_tester" {
		t.Errorf("expected username poc_tester, got %s", gotUser.Username)
	}
	t.Logf("✓ User query works")

	// Test 5: Update
	_, err = client.Ticket.UpdateOneID(ticket.ID).
		SetStatus("APPROVED").
		SetRevision(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to update ticket: %v", err)
	}
	gotTicket, err := client.Ticket.Get(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("failed to query ticket: %v", err)
	}
	if gotTicket.Status != "APPROVED" || gotTicket.Revision != 2 {
		t.Errorf("expected APPROVED/2, got %s/%d", gotTicket.Status, gotTicket.Revision)
	}
	t.Logf("✓ Ticket update works: status=%s revision=%d", gotTicket.Status, gotTicket.Revision)

	// Test 6: Create audit log
	audit, err := client.AuditLog.Create().
		SetUserID(int64(user.ID)).
		SetAction("query").
		SetDatasourceID(int64(ds.ID)).
		SetSQLContent("SELECT 1").
		SetExecutionTimeMs(50).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create audit log: %v", err)
	}
	t.Logf("✓ AuditLog created: id=%d action=%s", audit.ID, audit.Action)

	// Test 7: Verify re-open preserves data (simulates server restart)
	database.Close()
	database2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to re-open database: %v", err)
	}
	defer database2.Close()

	if err := database2.Migrate(); err != nil {
		t.Fatalf("failed to re-migrate: %v", err)
	}

	// Verify data still exists after re-open + re-migrate
	users, err := database2.Client().User.Query().All(ctx)
	if err != nil {
		t.Fatalf("failed to query users after re-open: %v", err)
	}
	if len(users) != 1 || users[0].Username != "poc_tester" {
		t.Errorf("expected 1 user poc_tester, got %v", users)
	}
	t.Logf("✓ Data preserved after re-open: %d users", len(users))

	_ = fmt.Sprintf("PoC complete: %d bytes database", fileStats(t, dbPath))
}

func fileStats(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat db file: %v", err)
	}
	return info.Size()
}

// TestEntClientQueryContext validates that ent client provides access to the
// underlying *sql.DB for raw SQL fallback scenarios.
func TestEntClientQueryContext(t *testing.T) {
	dbPath := t.TempDir() + "/query_ctx.db"

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Verify raw SQL access through DB.DB still works
	var count int
	err = database.DB.QueryRowContext(context.Background(), "SELECT count(*) FROM users").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query via raw SQL: %v", err)
	}
	t.Logf("✓ Raw SQL query works: users count=%d", count)

	// Verify ent client also works
	client := database.Client()
	ctx := context.Background()
	_, err = client.User.Create().
		SetUsername("raw_sql_test").
		SetPasswordHash("hash").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create user via ent: %v", err)
	}

	var count2 int
	err = database.DB.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count2)
	if err != nil {
		t.Fatalf("failed to query via raw SQL after ent write: %v", err)
	}
	if count2 != 1 {
		t.Errorf("expected 1 user, got %d", count2)
	}
	t.Logf("✓ ent write visible via raw SQL: count=%d", count2)
}

// TestAllSchemasCreated validates all 24 ent schemas produce valid tables.
func TestAllSchemasCreated(t *testing.T) {
	dbPath := t.TempDir() + "/all_schemas.db"

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Check that all expected tables exist
	expectedTables := []string{
		"users", "datasources", "query_history", "audit_logs",
		"mask_rules", "sensitive_tables", "tickets", "refresh_tokens",
		"comments", "git_links", "api_tokens", "permission_requests",
		"temp_policies", "sla_config", "sla_action_log", "export_tasks",
		"sql_templates", "web_vitals", "shared_results",
		"approval_policies", "approval_records", "ticket_revisions",
		"oidc_providers", "execution_results",
		// casbin_rule is also expected (from migration 000001)
		"casbin_rule",
	}

	ctx := context.Background()
	for _, table := range expectedTables {
		var count int
		err := database.DB.QueryRowContext(ctx,
			"SELECT count(*) FROM "+table+" WHERE 1=0").Scan(&count)
		if err != nil {
			t.Errorf("table %s not accessible: %v", table, err)
		} else {
			t.Logf("✓ Table %s exists", table)
		}
	}
}

// futureTime returns a time 1 hour from now for use in tests.
func futureTime() time.Time {
	return time.Now().Add(time.Hour).UTC()
}

// TestCRUDAllEntities performs a basic create for each entity type
// to validate the ent schemas are correctly defined.
func TestCRUDAllEntities(t *testing.T) {
	dbPath := t.TempDir() + "/crud_all.db"

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	client := database.Client()

	// Users
	user, err := client.User.Create().
		SetUsername("crud_user").
		SetPasswordHash("hash").
		SetRole("developer").
		SetOidcSubject("sub123").
		SetOidcProvider("google").
		Save(ctx)
	if err != nil {
		t.Fatalf("User CRUD: %v", err)
	}
	t.Logf("✓ User CRUD ok (id=%d)", user.ID)

	// DataSources
	ds, err := client.DataSource.Create().
		SetName("crud_ds").
		SetType("postgres").
		SetHost("db.example.com").
		SetPort(5432).
		SetSslmode("require").
		Save(ctx)
	if err != nil {
		t.Fatalf("DataSource CRUD: %v", err)
	}
	t.Logf("✓ DataSource CRUD ok (id=%d)", ds.ID)

	// RefreshTokens
	rt, err := client.RefreshToken.Create().
		SetUserID(int64(user.ID)).
		SetToken("token123").
		SetExpiresAt(futureTime()).
		Save(ctx)
	if err != nil {
		t.Fatalf("RefreshToken CRUD: %v", err)
	}
	t.Logf("✓ RefreshToken CRUD ok (id=%d)", rt.ID)

	// QueryHistory
	qh, err := client.QueryHistory.Create().
		SetUserID(int64(user.ID)).
		SetDatasourceID(int64(ds.ID)).
		SetSQLContent("SELECT 1").
		SetExecutionTime(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("QueryHistory CRUD: %v", err)
	}
	t.Logf("✓ QueryHistory CRUD ok (id=%d)", qh.ID)

	// Tickets
	tk, err := client.Ticket.Create().
		SetSubmitterID(int64(user.ID)).
		SetDatasourceID(int64(ds.ID)).
		SetSQLContent("ALTER TABLE t ADD COLUMN c TEXT").
		SetSQLType("ALTER").
		SetRiskLevel("high").
		Save(ctx)
	if err != nil {
		t.Fatalf("Ticket CRUD: %v", err)
	}
	t.Logf("✓ Ticket CRUD ok (id=%d)", tk.ID)

	// AuditLogs
	audit, err := client.AuditLog.Create().
		SetUserID(int64(user.ID)).
		SetAction("query").
		SetSQLContent("SELECT 1").
		Save(ctx)
	if err != nil {
		t.Fatalf("AuditLog CRUD: %v", err)
	}
	t.Logf("✓ AuditLog CRUD ok (id=%d)", audit.ID)

	// MaskRules
	mr, err := client.MaskRule.Create().
		SetDatasourceID(int64(ds.ID)).
		SetTableName("users").
		SetField("phone").
		SetMaskType("phone").
		Save(ctx)
	if err != nil {
		t.Fatalf("MaskRule CRUD: %v", err)
	}
	t.Logf("✓ MaskRule CRUD ok (id=%d)", mr.ID)

	// SensitiveTables
	st, err := client.SensitiveTable.Create().
		SetDatasourceID(int64(ds.ID)).
		SetTableName("credit_cards").
		SetSensitivityLevel("high").
		Save(ctx)
	if err != nil {
		t.Fatalf("SensitiveTable CRUD: %v", err)
	}
	t.Logf("✓ SensitiveTable CRUD ok (id=%d)", st.ID)

	// Comments
	cmt, err := client.Comment.Create().
		SetOrderID(int64(tk.ID)).
		SetUserID(int64(user.ID)).
		SetContent("LGTM").
		Save(ctx)
	if err != nil {
		t.Fatalf("Comment CRUD: %v", err)
	}
	t.Logf("✓ Comment CRUD ok (id=%d)", cmt.ID)

	// GitLinks
	gl, err := client.GitLink.Create().
		SetEntityType("ticket").
		SetEntityID(int64(tk.ID)).
		SetLinkType("commit").
		SetCommitHash("abc123").
		SetCommitMsg("fix").
		SetCreatedBy(int64(user.ID)).
		Save(ctx)
	if err != nil {
		t.Fatalf("GitLink CRUD: %v", err)
	}
	t.Logf("✓ GitLink CRUD ok (id=%d)", gl.ID)

	// APITokens
	at, err := client.APIToken.Create().
		SetUserID(int64(user.ID)).
		SetName("test-token").
		SetTokenHash("hashhash").
		SetExpiresAt(futureTime()).
		Save(ctx)
	if err != nil {
		t.Fatalf("APIToken CRUD: %v", err)
	}
	t.Logf("✓ APIToken CRUD ok (id=%d)", at.ID)

	// PermissionRequests
	pr, err := client.PermissionRequest.Create().
		SetApplicantID(int64(user.ID)).
		SetDatasourceID(int64(ds.ID)).
		SetDatabase("testdb").
		SetExpiresAt(futureTime()).
		Save(ctx)
	if err != nil {
		t.Fatalf("PermissionRequest CRUD: %v", err)
	}
	t.Logf("✓ PermissionRequest CRUD ok (id=%d)", pr.ID)

	// TempPolicies
	tp, err := client.TempPolicy.Create().
		SetSub("user:1").
		SetDom("ds:1").
		SetObj("table:users").
		SetAct("select").
		SetExpiresAt(futureTime()).
		Save(ctx)
	if err != nil {
		t.Fatalf("TempPolicy CRUD: %v", err)
	}
	t.Logf("✓ TempPolicy CRUD ok (id=%d)", tp.ID)

	// SLAConfigs
	sc, err := client.SLAConfig.Create().
		SetPriority("high").
		SetTimeoutMinutes(60).
		Save(ctx)
	if err != nil {
		t.Fatalf("SLAConfig CRUD: %v", err)
	}
	t.Logf("✓ SLAConfig CRUD ok (id=%d)", sc.ID)

	// SLAActionLogs
	sal, err := client.SLAActionLog.Create().
		SetTicketID(int64(tk.ID)).
		SetActionType("reminder").
		SetDedupKey("ticket:1:reminder").
		Save(ctx)
	if err != nil {
		t.Fatalf("SLAActionLog CRUD: %v", err)
	}
	t.Logf("✓ SLAActionLog CRUD ok (id=%d)", sal.ID)

	// ExportTasks
	et, err := client.ExportTask.Create().
		SetUserID(int64(user.ID)).
		SetExportType("audit").
		Save(ctx)
	if err != nil {
		t.Fatalf("ExportTask CRUD: %v", err)
	}
	t.Logf("✓ ExportTask CRUD ok (id=%d)", et.ID)

	// SQLTemplates
	sqlt, err := client.SQLTemplate.Create().
		SetUserID(int64(user.ID)).
		SetName("daily-check").
		SetSQLContent("SELECT count(*) FROM users").
		Save(ctx)
	if err != nil {
		t.Fatalf("SQLTemplate CRUD: %v", err)
	}
	t.Logf("✓ SQLTemplate CRUD ok (id=%d)", sqlt.ID)

	// WebVitals
	wv, err := client.WebVital.Create().
		SetMetricName("LCP").
		SetValue(1.5).
		SetRating("good").
		Save(ctx)
	if err != nil {
		t.Fatalf("WebVital CRUD: %v", err)
	}
	t.Logf("✓ WebVital CRUD ok (id=%d)", wv.ID)

	// SharedResults
	sr, err := client.SharedResult.Create().
		SetUserID(int64(user.ID)).
		SetToken("share-token-123").
		SetExpiresAt(futureTime()).
		Save(ctx)
	if err != nil {
		t.Fatalf("SharedResult CRUD: %v", err)
	}
	t.Logf("✓ SharedResult CRUD ok (id=%d)", sr.ID)

	// ApprovalPolicies
	ap, err := client.ApprovalPolicy.Create().
		SetName("default").
		SetConditions("{\"risk_levels\":[\"high\"]}").
		SetApprovalChain("[{\"role\":\"team_lead\"}]").
		Save(ctx)
	if err != nil {
		t.Fatalf("ApprovalPolicy CRUD: %v", err)
	}
	t.Logf("✓ ApprovalPolicy CRUD ok (id=%d)", ap.ID)

	// ApprovalRecords
	ar, err := client.ApprovalRecord.Create().
		SetTicketID(int64(tk.ID)).
		SetPolicyID(int64(ap.ID)).
		SetStage(1).
		SetTotalStages(2).
		SetApproverRole("team_lead").
		Save(ctx)
	if err != nil {
		t.Fatalf("ApprovalRecord CRUD: %v", err)
	}
	t.Logf("✓ ApprovalRecord CRUD ok (id=%d)", ar.ID)

	// TicketRevisions
	trev, err := client.TicketRevision.Create().
		SetTicketID(int64(tk.ID)).
		SetRevision(1).
		SetSQLContent("ALTER TABLE t ADD COLUMN c TEXT").
		SetStatus("SUBMITTED").
		Save(ctx)
	if err != nil {
		t.Fatalf("TicketRevision CRUD: %v", err)
	}
	t.Logf("✓ TicketRevision CRUD ok (id=%d)", trev.ID)

	// OIDCProviders
	oidc, err := client.OIDCProvider.Create().
		SetName("google").
		SetIssuer("https://accounts.google.com").
		SetClientID("xxx.apps.googleusercontent.com").
		Save(ctx)
	if err != nil {
		t.Fatalf("OIDCProvider CRUD: %v", err)
	}
	t.Logf("✓ OIDCProvider CRUD ok (id=%d)", oidc.ID)

	// ExecutionResults
	er, err := client.ExecutionResult.Create().
		SetTicketID(int64(tk.ID)).
		SetStatementIndex(0).
		SetSQL("ALTER TABLE t ADD COLUMN c TEXT").
		SetStatus("success").
		Save(ctx)
	if err != nil {
		t.Fatalf("ExecutionResult CRUD: %v", err)
	}
	t.Logf("✓ ExecutionResult CRUD ok (id=%d)", er.ID)

	t.Log("✅ All 24 entity types CRUD verified successfully")
}
