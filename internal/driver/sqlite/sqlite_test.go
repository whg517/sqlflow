package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "modernc.org/sqlite"
)

func TestDriverReadOnlyMetadataAndQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO users(name) VALUES ('Alice')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	d := &Driver{}
	if err := d.Connect(context.Background(), &driver.Config{Database: path}); err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	tables, err := d.ListTables(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0].Name != "users" {
		t.Fatalf("tables = %#v", tables)
	}

	columns, err := d.GetColumns(context.Background(), "", "users")
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 2 || columns[1].Name != "name" {
		t.Fatalf("columns = %#v", columns)
	}

	result, err := d.ExecuteQuery(context.Background(), "SELECT name FROM users", 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Rows[0]["name"] != "Alice" {
		t.Fatalf("result = %#v", result)
	}

	if _, err := d.ExecuteQuery(context.Background(), "DELETE FROM users", 10); err == nil {
		t.Fatal("expected query-only connection to reject writes")
	}
}
