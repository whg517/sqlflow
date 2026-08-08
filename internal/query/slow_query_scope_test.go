package query

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/security"
	"github.com/whg517/sqlflow/internal/testutil"
)

// TestListSlowQueries_ShowsOnlyTheActorsOwnRows is the boundary the sibling
// entrance already applies.
//
// ListHistory scopes query_history to the caller. ListSlowQueries read the same
// table with no owner predicate at all and returned SQLContent verbatim — the
// statement text, WHERE-clause literals included. Its route carried no scope
// either, so any authenticated user could read every other user's SQL.
//
// The boundary belongs on the service and is derived from the actor, not left
// to whichever handler remembers it — that is the same argument
// ticketExportPredicates makes for itself.
func TestListSlowQueries_ShowsOnlyTheActorsOwnRows(t *testing.T) {
	svc, testDB, permSvc := newSlowQueryFixture(t)
	ctx := context.Background()

	mine := testutil.SeedUser(t, testDB, "slow_owner", "developer")
	theirs := testutil.SeedUser(t, testDB, "slow_other", "developer")
	dsID := testutil.SeedDatasource(t, testDB, "slow-ds")

	insertSlowQuery(t, testDB, mine, dsID, "SELECT * FROM mine WHERE ssn = '111-22-3333'")
	insertSlowQuery(t, testDB, theirs, dsID, "SELECT * FROM theirs WHERE ssn = '999-88-7777'")

	actor := ExportActor{UserID: mine, Username: "slow_owner", Role: "developer"}
	list, total, err := svc.ListSlowQueries(ctx, actor, SlowQueryParams{})
	if err != nil {
		t.Fatalf("ListSlowQueries: %v", err)
	}

	if total != 1 || len(list) != 1 {
		t.Fatalf("got %d rows (total %d), want only the actor's own", len(list), total)
	}
	for _, row := range list {
		if row.UserID != mine {
			t.Errorf("row belongs to user %d, not the caller", row.UserID)
		}
		if strings.Contains(row.SQLContent, "999-88-7777") {
			t.Error("another user's statement text was returned verbatim")
		}
	}

	_ = permSvc
}

// TestListSlowQueries_PlatformGrantWidensIt is the other half: a platform-wide
// view has to remain possible, and it has to be a grant rather than the absence
// of a check.
func TestListSlowQueries_PlatformGrantWidensIt(t *testing.T) {
	svc, testDB, _ := newSlowQueryFixture(t)
	ctx := context.Background()

	mine := testutil.SeedUser(t, testDB, "slow_admin", "admin")
	theirs := testutil.SeedUser(t, testDB, "slow_dev", "developer")
	dsID := testutil.SeedDatasource(t, testDB, "slow-ds-2")

	insertSlowQuery(t, testDB, mine, dsID, "SELECT 1")
	insertSlowQuery(t, testDB, theirs, dsID, "SELECT 2")

	actor := ExportActor{UserID: mine, Username: "slow_admin", Role: "admin"}
	_, total, err := svc.ListSlowQueries(ctx, actor, SlowQueryParams{})
	if err != nil {
		t.Fatalf("ListSlowQueries: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 — an admin holding the platform grant sees every row", total)
	}
}

func newSlowQueryFixture(t *testing.T) (*HistoryService, *sql.DB, *security.Service) {
	t.Helper()
	testDB := testutil.NewDB(t).DB
	seedCasbinRules(t, testDB)
	wrap := testutil.WrapSQL(t, testDB)

	permSvc, err := security.NewService(wrap)
	if err != nil {
		t.Fatalf("permission service: %v", err)
	}
	return NewHistoryServiceWithPerms(wrap, permSvc), testDB, permSvc
}

func insertSlowQuery(t *testing.T, conn *sql.DB, userID, dsID int64, sqlContent string) {
	t.Helper()
	if _, err := conn.Exec(
		`INSERT INTO query_history
		   (user_id, datasource_id, database, sql_content, sql_summary, db_type,
		    execution_time, result_rows, affected_rows, created_at)
		 VALUES ($1, $2, $3, $4, $5, 'mysql', 5000, 1, 0, now())`,
		userID, dsID, testutil.DatasourceDatabase, sqlContent, sqlContent,
	); err != nil {
		t.Fatalf("insert slow query: %v", err)
	}
}
