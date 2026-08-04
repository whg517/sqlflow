package ticket

import (
	"testing"

	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

// TestListTicketsKeywordIsNotAPattern checks that a keyword made of LIKE
// metacharacters is matched literally.
//
// The keyword went into a LIKE pattern unescaped, so a search for "%" matched
// every ticket the caller could see. On the DBA and admin views that is every
// ticket in the system, returned as though it were a search result.
func TestListTicketsKeywordIsNotAPattern(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := New(Deps{DB: testutil.WrapSQL(t, testDB), Audit: auditlog.Discard})

	userID := seedTestUser(t, testDB, "searcher", "dba")
	dsID := seedTestDatasource(t, testDB, "search-ds")

	for _, sqlText := range []string{
		"UPDATE orders SET status = 1",
		"DELETE FROM payments WHERE id = 2",
		"ALTER TABLE users ADD COLUMN note text",
	} {
		if _, err := testDB.Exec(
			`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary,
			 db_type, change_reason, status, risk_level, created_at, updated_at)
			 VALUES ($1, $2, 'shop', $3, $3, 'mysql', '需求', 'SUBMITTED', 'low', now(), now())`,
			userID, dsID, sqlText,
		); err != nil {
			t.Fatalf("seed ticket: %v", err)
		}
	}

	for _, keyword := range []string{"%", "_"} {
		_, total, err := svc.ListTickets(t.Context(), 1, 50, "", "", "", "", keyword, "", userID, "dba")
		if err != nil {
			t.Fatalf("ListTickets(%q): %v", keyword, err)
		}
		if total != 0 {
			t.Errorf("keyword %q matched %d tickets, want 0 — the wildcard leaked into the pattern",
				keyword, total)
		}
	}
}

// TestListTicketsKeywordStillMatches guards against the escaping being applied
// so eagerly that ordinary searches stop working.
func TestListTicketsKeywordStillMatches(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := New(Deps{DB: testutil.WrapSQL(t, testDB), Audit: auditlog.Discard})

	userID := seedTestUser(t, testDB, "searcher", "dba")
	dsID := seedTestDatasource(t, testDB, "search-ds")

	if _, err := testDB.Exec(
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary,
		 db_type, change_reason, status, risk_level, created_at, updated_at)
		 VALUES ($1, $2, 'shop', 'UPDATE orders SET status = 1', '改单', 'mysql', '需求', 'SUBMITTED', 'low', now(), now())`,
		userID, dsID,
	); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	// Mixed case, because the search is advertised as case-insensitive.
	for _, keyword := range []string{"orders", "ORDERS", "需求"} {
		_, total, err := svc.ListTickets(t.Context(), 1, 50, "", "", "", "", keyword, "", userID, "dba")
		if err != nil {
			t.Fatalf("ListTickets(%q): %v", keyword, err)
		}
		if total != 1 {
			t.Errorf("keyword %q matched %d tickets, want 1", keyword, total)
		}
	}
}

// TestListTicketsNonGovernanceRoleCannotWidenScope pins the ownership boundary.
//
// The submitter filter arrives as a query parameter. For a developer it is
// discarded and replaced with their own id, so a crafted submitter_id cannot
// reach another user's tickets — the server is the only arbiter of this, and a
// hidden UI control is not a control.
func TestListTicketsNonGovernanceRoleCannotWidenScope(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := New(Deps{DB: testutil.WrapSQL(t, testDB), Audit: auditlog.Discard})

	alice := seedTestUser(t, testDB, "alice", "developer")
	bob := seedTestUser(t, testDB, "bob", "developer")
	dsID := seedTestDatasource(t, testDB, "scope-ds")

	for _, submitter := range []int64{alice, bob} {
		if _, err := testDB.Exec(
			`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary,
			 db_type, change_reason, status, risk_level, created_at, updated_at)
			 VALUES ($1, $2, 'shop', 'UPDATE t SET x = 1', '摘要', 'mysql', '需求', 'SUBMITTED', 'low', now(), now())`,
			submitter, dsID,
		); err != nil {
			t.Fatalf("seed ticket: %v", err)
		}
	}

	// Alice asks for Bob's tickets by id.
	tickets, total, err := svc.ListTickets(t.Context(), 1, 50, "", "", "2", "", "", "", alice, "developer")
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if total != 1 {
		t.Fatalf("alice sees %d tickets, want only her own", total)
	}
	if tickets[0].SubmitterID != alice {
		t.Errorf("alice received ticket submitted by %d", tickets[0].SubmitterID)
	}
}
