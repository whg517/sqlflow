package ticket

import (
	"database/sql"
	"testing"

	"github.com/whg517/sqlflow/internal/testutil"
)

// seedGatedDatasource creates a datasource with an explicit status and extra
// config, so the gates can be exercised.
func seedGatedDatasource(t *testing.T, testDB *sql.DB, name, dsType, status, extra string) int64 {
	t.Helper()
	var id int64
	if err := testDB.QueryRow(
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted,
		 database, status, extra_config, created_at, updated_at)
		 VALUES ($1, $2, 'localhost', 5432, 'u', '', $3, $4, $5, now(), now()) RETURNING id`,
		name, dsType, testutil.DatasourceDatabase, status, extra,
	).Scan(&id); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	return id
}

// TestCreateTicketRefusesTheInternalDatasource pins the gate the query path has
// and this one did not.
//
// The platform's own metadata database is admin-only: it holds every ticket,
// approval record, audit row and encrypted credential. ExecuteQuery refuses it
// for anyone else; CreateTicket checked nothing at all, so a developer could
// file DROP TABLE tickets against it and put it in front of an approver as an
// ordinary change.
//
// Invariant: every entrance authorizes independently. A gate on the query path
// says nothing about the ticket path.
func TestCreateTicketRefusesTheInternalDatasource(t *testing.T) {
	svc, testDB := newTicketServiceWithDatasources(t)
	userID := seedTestUser(t, testDB, "dev", "developer")
	dsID := seedGatedDatasource(t, testDB, "SQLFlow 元数据库", "postgresql", "active", `{"system":true}`)

	_, err := svc.CreateTicket(t.Context(), userID, "developer", dsID, testutil.DatasourceDatabase,
		"DROP TABLE tickets", "需求")
	if err == nil {
		t.Fatal("a developer filed a ticket against the platform metadata database")
	}
}

// TestCreateTicketAllowsAdminOnTheInternalDatasource keeps the gate a gate
// rather than a wall: operating on the platform's own store is a real admin
// task.
func TestCreateTicketAllowsAdminOnTheInternalDatasource(t *testing.T) {
	svc, testDB := newTicketServiceWithDatasources(t)
	userID := seedTestUser(t, testDB, "root", "admin")
	dsID := seedGatedDatasource(t, testDB, "SQLFlow 元数据库", "postgresql", "active", `{"system":true}`)

	if _, err := svc.CreateTicket(t.Context(), userID, "admin", dsID, testutil.DatasourceDatabase,
		"UPDATE tickets SET status = 'DONE' WHERE id = 1", "需求"); err != nil {
		t.Fatalf("an admin was refused: %v", err)
	}
}

// TestCreateTicketRefusesADisabledDatasource covers the other gate the query
// path applies. A disabled datasource is one an operator has taken out of
// service; queueing changes against it produces work that cannot execute.
func TestCreateTicketRefusesADisabledDatasource(t *testing.T) {
	svc, testDB := newTicketServiceWithDatasources(t)
	userID := seedTestUser(t, testDB, "dev", "developer")
	dsID := seedGatedDatasource(t, testDB, "retired", "mysql", "disabled", "")

	if _, err := svc.CreateTicket(t.Context(), userID, "developer", dsID, testutil.DatasourceDatabase,
		"DELETE FROM users WHERE id = 1", "需求"); err == nil {
		t.Error("a ticket was filed against a disabled datasource")
	}
}

// TestCreateTicketIsReachableForAReadOnlyDeveloper is the other half, and the
// reason the collection-level check is gone.
//
// A developer is read-only by default and files a ticket precisely because they
// lack the write permission — that is what the workflow is for. The check
// demanded that permission up front, so a read-only developer could not file a
// MongoDB delete at all, while the same developer could file DROP TABLE against
// MySQL because SQL tickets had no equivalent check. The workflow was
// unreachable for the role it exists to serve, on one driver only.
func TestCreateTicketIsReachableForAReadOnlyDeveloper(t *testing.T) {
	svc, testDB := newTicketServiceWithDatasources(t)
	userID := seedTestUser(t, testDB, "dev", "developer")

	mongoID := seedGatedDatasource(t, testDB, "mongo", "mongodb", "active", "")
	mysqlID := seedGatedDatasource(t, testDB, "mysql-ds", "mysql", "active", "")

	tests := []struct {
		name string
		ds   int64
		sql  string
	}{
		{"mongo delete", mongoID, `{"operation":"delete","collection":"users","filter":{"id":1}}`},
		{"mongo insert", mongoID, `{"operation":"insert","collection":"users","document":{"a":1}}`},
		{"sql delete", mysqlID, `DELETE FROM users WHERE id = 1`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.CreateTicket(t.Context(), userID, "developer", tt.ds, testutil.DatasourceDatabase, tt.sql, "需求"); err != nil {
				t.Errorf("a read-only developer could not file the ticket the workflow exists for: %v", err)
			}
		})
	}
}
