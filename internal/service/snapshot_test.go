package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/whg517/sqlflow/internal/db"
)

// mockQueryExecutor implements QueryExecutor for testing.
type mockQueryExecutor struct {
	result *QueryResult
	err    error
}

func (m *mockQueryExecutor) ExecuteQuery(ctx context.Context, userID int64, username, role string, datasourceID int64, database, sqlContent, dbType string) (*QueryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

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
	return NewSnapshotService(database.DB, &mockQueryExecutor{})
}

func setupSnapshotTestDBWithExecutor(t *testing.T, exec QueryExecutor) *SnapshotService {
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
	return NewSnapshotService(database.DB, exec)
}

func seedTestHistory(t *testing.T, testDB *sql.DB, userID, dsID int64, sqlContent, dbType string) int64 {
	t.Helper()
	result, err := testDB.Exec(
		`INSERT INTO query_history (user_id, datasource_id, database, sql_content, sql_summary, db_type, execution_time, result_rows, affected_rows) VALUES (?, ?, 'mydb', ?, '', ?, 100, 2, 0)`,
		userID, dsID, sqlContent, dbType,
	)
	if err != nil {
		t.Fatalf("seed test history: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}

func seedTestDatasourceForSnapshot(t *testing.T, testDB *sql.DB, name string) int64 {
	t.Helper()
	result, err := testDB.Exec(
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, database) VALUES (?, 'mysql', 'localhost', 3306, 'root', '', ?)`,
		name, name,
	)
	if err != nil {
		t.Fatalf("seed test datasource: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}

func TestSnapshotService_GetSnapshot_NotFound(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	_, err := svc.GetSnapshot(ctx, 99999, 1)
	if err != ErrSnapshotNotFound {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestSnapshotService_ListSnapshots_Empty(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	snapshots, total, err := svc.ListSnapshots(ctx, 1, 1, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(snapshots) != 0 {
		t.Errorf("len(snapshots) = %d, want 0", len(snapshots))
	}
}

func TestSnapshotService_DeleteSnapshot_NotFound(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	err := svc.DeleteSnapshot(ctx, 99999, 1)
	if err != ErrSnapshotNotFound {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestSnapshotService_CreateSnapshotFromHistory(t *testing.T) {
	// Set up mock executor
	mockExec := &mockQueryExecutor{
		result: &QueryResult{
			Columns: []string{"id", "name", "age"},
			Rows: []map[string]interface{}{
				{"id": float64(1), "name": "Alice", "age": float64(30)},
				{"id": float64(2), "name": "Bob", "age": float64(25)},
			},
			Total: 2,
		},
	}

	svc := setupSnapshotTestDBWithExecutor(t, mockExec)
	ctx := context.Background()

	// Seed datasource + history
	testDB := getDBFromService(t, svc)
	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID := seedTestHistory(t, testDB, 1, dsID, "SELECT * FROM users", "mysql")

	snap, err := svc.CreateSnapshotFromHistory(ctx, 1, "testuser", "admin", historyID)
	if err != nil {
		t.Fatalf("CreateSnapshotFromHistory: %v", err)
	}

	if snap.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if snap.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", snap.RowCount)
	}
	if snap.QueryHistoryID != historyID {
		t.Errorf("QueryHistoryID = %d, want %d", snap.QueryHistoryID, historyID)
	}
	if snap.SQLContent != "SELECT * FROM users" {
		t.Errorf("SQLContent = %s, want SELECT * FROM users", snap.SQLContent)
	}

	// Verify stored data
	got, err := svc.GetSnapshot(ctx, snap.ID, 1)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	var columns []string
	if err := json.Unmarshal(got.Columns, &columns); err != nil {
		t.Fatalf("unmarshal columns: %v", err)
	}
	if len(columns) != 3 || columns[0] != "id" {
		t.Errorf("columns = %v, want [id name age]", columns)
	}
}

func TestSnapshotService_CreateSnapshotFromHistory_Forbidden(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	testDB := getDBFromService(t, svc)
	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID := seedTestHistory(t, testDB, 1, dsID, "SELECT 1", "mysql")

	// User 2 tries to snapshot User 1's history
	_, err := svc.CreateSnapshotFromHistory(ctx, 2, "other", "admin", historyID)
	if err != ErrSnapshotForbidden {
		t.Errorf("expected ErrSnapshotForbidden, got %v", err)
	}
}

func TestSnapshotService_CreateSnapshotFromHistory_NotFound(t *testing.T) {
	svc := setupSnapshotTestDB(t)
	ctx := context.Background()

	_, err := svc.CreateSnapshotFromHistory(ctx, 1, "test", "admin", 99999)
	if err != ErrHistoryNotFound {
		t.Errorf("expected ErrHistoryNotFound, got %v", err)
	}
}

func TestSnapshotService_ListSnapshots(t *testing.T) {
	mockExec := &mockQueryExecutor{
		result: &QueryResult{
			Columns: []string{"id"},
			Rows:    []map[string]interface{}{{"id": float64(1)}},
			Total:   1,
		},
	}

	svc := setupSnapshotTestDBWithExecutor(t, mockExec)
	ctx := context.Background()
	testDB := getDBFromService(t, svc)

	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID := seedTestHistory(t, testDB, 1, dsID, "SELECT 1", "mysql")
	svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID)

	historyID2 := seedTestHistory(t, testDB, 1, dsID, "SELECT 2", "mysql")
	svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID2)

	// User 2's snapshot
	historyID3 := seedTestHistory(t, testDB, 2, dsID, "SELECT 3", "mysql")
	svc.CreateSnapshotFromHistory(ctx, 2, "u2", "admin", historyID3)

	snapshots, total, err := svc.ListSnapshots(ctx, 1, 1, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(snapshots) != 2 {
		t.Errorf("len = %d, want 2", len(snapshots))
	}
}

func TestSnapshotService_DeleteSnapshot(t *testing.T) {
	mockExec := &mockQueryExecutor{
		result: &QueryResult{
			Columns: []string{"id"},
			Rows:    []map[string]interface{}{{"id": float64(1)}},
			Total:   1,
		},
	}

	svc := setupSnapshotTestDBWithExecutor(t, mockExec)
	ctx := context.Background()
	testDB := getDBFromService(t, svc)

	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID := seedTestHistory(t, testDB, 1, dsID, "SELECT 1", "mysql")
	snap, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID)

	if err := svc.DeleteSnapshot(ctx, snap.ID, 1); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	_, err := svc.GetSnapshot(ctx, snap.ID, 1)
	if err != ErrSnapshotNotFound {
		t.Errorf("expected ErrSnapshotNotFound after delete, got %v", err)
	}
}

func TestSnapshotService_CompareSnapshots(t *testing.T) {
	mockExec := &mockQueryExecutor{
		result: &QueryResult{
			Columns: []string{"id", "name"},
			Rows: []map[string]interface{}{
				{"id": float64(1), "name": "Alice"},
				{"id": float64(2), "name": "Bob"},
			},
			Total: 2,
		},
	}

	svc := setupSnapshotTestDBWithExecutor(t, mockExec)
	ctx := context.Background()
	testDB := getDBFromService(t, svc)

	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID1 := seedTestHistory(t, testDB, 1, dsID, "SELECT 1", "mysql")
	historyID2 := seedTestHistory(t, testDB, 1, dsID, "SELECT 2", "mysql")

	snapA, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID1)

	// Change mock result for second snapshot
	mockExec.result = &QueryResult{
		Columns: []string{"id", "name"},
		Rows: []map[string]interface{}{
			{"id": float64(1), "name": "Alice"},
			{"id": float64(3), "name": "Charlie"},
		},
		Total: 2,
	}
	snapB, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID2)

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

func TestSnapshotService_CompareSnapshots_SchemaMismatch(t *testing.T) {
	mockExec := &mockQueryExecutor{
		result: &QueryResult{
			Columns: []string{"id", "name"},
			Rows:    []map[string]interface{}{{"id": float64(1), "name": "Alice"}},
			Total:   1,
		},
	}

	svc := setupSnapshotTestDBWithExecutor(t, mockExec)
	ctx := context.Background()
	testDB := getDBFromService(t, svc)

	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID1 := seedTestHistory(t, testDB, 1, dsID, "SELECT 1", "mysql")
	snapA, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID1)

	mockExec.result = &QueryResult{
		Columns: []string{"id", "email"},
		Rows:    []map[string]interface{}{{"id": float64(1), "email": "a@b.com"}},
		Total:   1,
	}
	historyID2 := seedTestHistory(t, testDB, 1, dsID, "SELECT 2", "mysql")
	snapB, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID2)

	_, err := svc.CompareSnapshots(ctx, snapA.ID, snapB.ID, 1)
	if err != ErrSchemaMismatch {
		t.Errorf("expected ErrSchemaMismatch, got %v", err)
	}
}

func TestSnapshotService_CompareSnapshots_SameSnapshot(t *testing.T) {
	mockExec := &mockQueryExecutor{
		result: &QueryResult{
			Columns: []string{"id"},
			Rows:    []map[string]interface{}{{"id": float64(1)}},
			Total:   1,
		},
	}

	svc := setupSnapshotTestDBWithExecutor(t, mockExec)
	ctx := context.Background()
	testDB := getDBFromService(t, svc)

	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID := seedTestHistory(t, testDB, 1, dsID, "SELECT 1", "mysql")
	snap, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID)

	_, err := svc.CompareSnapshots(ctx, snap.ID, snap.ID, 1)
	if err != ErrSameSnapshot {
		t.Errorf("expected ErrSameSnapshot, got %v", err)
	}
}

func TestSnapshotService_GetSnapshot_Forbidden(t *testing.T) {
	mockExec := &mockQueryExecutor{
		result: &QueryResult{
			Columns: []string{"id"},
			Rows:    []map[string]interface{}{{"id": float64(1)}},
			Total:   1,
		},
	}

	svc := setupSnapshotTestDBWithExecutor(t, mockExec)
	ctx := context.Background()
	testDB := getDBFromService(t, svc)

	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID := seedTestHistory(t, testDB, 1, dsID, "SELECT 1", "mysql")
	snap, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID)

	_, err := svc.GetSnapshot(ctx, snap.ID, 2)
	if err != ErrSnapshotForbidden {
		t.Errorf("expected ErrSnapshotForbidden, got %v", err)
	}
}

func TestSnapshotService_DeleteSnapshot_Forbidden(t *testing.T) {
	mockExec := &mockQueryExecutor{
		result: &QueryResult{
			Columns: []string{"id"},
			Rows:    []map[string]interface{}{{"id": float64(1)}},
			Total:   1,
		},
	}

	svc := setupSnapshotTestDBWithExecutor(t, mockExec)
	ctx := context.Background()
	testDB := getDBFromService(t, svc)

	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID := seedTestHistory(t, testDB, 1, dsID, "SELECT 1", "mysql")
	snap, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID)

	err := svc.DeleteSnapshot(ctx, snap.ID, 2)
	if err != ErrSnapshotForbidden {
		t.Errorf("expected ErrSnapshotForbidden, got %v", err)
	}
}

func TestSnapshotService_CompareSnapshots_Modified(t *testing.T) {
	mockExec := &mockQueryExecutor{
		result: &QueryResult{
			Columns: []string{"id", "name", "score"},
			Rows: []map[string]interface{}{
				{"id": float64(1), "name": "Alice", "score": float64(90)},
				{"id": float64(2), "name": "Bob", "score": float64(85)},
			},
			Total: 2,
		},
	}

	svc := setupSnapshotTestDBWithExecutor(t, mockExec)
	ctx := context.Background()
	testDB := getDBFromService(t, svc)

	dsID := seedTestDatasourceForSnapshot(t, testDB, "test-mysql")
	historyID1 := seedTestHistory(t, testDB, 1, dsID, "SELECT 1", "mysql")
	snapA, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID1)

	mockExec.result = &QueryResult{
		Columns: []string{"id", "name", "score"},
		Rows: []map[string]interface{}{
			{"id": float64(1), "name": "Alice", "score": float64(90)},
			{"id": float64(2), "name": "Bob", "score": float64(88)},
		},
		Total: 2,
	}
	historyID2 := seedTestHistory(t, testDB, 1, dsID, "SELECT 2", "mysql")
	snapB, _ := svc.CreateSnapshotFromHistory(ctx, 1, "u1", "admin", historyID2)

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

	for _, dr := range result.DiffRows {
		if dr.Type == "modified" {
			if len(dr.ChangedFields) != 1 || dr.ChangedFields[0] != "score" {
				t.Errorf("changedFields = %v, want [score]", dr.ChangedFields)
			}
		}
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

func TestCompareRows_AllUnchanged(t *testing.T) {
	columns := []string{"id", "name"}
	rows := [][]interface{}{
		{1, "Alice"},
		{2, "Bob"},
	}

	result := compareRows(columns, rows, rows)

	if result.Summary.Unchanged != 2 {
		t.Errorf("unchanged = %d, want 2", result.Summary.Unchanged)
	}
	if result.Summary.Added != 0 || result.Summary.Removed != 0 || result.Summary.Modified != 0 {
		t.Errorf("expected all zero, got added=%d removed=%d modified=%d",
			result.Summary.Added, result.Summary.Removed, result.Summary.Modified)
	}
}

func TestValueEqual_FloatVsInt(t *testing.T) {
	// json.Marshal distinguishes float64(1) from int(1)
	if !valueEqual(float64(1), float64(1)) {
		t.Error("float64(1) should equal float64(1)")
	}
	if valueEqual(float64(1), "1") {
		t.Error("float64(1) should not equal string '1'")
	}
}

// DB returns the underlying database connection (for testing only).
func (s *SnapshotService) DB() *sql.DB { return s.db }

// getDBFromService extracts the underlying db for seeding test data.
func getDBFromService(t *testing.T, svc *SnapshotService) *sql.DB {
	t.Helper()
	return svc.DB()
}
