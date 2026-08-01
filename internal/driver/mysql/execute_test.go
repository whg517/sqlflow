package mysql

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Row scanning, empty results and the row cap used to be covered through
// QueryService's connpool helpers. They are the driver's behavior, so they are
// pinned here now that the service delegates.

func TestExecuteQuery_ScansRowsAndColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "hello"),
	)

	d := NewWithDB(db)
	res, err := d.ExecuteQuery(context.Background(), "testdb", "SELECT 1 AS id, 'hello' AS name", 100)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if res.Total != 1 {
		t.Errorf("Total = %d, want 1", res.Total)
	}
	if len(res.Columns) != 2 || res.Columns[0] != "id" || res.Columns[1] != "name" {
		t.Errorf("Columns = %v, want [id name]", res.Columns)
	}
	if res.Rows[0]["name"] != "hello" {
		t.Errorf("row name = %v, want hello", res.Rows[0]["name"])
	}
}

func TestExecuteQuery_EmptyResultIsNotAnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}))

	res, err := NewWithDB(db).ExecuteQuery(context.Background(), "testdb", "SELECT 1 WHERE 1=0", 100)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if res.Total != 0 {
		t.Errorf("Total = %d, want 0", res.Total)
	}
	if res.Rows == nil {
		t.Error("Rows should be an empty slice, not nil — clients iterate it directly")
	}
}

func TestExecuteQuery_StopsAtRowLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"n"})
	for i := range 10 {
		rows.AddRow(i)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	res, err := NewWithDB(db).ExecuteQuery(context.Background(), "testdb", "SELECT n FROM t", 3)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if res.Total != 3 {
		t.Errorf("Total = %d, want 3 — the limit must be enforced by the driver", res.Total)
	}
}

func TestExecuteQuery_NotConnected(t *testing.T) {
	d := NewWithDB(nil)
	if _, err := d.ExecuteQuery(context.Background(), "db", "SELECT 1", 10); err == nil {
		t.Fatal("expected an error when the driver has no connection")
	}
}
