package ticket

import (
	"database/sql"
	"testing"

	"github.com/whg517/sqlflow/internal/platform/auditlog"
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

// newTicketServiceWithDatasources builds a Service against a real schema.
//
// It carried a permission service until the collection-level check came out:
// that check demanded the very permission a ticket is filed to request, so a
// read-only developer could not file one. What guards this path now is the
// datasource gate, which needs no policy to test.
func newTicketServiceWithDatasources(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	testDB := setupTicketTestDB(t)
	svc := New(Deps{
		DB:    testutil.WrapSQL(t, testDB),
		Audit: auditlog.Discard,
	})
	return svc, testDB
}

// TestCreateTicketRecordsDatasourceType pins where the type comes from.
//
// db_type used to arrive in the request body and select which checks ran, so a
// submitter could steer their own ticket by claiming a different one. It is not
// a parameter any more — the compiler enforces that — and this checks the value
// that lands in the row is the datasource's own.
func TestCreateTicketRecordsDatasourceType(t *testing.T) {
	svc, testDB := newTicketServiceWithDatasources(t)
	userID := seedTestUser(t, testDB, "dev-type", "admin")

	for _, dsType := range []string{"postgresql", "mysql", "mongodb"} {
		t.Run(dsType, func(t *testing.T) {
			dsID := seedTypedDatasource(t, testDB, dsType+"-target", dsType)
			sqlContent := "UPDATE t SET x = 1 WHERE id = 2"
			if dsType == "mongodb" {
				sqlContent = `{"operation":"update","collection":"t","filter":{"id":2},"update":{"$set":{"x":1}}}`
			}

			tk, err := svc.CreateTicket(t.Context(), userID, "admin", dsID, "app", sqlContent, "变更需求")
			if err != nil {
				t.Fatalf("CreateTicket: %v", err)
			}
			if tk.DBType != dsType {
				t.Errorf("db_type = %q, want %q — it must come from the datasource", tk.DBType, dsType)
			}
		})
	}
}

// TestCreateTicketRejectsUnknownDatasource closes the gap the type lookup
// exposed: nothing used to verify the datasource existed at all, so a ticket
// could be filed against any integer.
func TestCreateTicketRejectsUnknownDatasource(t *testing.T) {
	svc, testDB := newTicketServiceWithDatasources(t)
	userID := seedTestUser(t, testDB, "dev-ghost", "developer")

	if _, err := svc.CreateTicket(t.Context(), userID, "developer", 99999, "app", "SELECT 1", "变更需求"); err == nil {
		t.Error("a ticket was created against a datasource that does not exist")
	}
}
