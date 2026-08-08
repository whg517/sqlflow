package mysql_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/whg517/sqlflow/internal/driver"
	mysqldriver "github.com/whg517/sqlflow/internal/driver/mysql"
)

// MySQL's half of the ExecuteStatements contract, which is the opposite of
// PostgreSQL's and equally binding: MySQL commits DDL implicitly, so there is
// no transaction to roll back. Each statement stands, execution continues past
// a failure, and the first error is what the caller is told.
//
// That difference is the reason internal/ticket delegates instead of writing
// its own loop, and neither half had a test. A ticket that failed halfway
// through a MySQL batch left the target database in a state nothing verified.

func newMockDriver(t *testing.T) (driver.StatementExecutor, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	d, ok := mysqldriver.NewWithDB(db).(driver.StatementExecutor)
	if !ok {
		t.Fatal("the MySQL driver no longer satisfies StatementExecutor")
	}
	return d, mock
}

// TestExecuteStatements_ContinuesPastAFailure is the clause that separates this
// driver from PostgreSQL's.
//
// Stopping at the first error would be wrong here rather than merely different:
// MySQL has already committed the earlier statements, so refusing to run the
// rest leaves the change half-applied with no record of what was skipped.
func TestExecuteStatements_ContinuesPastAFailure(t *testing.T) {
	d, mock := newMockDriver(t)

	mock.ExpectExec("ALTER TABLE a ADD c INT").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("ALTER TABLE missing ADD c INT").WillReturnError(errNoSuchTable{})
	mock.ExpectExec("ALTER TABLE c ADD c INT").WillReturnResult(sqlmock.NewResult(0, 3))

	results, err := d.ExecuteStatements(context.Background(), []string{
		"ALTER TABLE a ADD c INT",
		"ALTER TABLE missing ADD c INT",
		"ALTER TABLE c ADD c INT",
	})
	if err == nil {
		t.Fatal("a batch containing a failure reported success")
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3 — execution stopped at the failure", len(results))
	}
	if results[0].Status != "success" {
		t.Errorf("result 0 = %q, want success; MySQL committed it and nothing rolls it back", results[0].Status)
	}
	if results[1].Status != "error" {
		t.Errorf("result 1 = %q, want error", results[1].Status)
	}
	if results[2].Status != "success" {
		t.Errorf("result 2 = %q, want success — the batch continues past a failure", results[2].Status)
	}

	// No rollback: there is no transaction. sqlmock would report an unexpected
	// Rollback here, which is the point of asserting expectations.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestExecuteStatements_ReportsTheFirstError pins which of several failures the
// caller is told about, since the audit record carries exactly one message.
func TestExecuteStatements_ReportsTheFirstError(t *testing.T) {
	d, mock := newMockDriver(t)

	mock.ExpectExec("ALTER TABLE first ADD c INT").WillReturnError(errNoSuchTable{})
	mock.ExpectExec("ALTER TABLE second ADD c INT").WillReturnError(errSyntax{})

	_, err := d.ExecuteStatements(context.Background(), []string{
		"ALTER TABLE first ADD c INT",
		"ALTER TABLE second ADD c INT",
	})
	if err == nil {
		t.Fatal("a batch of failures reported success")
	}
	if got := err.Error(); !contains(got, errNoSuchTable{}.Error()) {
		t.Errorf("error = %q, want the FIRST failure (%q)", got, errNoSuchTable{}.Error())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

type errNoSuchTable struct{}

func (errNoSuchTable) Error() string { return "Error 1146: Table 'missing' doesn't exist" }

type errSyntax struct{}

func (errSyntax) Error() string { return "Error 1064: You have an error in your SQL syntax" }
