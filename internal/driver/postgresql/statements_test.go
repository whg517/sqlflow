package postgresql_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/whg517/sqlflow/internal/driver"
	pgdriver "github.com/whg517/sqlflow/internal/driver/postgresql"
)

// driver.ExecuteStatements documents three different transaction semantics as a
// binding contract, and internal/ticket delegates to it explicitly rather than
// re-implementing them. Nothing tested any of it: the only occurrence of
// ExecuteStatements in a test was a fake in internal/ticket that records the
// call. What a ticket execution actually does to a target database — how far it
// gets, what it rolls back, what it reports afterwards — was unverified.
//
// This is PostgreSQL's half: one transaction, stop at the first error, roll
// back, and re-mark the statements that had already succeeded.

func newMockDriver(t *testing.T) (driver.StatementExecutor, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	d, ok := pgdriver.NewWithDB(db).(driver.StatementExecutor)
	if !ok {
		t.Fatal("the PostgreSQL driver no longer satisfies StatementExecutor")
	}
	return d, mock
}

// TestExecuteStatements_CommitsWhenEveryStatementSucceeds is the happy path, and
// the control for the rollback tests: without it, a driver that rolled back
// unconditionally would pass them.
func TestExecuteStatements_CommitsWhenEveryStatementSucceeds(t *testing.T) {
	d, mock := newMockDriver(t)

	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE a ADD c INT").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("ALTER TABLE b ADD c INT").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	results, err := d.ExecuteStatements(context.Background(),
		[]string{"ALTER TABLE a ADD c INT", "ALTER TABLE b ADD c INT"})
	if err != nil {
		t.Fatalf("ExecuteStatements: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for i, r := range results {
		if r.Status != "success" {
			t.Errorf("result %d status = %q, want success", i, r.Status)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestExecuteStatements_RollsBackAndRemarksSucceededStatements is the contract's
// most consequential clause.
//
// An operator reading a failed ticket needs to know that the statements before
// the failure did not survive. Reporting them as "success" next to a rolled-back
// transaction would be worse than reporting nothing: it names a change to the
// target database that is not there.
func TestExecuteStatements_RollsBackAndRemarksSucceededStatements(t *testing.T) {
	d, mock := newMockDriver(t)

	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE a ADD c INT").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("ALTER TABLE missing ADD c INT").WillReturnError(errNoSuchTable{})
	mock.ExpectRollback()

	results, err := d.ExecuteStatements(context.Background(),
		[]string{"ALTER TABLE a ADD c INT", "ALTER TABLE missing ADD c INT", "ALTER TABLE c ADD c INT"})
	if err == nil {
		t.Fatal("a failed batch reported success")
	}

	// Stops at the first error: the third statement is never attempted.
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 — execution continued past the first error", len(results))
	}
	if results[0].Status != "rolled_back" {
		t.Errorf("the statement that succeeded before the failure is reported as %q; "+
			"it did not survive the rollback and must not be called success", results[0].Status)
	}
	if results[1].Status != "error" {
		t.Errorf("the failing statement is reported as %q, want error", results[1].Status)
	}
	if results[1].Error == "" {
		t.Error("the failing statement carries no error text")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestExecuteStatements_SkipsEmptyStatements pins a behavior the splitter
// relies on: a trailing separator produces an empty unit, and running it would
// be a syntax error rather than a no-op.
func TestExecuteStatements_SkipsEmptyStatements(t *testing.T) {
	d, mock := newMockDriver(t)

	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE a ADD c INT").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	results, err := d.ExecuteStatements(context.Background(),
		[]string{"ALTER TABLE a ADD c INT", "", "   "})
	if err != nil {
		t.Fatalf("ExecuteStatements: %v", err)
	}
	// Fewer results than statements, deliberately: the caller pairs results with
	// statements by index elsewhere, so this asymmetry is worth stating.
	if len(results) != 1 {
		t.Errorf("got %d results, want 1 — an empty statement was executed", len(results))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

type errNoSuchTable struct{}

func (errNoSuchTable) Error() string { return `relation "missing" does not exist` }
