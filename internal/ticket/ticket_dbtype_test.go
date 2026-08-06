package ticket

import (
	"database/sql"
	"testing"

	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/security"
	"github.com/whg517/sqlflow/internal/testutil"
)

// seedTypedDatasource creates a datasource of an explicit driver type.
//
// seedTestDatasource hardcodes mysql, which is exactly the shape that let the
// db_type hole go unnoticed: no ticket test ever had a datasource whose type
// disagreed with the value the caller passed in.
func seedTypedDatasource(t *testing.T, testDB *sql.DB, name, dsType string) int64 {
	t.Helper()
	var id int64
	if err := testDB.QueryRow(
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, status, created_at, updated_at)
		 VALUES ($1, $2, 'localhost', 27017, 'root', '', 'active', now(), now()) RETURNING id`,
		name, dsType,
	).Scan(&id); err != nil {
		t.Fatalf("seed %s datasource: %v", dsType, err)
	}
	return id
}

// seedTicketCasbinRules gives developer select-only, so any write action on a
// collection must be refused.
func seedTicketCasbinRules(t *testing.T, testDB *sql.DB) {
	t.Helper()
	policies := [][]string{
		{"p", "admin", "*", "*", "*"},
		{"p", "developer", "*", "*", "select"},
	}
	for _, p := range policies {
		if _, err := testDB.Exec(
			`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3) VALUES ($1, $2, $3, $4, $5)`,
			p[0], p[1], p[2], p[3], p[4],
		); err != nil {
			t.Fatalf("seed casbin rule: %v", err)
		}
	}
}

// newMongoPermissionTicketService wires the collaborators the collection-level
// check actually needs, rather than leaving permSvc nil.
func newMongoPermissionTicketService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	testDB := setupTicketTestDB(t)
	seedTicketCasbinRules(t, testDB)

	permSvc, err := security.NewService(testutil.WrapSQL(t, testDB))
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}
	svc := New(Deps{
		DB:         testutil.WrapSQL(t, testDB),
		Audit:      auditlog.Discard,
		Permission: permSvc,
	})
	return svc, testDB
}

// mongoDeleteBody removes every document in the collection.
const mongoDeleteBody = `{"operation":"delete","collection":"users","filter":{}}`

// TestCreateTicketDerivesDBTypeFromDatasource is the negative case the
// collection-level check existed for.
//
// db_type arrived in the request body and was the only thing that selected the
// MongoDB branch. A submitter who wrote "mysql" — or who simply omitted the
// field, since the default was "mysql" — skipped the Casbin check on the
// collection entirely and got the ticket into the approval queue.
//
// The server is the only arbiter of what a datasource is: the type has to come
// from the datasource row, never from the caller.
func TestCreateTicketDerivesDBTypeFromDatasource(t *testing.T) {
	svc, testDB := newMongoPermissionTicketService(t)
	userID := seedTestUser(t, testDB, "dev-liar", "developer")
	dsID := seedTypedDatasource(t, testDB, "mongo-target", "mongodb")

	_, err := svc.CreateTicket(t.Context(), userID, "developer", dsID, "app", mongoDeleteBody, "变更需求")
	if err == nil {
		t.Fatal("a developer without delete permission created a MongoDB delete ticket — " +
			"the collection-level check was skipped")
	}
}

// TestCreateTicketMongoPermissionAllowsPermittedAction guards against the check
// being applied so bluntly that legitimate tickets stop working.
func TestCreateTicketMongoPermissionAllowsPermittedAction(t *testing.T) {
	svc, testDB := newMongoPermissionTicketService(t)
	userID := seedTestUser(t, testDB, "dev-ok", "developer")
	dsID := seedTypedDatasource(t, testDB, "mongo-readable", "mongodb")

	body := `{"operation":"find","collection":"users","filter":{}}`
	if _, err := svc.CreateTicket(t.Context(), userID, "developer", dsID, "app", body, "变更需求"); err != nil {
		t.Fatalf("a permitted find was refused: %v", err)
	}
}

// TestCreateTicketRecordsDatasourceType pins the stored value, since db_type is
// read back on the execution path and shown to approvers.
func TestCreateTicketRecordsDatasourceType(t *testing.T) {
	svc, testDB := newMongoPermissionTicketService(t)
	userID := seedTestUser(t, testDB, "dev-type", "admin")
	dsID := seedTypedDatasource(t, testDB, "pg-target", "postgresql")

	tk, err := svc.CreateTicket(t.Context(), userID, "admin", dsID, "app", "UPDATE t SET x = 1 WHERE id = 2", "变更需求")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if tk.DBType != "postgresql" {
		t.Errorf("db_type = %q, want postgresql — it must come from the datasource", tk.DBType)
	}
}

// TestCreateTicketRejectsUnknownDatasource closes the gap the type lookup
// exposed: nothing used to verify the datasource existed at all.
func TestCreateTicketRejectsUnknownDatasource(t *testing.T) {
	svc, testDB := newMongoPermissionTicketService(t)
	userID := seedTestUser(t, testDB, "dev-ghost", "developer")

	if _, err := svc.CreateTicket(t.Context(), userID, "developer", 99999, "app", "SELECT 1", "变更需求"); err == nil {
		t.Error("a ticket was created against a datasource that does not exist")
	}
}
