package ticket

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/model"
)

// A ticket's database field is what the approver reads to know where the change
// lands. It was free text and nothing checked it, while the executor's SQL
// drivers discarded it entirely — so the record could name one database and the
// change happen in another. These tests pin both ends of that.

// TestCreateTicket_RefusesAForeignDatabase keeps the unenforceable record from
// being written in the first place.
func TestCreateTicket_RefusesAForeignDatabase(t *testing.T) {
	platform, ticketSvc, _ := setupTicketExecTest(t)

	_, err := ticketSvc.CreateTicket(context.Background(), 1, "developer", 1,
		"some_other_db", "ALTER TABLE demo ADD COLUMN c INT", "变更")
	if !errors.Is(err, datasource.ErrDatabaseScopeMismatch) {
		t.Fatalf("CreateTicket error = %v, want ErrDatabaseScopeMismatch", err)
	}

	var count int
	if err := platform.QueryRow(`SELECT count(*) FROM tickets`).Scan(&count); err != nil {
		t.Fatalf("count tickets: %v", err)
	}
	if count != 0 {
		t.Errorf("%d ticket(s) were stored for a scope no executor can reach", count)
	}
}

// TestCreateTicket_NormalizesTheScopeToTheDatasource covers the caller who
// names nothing: the stored record must still say where the change will land,
// because that is what the approver is deciding about.
func TestCreateTicket_NormalizesTheScopeToTheDatasource(t *testing.T) {
	_, ticketSvc, _ := setupTicketExecTest(t)

	tk, err := ticketSvc.CreateTicket(context.Background(), 1, "developer", 1,
		"", "ALTER TABLE demo ADD COLUMN c INT", "变更")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if tk.Database != ticketExecDatabase {
		t.Errorf("Database = %q, want the datasource's %q", tk.Database, ticketExecDatabase)
	}
}

// TestExecuteTicket_RefusesATicketWhoseScopeCannotBeApplied covers the rows
// that already exist: a ticket filed before the field was checked can still
// name a database the connection never reaches. Running it against whatever the
// DSN points at would apply an approved change somewhere nobody approved, so it
// fails instead.
func TestExecuteTicket_RefusesATicketWhoseScopeCannotBeApplied(t *testing.T) {
	platform, ticketSvc, target := setupTicketExecTest(t)

	id := insertTicket(t, platform, model.TicketStatusApproved, "UPDATE demo SET n = 42", time.Time{})
	if _, err := platform.Exec(`UPDATE tickets SET database = $1 WHERE id = $2`, "elsewhere", id); err != nil {
		t.Fatalf("rewrite ticket scope: %v", err)
	}

	if _, err := ticketSvc.ExecuteTicket(context.Background(), id, 1, "admin", "admin"); err == nil {
		t.Fatal("a ticket naming an unreachable database was executed")
	}
	if got := ticketStatus(t, platform, id); got != string(model.TicketStatusFailed) {
		t.Errorf("status = %q, want FAILED", got)
	}
	// The fixture seeds n = 0, so an untouched target still reads 0.
	if n := demoValue(t, target); n != 0 {
		t.Errorf("demo.n = %d, want 0 — the statement ran anyway", n)
	}
}
