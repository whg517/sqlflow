package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
	"github.com/whg517/sqlflow/internal/testutil"
)

// AI review integration tests.
//
// They lived in internal/ticket, under a heading that described a "SQL parser →
// AI review → ticket creation pipeline". No such pipeline exists: AI review is
// advisory (ADR-0004), its only entrance is the query workbench, and nothing in
// the ticket lifecycle has ever called it. The tests moved here with the code
// they exercise; what they assert about parsing and static rules is unchanged.

func TestIntegration_SQLParsingToTicketCreation(t *testing.T) {
	testDB := testutil.NewDB(t).DB
	aiSvc := NewAIReviewService(testutil.WrapSQL(t, testDB), "openai", "test-model", "", "", 5*time.Second)
	devID := testutil.SeedUser(t, testDB, "dev_sql", "developer")
	dsID := testutil.SeedDatasource(t, testDB, "sql-test-db")

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
			parseResult, err := driver.ParseFor(tt.dbType, tt.sql)
			if err != nil {
				t.Fatalf("ParseSQL: %v", err)
			}

			if parseResult.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v (reason: %s)", parseResult.IsBlocked, tt.wantBlocked, parseResult.BlockReason)
			}

			// Step 2: AI Review (static only since no API key)
			aiReq := &AIReviewRequest{
				SQL:          tt.sql,
				DBType:       tt.dbType,
				DatasourceID: dsID,
				Database:     "testdb",
				UserID:       devID,
				Operation:    parseResult.Operation,
				Tables:       parseResult.Targets,
				ParseResult:  parseResult,
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

			// What the ticket does with this verdict is not asserted here.
			// It is advisory (ADR-0004) and the ticket's own risk level is
			// server-derived from the SQL, which internal/ticket's governance
			// tests already pin.
		})
	}
}

// ---------------------------------------------------------------------------
// Integration Test 4: Mask Rules + Data Masking Pipeline
// Verifies: Mask rule CRUD + mask.ApplyToRows end-to-end
// ---------------------------------------------------------------------------

func TestIntegration_SensitiveTableAffectsAIReview(t *testing.T) {
	testDB := testutil.NewDB(t).DB

	dsID := testutil.SeedDatasource(t, testDB, "sensitive-db")

	// Create a mask rule that marks "users" as sensitive via mask_rules table
	// (AI review checks mask_rules for sensitive table detection)
	_, err := testDB.Exec(
		`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type, created_at, updated_at) VALUES ($1, '', 'users', 'phone', 'phone', now(), now())`,
		dsID,
	)
	if err != nil {
		t.Fatalf("insert mask rule: %v", err)
	}

	aiSvc := NewAIReviewService(testutil.WrapSQL(t, testDB), "openai", "test-model", "", "", 5*time.Second)

	t.Run("select_on_sensitive_table_upgraded_risk", func(t *testing.T) {
		req := &AIReviewRequest{
			SQL:          "SELECT * FROM users LIMIT 10",
			DBType:       "mysql",
			DatasourceID: dsID,
			Operation:    driver.OpSelect,
			Tables:       []string{"users"},
			ParseResult: &driver.ParseResult{
				Operation: driver.OpSelect,
				Targets:   []string{"users"},
				RiskLevel: driver.RiskLow,
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

func TestIntegration_AIReviewWithMockLLMToDecision(t *testing.T) {
	testDB := testutil.NewDB(t).DB

	tests := []struct {
		name         string
		aiResponse   string
		sql          string
		operation    string
		wantDecision ReviewDecision
		wantRisk     string
	}{
		{
			name:         "low_risk_auto_execute",
			aiResponse:   `{"risk_level": "low", "risk_score": 10, "summary": "safe select", "suggestions": [], "impact_analysis": "none", "rollback_sql": ""}`,
			sql:          "SELECT * FROM users LIMIT 10",
			operation:    driver.OpSelect,
			wantDecision: DecisionExecute,
			wantRisk:     AIRiskLow,
		},
		{
			name:         "high_risk_requires_ticket",
			aiResponse:   `{"risk_level": "high", "risk_score": 85, "summary": "dangerous DDL", "suggestions": ["backup first"], "impact_analysis": "table lock", "rollback_sql": "ALTER TABLE users DROP COLUMN phone"}`,
			sql:          "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
			operation:    driver.OpDDL,
			wantDecision: DecisionTicket,
			wantRisk:     AIRiskHigh,
		},
		{
			name:         "medium_risk_needs_confirm",
			aiResponse:   `{"risk_level": "medium", "risk_score": 45, "summary": "update with where", "suggestions": ["verify where clause"], "impact_analysis": "modifies data", "rollback_sql": ""}`,
			sql:          "UPDATE users SET name = 'test' WHERE id = 1",
			operation:    driver.OpUpdate,
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

			svc := NewAIReviewService(testutil.WrapSQL(t, testDB), "openai", "test-model", "test-api-key", server.URL, 5*time.Second)
			svc.client = server.Client()
			svc.client.Timeout = 5 * time.Second

			req := &AIReviewRequest{
				SQL:       tt.sql,
				DBType:    "mysql",
				Operation: tt.operation,
				Tables:    []string{"users"},
				ParseResult: &driver.ParseResult{
					Operation: tt.operation,
					Targets:   []string{"users"},
					RiskLevel: driver.RiskLow,
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
