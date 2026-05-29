package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

// SnapshotService handles query snapshot CRUD and comparison.
type SnapshotService struct {
	db *sql.DB
}

// NewSnapshotService creates a new SnapshotService.
func NewSnapshotService(db *sql.DB) *SnapshotService {
	return &SnapshotService{db: db}
}

var (
	ErrSnapshotNotFound   = errors.New("快照不存在")
	ErrSnapshotForbidden  = errors.New("无权访问此快照")
	ErrSchemaMismatch     = errors.New("两个快照的列结构不一致")
	ErrSameSnapshot       = errors.New("不能对比同一个快照")
)

// CreateSnapshot saves a query result as a snapshot.
func (s *SnapshotService) CreateSnapshot(ctx context.Context, userID int64, label string, columns, rows json.RawMessage, rowCount int) (*model.QuerySnapshot, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO query_snapshots (user_id, label, columns, rows, row_count, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, label, string(columns), string(rows), rowCount, now,
	)
	if err != nil {
		return nil, fmt.Errorf("创建快照失败: %w", err)
	}

	id, _ := result.LastInsertId()
	return &model.QuerySnapshot{
		ID:        id,
		UserID:    userID,
		Label:     label,
		Columns:   columns,
		Rows:      rows,
		RowCount:  rowCount,
		CreatedAt: now,
	}, nil
}

// GetSnapshot returns a single snapshot by ID (must belong to userID).
func (s *SnapshotService) GetSnapshot(ctx context.Context, id, userID int64) (*model.QuerySnapshot, error) {
	snap := &model.QuerySnapshot{}
	var columns, rows string

	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, label, columns, rows, row_count, created_at FROM query_snapshots WHERE id = ?`,
		id,
	).Scan(&snap.ID, &snap.UserID, &snap.Label, &columns, &rows, &snap.RowCount, &snap.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询快照失败: %w", err)
	}

	if snap.UserID != userID {
		return nil, ErrSnapshotForbidden
	}

	snap.Columns = json.RawMessage(columns)
	snap.Rows = json.RawMessage(rows)
	return snap, nil
}

// ListSnapshots returns snapshots belonging to userID, ordered by newest first.
func (s *SnapshotService) ListSnapshots(ctx context.Context, userID int64, page, pageSize int) ([]*model.QuerySnapshot, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	var total int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM query_snapshots WHERE user_id = ?`, userID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("统计快照失败: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, label, columns, rows, row_count, created_at FROM query_snapshots WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		userID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("查询快照列表失败: %w", err)
	}
	defer rows.Close()

	var snapshots []*model.QuerySnapshot
	for rows.Next() {
		snap := &model.QuerySnapshot{}
		var columns, rowsData string
		if err := rows.Scan(&snap.ID, &snap.UserID, &snap.Label, &columns, &rowsData, &snap.RowCount, &snap.CreatedAt); err != nil {
			return nil, 0, err
		}
		snap.Columns = json.RawMessage(columns)
		snap.Rows = json.RawMessage(rowsData)
		snapshots = append(snapshots, snap)
	}

	return snapshots, total, nil
}

// DeleteSnapshot deletes a snapshot (must belong to userID).
func (s *SnapshotService) DeleteSnapshot(ctx context.Context, id, userID int64) error {
	// Verify ownership
	snap, err := s.GetSnapshot(ctx, id, userID)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM query_snapshots WHERE id = ? AND user_id = ?`, snap.ID, userID)
	if err != nil {
		return fmt.Errorf("删除快照失败: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------

// DiffRow represents a single row diff result.
type DiffRow struct {
	Type         string                 `json:"type"`          // "added" | "removed" | "modified" | "unchanged"
	RowIndex     int                    `json:"rowIndex"`
	Left         map[string]interface{} `json:"left,omitempty"`
	Right        map[string]interface{} `json:"right,omitempty"`
	ChangedFields []string              `json:"changedFields,omitempty"`
}

// CompareResult represents the full diff result.
type CompareResult struct {
	Columns   []string  `json:"columns"`
	TotalLeft int       `json:"totalLeft"`
	TotalRight int      `json:"totalRight"`
	DiffRows  []DiffRow `json:"diffRows"`
	Summary   struct {
		Added     int `json:"added"`
		Removed   int `json:"removed"`
		Modified  int `json:"modified"`
		Unchanged int `json:"unchanged"`
	} `json:"summary"`
}

// CompareSnapshots compares two snapshots and returns the diff.
func (s *SnapshotService) CompareSnapshots(ctx context.Context, snapshotAID, snapshotBID, userID int64) (*CompareResult, error) {
	if snapshotAID == snapshotBID {
		return nil, ErrSameSnapshot
	}

	snapA, err := s.GetSnapshot(ctx, snapshotAID, userID)
	if err != nil {
		return nil, err
	}

	snapB, err := s.GetSnapshot(ctx, snapshotBID, userID)
	if err != nil {
		return nil, err
	}

	// Parse columns
	var colsA, colsB []string
	if err := json.Unmarshal(snapA.Columns, &colsA); err != nil {
		return nil, fmt.Errorf("解析快照A列信息失败: %w", err)
	}
	if err := json.Unmarshal(snapB.Columns, &colsB); err != nil {
		return nil, fmt.Errorf("解析快照B列信息失败: %w", err)
	}

	// Schema check
	if !columnsEqual(colsA, colsB) {
		return nil, ErrSchemaMismatch
	}

	// Parse rows
	var rowsA, rowsB [][]interface{}
	if err := json.Unmarshal(snapA.Rows, &rowsA); err != nil {
		return nil, fmt.Errorf("解析快照A行数据失败: %w", err)
	}
	if err := json.Unmarshal(snapB.Rows, &rowsB); err != nil {
		return nil, fmt.Errorf("解析快照B行数据失败: %w", err)
	}

	return compareRows(colsA, rowsA, rowsB), nil
}

// compareRows performs row-level comparison between two row sets.
func compareRows(columns []string, rowsA, rowsB [][]interface{}) *CompareResult {
	result := &CompareResult{
		Columns:    columns,
		TotalLeft:  len(rowsA),
		TotalRight: len(rowsB),
		DiffRows:   make([]DiffRow, 0),
	}

	// Build hash maps for fast lookup
	mapA := make(map[string]int) // hash -> index in rowsA
	for i, row := range rowsA {
		h := rowHash(row)
		mapA[h] = i
	}

	mapB := make(map[string]int) // hash -> index in rowsB
	for i, row := range rowsB {
		h := rowHash(row)
		mapB[h] = i
	}

	// Track processed rows to avoid duplicates
	processedA := make(map[int]bool)
	processedB := make(map[int]bool)

	// First pass: find rows in A and check if they exist in B
	for i, row := range rowsA {
		h := rowHash(row)
		rowMap := rowToMap(columns, row)

		if j, exists := mapB[h]; exists {
			// Exact match found in B — unchanged
			if !processedB[j] {
				result.DiffRows = append(result.DiffRows, DiffRow{
					Type:     "unchanged",
					RowIndex: i,
					Left:     rowMap,
					Right:    rowMap,
				})
				result.Summary.Unchanged++
				processedB[j] = true
				processedA[i] = true
			}
		}
	}

	// Second pass: find modified and remaining removed/added
	for i, row := range rowsA {
		if processedA[i] {
			continue
		}

		rowMapA := rowToMap(columns, row)
		// Try to find a "close" match in B by first column (primary key heuristic)
		matched := false
		for j, rowB := range rowsB {
			if processedB[j] {
				continue
			}
			// If first column matches, consider it a modification
			if len(row) > 0 && len(rowB) > 0 && fmt.Sprintf("%v", row[0]) == fmt.Sprintf("%v", rowB[0]) {
				rowMapB := rowToMap(columns, rowB)
				changedFields := findChangedFields(rowMapA, rowMapB)
				result.DiffRows = append(result.DiffRows, DiffRow{
					Type:          "modified",
					RowIndex:      i,
					Left:          rowMapA,
					Right:         rowMapB,
					ChangedFields: changedFields,
				})
				result.Summary.Modified++
				processedB[j] = true
				matched = true
				break
			}
		}

		if !matched {
			// Removed — exists in A but not in B
			result.DiffRows = append(result.DiffRows, DiffRow{
				Type:     "removed",
				RowIndex: i,
				Left:     rowMapA,
			})
			result.Summary.Removed++
		}
	}

	// Third pass: find added rows (in B but not matched)
	for j, row := range rowsB {
		if processedB[j] {
			continue
		}
		result.DiffRows = append(result.DiffRows, DiffRow{
			Type:     "added",
			RowIndex: j,
			Right:    rowToMap(columns, row),
		})
		result.Summary.Added++
	}

	return result
}

// rowHash returns a deterministic hash for a row.
func rowHash(row []interface{}) string {
	b, _ := json.Marshal(row)
	return string(b)
}

// rowToMap converts a row array + column names to a map.
func rowToMap(columns []string, row []interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(columns))
	for i, col := range columns {
		if i < len(row) {
			m[col] = row[i]
		}
	}
	return m
}

// findChangedFields returns the list of field names that differ between two maps.
func findChangedFields(a, b map[string]interface{}) []string {
	var changed []string
	for k := range a {
		if bv, ok := b[k]; ok {
			if fmt.Sprintf("%v", a[k]) != fmt.Sprintf("%v", bv) {
				changed = append(changed, k)
			}
		}
	}
	return changed
}

// columnsEqual checks if two column slices are identical.
func columnsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
