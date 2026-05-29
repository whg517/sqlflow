package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/whg517/sqlflow/internal/db"
)

func setupSnapshotTestDB(t *testing.T) *SnapshotService {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return NewSnapshotService(database.DB)
}

func TestSnapshotService_CreateAndGet(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id","name","age"]`)
	rows := json.RawMessage(`[[1,"Alice",30],[2,"Bob",25]]`)

	snap, err := svc.CreateSnapshot(ctx, 1, "test snapshot", columns, rows, 2)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if snap.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if snap.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", snap.RowCount)
	}

	// Get it back
	got, err := svc.GetSnapshot(ctx, snap.ID, 1)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if got.Label != "test snapshot" {
		t.Errorf("Label = %s, want test snapshot", got.Label)
	}
}

func TestSnapshotService_GetSnapshot_Forbidden(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id"]`)
	rows := json.RawMessage(`[[1]]`)

	snap, _ := svc.CreateSnapshot(ctx, 1, "owned by user 1", columns, rows, 1)

	// User 2 cannot access user 1's snapshot
	_, err := svc.GetSnapshot(ctx, snap.ID, 2)
	if err != ErrSnapshotForbidden {
		t.Errorf("expected ErrSnapshotForbidden, got %v", err)
	}
}

func TestSnapshotService_GetSnapshot_NotFound(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	_, err := svc.GetSnapshot(ctx, 99999, 1)
	if err != ErrSnapshotNotFound {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestSnapshotService_ListSnapshots(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id"]`)
	rows := json.RawMessage(`[[1]]`)

	svc.CreateSnapshot(ctx, 1, "snap1", columns, rows, 1)
	svc.CreateSnapshot(ctx, 1, "snap2", columns, rows, 1)
	svc.CreateSnapshot(ctx, 2, "other user", columns, rows, 1)

	snapshots, total, err := svc.ListSnapshots(ctx, 1, 1, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(snapshots) != 2 {
		t.Errorf("len(snapshots) = %d, want 2", len(snapshots))
	}
}

func TestSnapshotService_DeleteSnapshot(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id"]`)
	rows := json.RawMessage(`[[1]]`)

	snap, _ := svc.CreateSnapshot(ctx, 1, "to delete", columns, rows, 1)

	if err := svc.DeleteSnapshot(ctx, snap.ID, 1); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	_, err := svc.GetSnapshot(ctx, snap.ID, 1)
	if err != ErrSnapshotNotFound {
		t.Errorf("expected ErrSnapshotNotFound after delete, got %v", err)
	}
}

func TestSnapshotService_DeleteSnapshot_Forbidden(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id"]`)
	rows := json.RawMessage(`[[1]]`)

	snap, _ := svc.CreateSnapshot(ctx, 1, "owned", columns, rows, 1)

	err := svc.DeleteSnapshot(ctx, snap.ID, 2)
	if err != ErrSnapshotForbidden {
		t.Errorf("expected ErrSnapshotForbidden, got %v", err)
	}
}

func TestSnapshotService_CompareSnapshots_Unchanged(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id","name"]`)
	rows := json.RawMessage(`[[1,"Alice"],[2,"Bob"]]`)

	snapA, _ := svc.CreateSnapshot(ctx, 1, "a", columns, rows, 2)
	snapB, _ := svc.CreateSnapshot(ctx, 1, "b", columns, rows, 2)

	result, err := svc.CompareSnapshots(ctx, snapA.ID, snapB.ID, 1)
	if err != nil {
		t.Fatalf("CompareSnapshots: %v", err)
	}

	if result.Summary.Unchanged != 2 {
		t.Errorf("unchanged = %d, want 2", result.Summary.Unchanged)
	}
	if result.Summary.Added != 0 || result.Summary.Removed != 0 || result.Summary.Modified != 0 {
		t.Errorf("expected all zero for added/removed/modified, got added=%d removed=%d modified=%d",
			result.Summary.Added, result.Summary.Removed, result.Summary.Modified)
	}
}

func TestSnapshotService_CompareSnapshots_AddedRemoved(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id","name"]`)

	rowsA := json.RawMessage(`[[1,"Alice"],[2,"Bob"]]`)
	rowsB := json.RawMessage(`[[1,"Alice"],[3,"Charlie"]]`)

	snapA, _ := svc.CreateSnapshot(ctx, 1, "a", columns, rowsA, 2)
	snapB, _ := svc.CreateSnapshot(ctx, 1, "b", columns, rowsB, 2)

	result, err := svc.CompareSnapshots(ctx, snapA.ID, snapB.ID, 1)
	if err != nil {
		t.Fatalf("CompareSnapshots: %v", err)
	}

	if result.Summary.Unchanged != 1 {
		t.Errorf("unchanged = %d, want 1", result.Summary.Unchanged)
	}
	if result.Summary.Removed != 1 {
		t.Errorf("removed = %d, want 1", result.Summary.Removed)
	}
	if result.Summary.Added != 1 {
		t.Errorf("added = %d, want 1", result.Summary.Added)
	}
}

func TestSnapshotService_CompareSnapshots_Modified(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id","name","age"]`)

	rowsA := json.RawMessage(`[[1,"Alice",30],[2,"Bob",25]]`)
	rowsB := json.RawMessage(`[[1,"Alice",31],[2,"Bob",25]]`)

	snapA, _ := svc.CreateSnapshot(ctx, 1, "a", columns, rowsA, 2)
	snapB, _ := svc.CreateSnapshot(ctx, 1, "b", columns, rowsB, 2)

	result, err := svc.CompareSnapshots(ctx, snapA.ID, snapB.ID, 1)
	if err != nil {
		t.Fatalf("CompareSnapshots: %v", err)
	}

	if result.Summary.Modified != 1 {
		t.Errorf("modified = %d, want 1", result.Summary.Modified)
	}
	if result.Summary.Unchanged != 1 {
		t.Errorf("unchanged = %d, want 1", result.Summary.Unchanged)
	}

	// Check changed fields
	for _, dr := range result.DiffRows {
		if dr.Type == "modified" {
			if len(dr.ChangedFields) != 1 || dr.ChangedFields[0] != "age" {
				t.Errorf("changedFields = %v, want [age]", dr.ChangedFields)
			}
		}
	}
}

func TestSnapshotService_CompareSnapshots_SchemaMismatch(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	colsA := json.RawMessage(`["id","name"]`)
	colsB := json.RawMessage(`["id","email"]`)

	rows := json.RawMessage(`[[1,"x"]]`)

	snapA, _ := svc.CreateSnapshot(ctx, 1, "a", colsA, rows, 1)
	snapB, _ := svc.CreateSnapshot(ctx, 1, "b", colsB, rows, 1)

	_, err := svc.CompareSnapshots(ctx, snapA.ID, snapB.ID, 1)
	if err != ErrSchemaMismatch {
		t.Errorf("expected ErrSchemaMismatch, got %v", err)
	}
}

func TestSnapshotService_CompareSnapshots_SameSnapshot(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id"]`)
	rows := json.RawMessage(`[[1]]`)

	snap, _ := svc.CreateSnapshot(ctx, 1, "a", columns, rows, 1)

	_, err := svc.CompareSnapshots(ctx, snap.ID, snap.ID, 1)
	if err != ErrSameSnapshot {
		t.Errorf("expected ErrSameSnapshot, got %v", err)
	}
}

func TestSnapshotService_CompareSnapshots_Forbidden(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	columns := json.RawMessage(`["id"]`)
	rows := json.RawMessage(`[[1]]`)

	snapA, _ := svc.CreateSnapshot(ctx, 1, "a", columns, rows, 1)
	snapB, _ := svc.CreateSnapshot(ctx, 2, "b", columns, rows, 1)

	// User 1 cannot compare with user 2's snapshot
	_, err := svc.CompareSnapshots(ctx, snapA.ID, snapB.ID, 1)
	if err != ErrSnapshotForbidden {
		t.Errorf("expected ErrSnapshotForbidden, got %v", err)
	}
}

func TestCompareRows_AllTypes(t *testing.T) {
	columns := []string{"id", "name", "score"}
	rowsA := [][]interface{}{
		{1, "Alice", 90.0},
		{2, "Bob", 85.0},
		{3, "Charlie", 70.0},
	}
	rowsB := [][]interface{}{
		{1, "Alice", 90.0},    // unchanged
		{2, "Bob", 88.0},      // modified (score changed)
		{4, "Diana", 95.0},    // added (new row)
		// Charlie removed
	}

	result := compareRows(columns, rowsA, rowsB)

	if result.Summary.Unchanged != 1 {
		t.Errorf("unchanged = %d, want 1", result.Summary.Unchanged)
	}
	if result.Summary.Modified != 1 {
		t.Errorf("modified = %d, want 1", result.Summary.Modified)
	}
	if result.Summary.Removed != 1 {
		t.Errorf("removed = %d, want 1", result.Summary.Removed)
	}
	if result.Summary.Added != 1 {
		t.Errorf("added = %d, want 1", result.Summary.Added)
	}
	if result.TotalLeft != 3 {
		t.Errorf("totalLeft = %d, want 3", result.TotalLeft)
	}
	if result.TotalRight != 3 {
		t.Errorf("totalRight = %d, want 3", result.TotalRight)
	}
}
