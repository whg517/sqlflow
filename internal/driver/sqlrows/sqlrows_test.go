package sqlrows

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestQuery_SignalsTruncation verifies that Query sets Truncated=true when the
// row limit cuts the result short, and Truncated=false when it does not.
//
// Before the fix, a truncated result and a complete result were
// indistinguishable in the response — both reported Total == len(Rows) and
// Truncated was not present. In an analysis context this is the most dangerous
// kind of silence: no one checks whether they only saw part of the data.
func TestQuery_SignalsTruncation(t *testing.T) {
	dsn := os.Getenv("SQLFLOW_TEST_DSN")
	if dsn == "" {
		t.Skip("SQLFLOW_TEST_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// generate_series produces a known number of rows. 50 rows with a limit of
	// 10 → truncated; 5 rows with a limit of 10 → not truncated.
	t.Run("truncated when rows exceed limit", func(t *testing.T) {
		result, err := Query(ctx, db, "postgresql",
			"SELECT i FROM generate_series(1, 50) AS t(i)", nil, 10)
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(result.Rows) != 10 {
			t.Errorf("got %d rows, want 10", len(result.Rows))
		}
		if !result.Truncated {
			t.Error("Truncated should be true when the result was cut at the limit")
		}
	})

	t.Run("not truncated when rows fit within limit", func(t *testing.T) {
		result, err := Query(ctx, db, "postgresql",
			"SELECT i FROM generate_series(1, 5) AS t(i)", nil, 10)
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(result.Rows) != 5 {
			t.Errorf("got %d rows, want 5", len(result.Rows))
		}
		if result.Truncated {
			t.Error("Truncated should be false when the result was NOT cut at the limit")
		}
	})

	// Exact boundary: 10 rows with limit 10 — the database has no more rows to
	// give, so this is NOT truncated even though len == limit.
	t.Run("not truncated at exact boundary", func(t *testing.T) {
		result, err := Query(ctx, db, "postgresql",
			"SELECT i FROM generate_series(1, 10) AS t(i)", nil, 10)
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(result.Rows) != 10 {
			t.Errorf("got %d rows, want 10", len(result.Rows))
		}
		// rows.Next() returned false because the cursor was exhausted, not
		// because we broke at the limit. Truncated must be false.
		if result.Truncated {
			t.Error("Truncated should be false at the exact boundary — the cursor was exhausted, not the limit")
		}
	})
}

// TestQuery_NilDB is the not-connected error path, and does not need a real
// database.
func TestQuery_NilDB(t *testing.T) {
	_, err := Query(context.Background(), nil, "postgresql",
		"SELECT 1", nil, 10)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
	if fmt.Sprint(err) != "postgresql: not connected" {
		t.Errorf("got %q, want 'postgresql: not connected'", err)
	}
}
