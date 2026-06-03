package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	"entgo.io/ent/dialect/sql"
	"github.com/whg517/sqlflow/internal/db/ent/querysnapshot"
	"github.com/whg517/sqlflow/internal/model"
)

// QueryExecutor is an interface for executing SQL queries.
// Used to decouple SnapshotService from QueryService.
type QueryExecutor interface {
	ExecuteQuery(ctx context.Context, userID int64, username, role string, datasourceID int64, database, sqlContent, dbType string) (*QueryResult, error)
}

// SnapshotService handles query snapshot CRUD and comparison.
type SnapshotService struct {
	database *db.DB
	client   *ent.Client
	exec     QueryExecutor
}

// NewSnapshotService creates a new SnapshotService.
func NewSnapshotService(database *db.DB, exec QueryExecutor) *SnapshotService {
	return &SnapshotService{database: database, client: database.Client(), exec: exec}
}

var (
	ErrSnapshotNotFound  = errors.New("快照不存在")
	ErrSnapshotForbidden = errors.New("无权访问此快照")
	ErrSchemaMismatch    = errors.New("两个快照的列结构不一致")
	ErrSameSnapshot      = errors.New("不能对比同一个快照")
	ErrHistoryNotFound   = errors.New("查询历史记录不存在")
)

// queryHistoryRow represents a row from query_history table.
type queryHistoryRow struct {
	ID           int64
	UserID       int64
	DatasourceID int64
	Database     string
	SQLContent   string
	SQLSummary   string
	DBType       string
}

// CreateSnapshotFromHistory creates a snapshot by re-executing a query from history.
func (s *SnapshotService) CreateSnapshotFromHistory(ctx context.Context, userID int64, username, role string, queryHistoryID int64) (*model.QuerySnapshot, error) {
	// 1. Fetch query history record
	history, err := s.getQueryHistory(ctx, queryHistoryID, userID)
	if err != nil {
		return nil, err
	}

	// 2. Re-execute the SQL
	result, err := s.exec.ExecuteQuery(ctx, userID, username, role, history.DatasourceID, history.Database, history.SQLContent, history.DBType)
	if err != nil {
		return nil, fmt.Errorf("重新执行查询失败: %w", err)
	}

	// 3. Convert QueryResult columns/rows to JSON
	columnsJSON, err := json.Marshal(result.Columns)
	if err != nil {
		return nil, fmt.Errorf("序列化列信息失败: %w", err)
	}

	// Convert []map[string]interface{} rows to [][]interface{} for storage
	rowArrays := make([][]interface{}, len(result.Rows))
	for i, rowMap := range result.Rows {
		row := make([]interface{}, len(result.Columns))
		for j, col := range result.Columns {
			row[j] = rowMap[col]
		}
		rowArrays[i] = row
	}

	rowsJSON, err := json.Marshal(rowArrays)
	if err != nil {
		return nil, fmt.Errorf("序列化行数据失败: %w", err)
	}

	// 4. Save snapshot via ent
	saved, err := s.client.QuerySnapshot.Create().
		SetUserID(userID).
		SetQueryHistoryID(queryHistoryID).
		SetLabel("").
		SetColumnsJSON(string(columnsJSON)).
		SetRowsJSON(string(rowsJSON)).
		SetRowCount(int64(len(result.Rows))).
		SetCreatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建快照失败: %w", err)
	}

	return &model.QuerySnapshot{
		ID:             int64(saved.ID),
		UserID:         userID,
		QueryHistoryID: queryHistoryID,
		Columns:        json.RawMessage(columnsJSON),
		Rows:           json.RawMessage(rowsJSON),
		RowCount:       len(result.Rows),
		CreatedAt:      saved.CreatedAt.Format("2006-01-02 15:04:05"),
		SQLContent:     history.SQLContent,
		SQLSummary:     history.SQLSummary,
		Database:       history.Database,
	}, nil
}

// getQueryHistory fetches a query history record and verifies ownership.
func (s *SnapshotService) getQueryHistory(ctx context.Context, id, userID int64) (*queryHistoryRow, error) {
	h, err := s.client.QueryHistory.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrHistoryNotFound
		}
		return nil, fmt.Errorf("查询历史记录失败: %w", err)
	}

	if h.UserID != userID {
		return nil, ErrSnapshotForbidden
	}

	return &queryHistoryRow{
		ID:           int64(h.ID),
		UserID:       h.UserID,
		DatasourceID: h.DatasourceID,
		Database:     h.Database,
		SQLContent:   h.SQLContent,
		SQLSummary:   h.SQLSummary,
		DBType:       h.DbType,
	}, nil
}

// GetSnapshot returns a single snapshot by ID (must belong to userID).
func (s *SnapshotService) GetSnapshot(ctx context.Context, id, userID int64) (*model.QuerySnapshot, error) {
	snap, err := s.client.QuerySnapshot.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("查询快照失败: %w", err)
	}

	if snap.UserID != userID {
		return nil, ErrSnapshotForbidden
	}

	result := &model.QuerySnapshot{
		ID:             int64(snap.ID),
		UserID:         snap.UserID,
		QueryHistoryID: snap.QueryHistoryID,
		Columns:        json.RawMessage(snap.ColumnsJSON),
		Rows:           json.RawMessage(snap.RowsJSON),
		RowCount:       int(snap.RowCount),
		CreatedAt:      snap.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	// Join query_history info for display
	if snap.QueryHistoryID > 0 {
		qh, err := s.client.QueryHistory.Get(ctx, int(snap.QueryHistoryID))
		if err == nil {
			result.SQLContent = qh.SQLContent
			result.SQLSummary = qh.SQLSummary
			result.Database = qh.Database
		}
	}

	return result, nil
}

// ListSnapshots returns snapshots belonging to userID, ordered by newest first.
func (s *SnapshotService) ListSnapshots(ctx context.Context, userID int64, page, pageSize int) ([]*model.QuerySnapshot, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	total, err := s.client.QuerySnapshot.Query().
		Where(querysnapshot.UserID(userID)).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("统计快照失败: %w", err)
	}

	offset := (page - 1) * pageSize
	snapshots, err := s.client.QuerySnapshot.Query().
		Where(querysnapshot.UserID(userID)).
		Order(querysnapshot.ByCreatedAt(sql.OrderDesc())).
		Limit(pageSize).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("查询快照列表失败: %w", err)
	}

	var result []*model.QuerySnapshot
	for _, snap := range snapshots {
		result = append(result, &model.QuerySnapshot{
			ID:             int64(snap.ID),
			UserID:         snap.UserID,
			QueryHistoryID: snap.QueryHistoryID,
			Columns:        json.RawMessage(snap.ColumnsJSON),
			Rows:           json.RawMessage(snap.RowsJSON),
			RowCount:       int(snap.RowCount),
			CreatedAt:      snap.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return result, total, nil
}

// DeleteSnapshot deletes a snapshot (must belong to userID).
func (s *SnapshotService) DeleteSnapshot(ctx context.Context, id, userID int64) error {
	snap, err := s.GetSnapshot(ctx, id, userID)
	if err != nil {
		return err
	}

	_, err = s.client.QuerySnapshot.Delete().
		Where(querysnapshot.ID(int(snap.ID)), querysnapshot.UserID(userID)).
		Exec(ctx)
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
	Type          string                 `json:"type"`
	RowIndex      int                    `json:"rowIndex"`
	Left          map[string]interface{} `json:"left,omitempty"`
	Right         map[string]interface{} `json:"right,omitempty"`
	ChangedFields []string               `json:"changedFields,omitempty"`
}

// CompareResult represents the full diff result.
type CompareResult struct {
	Columns    []string  `json:"columns"`
	TotalLeft  int       `json:"totalLeft"`
	TotalRight int       `json:"totalRight"`
	DiffRows   []DiffRow `json:"diffRows"`
	Summary    struct {
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

	var colsA, colsB []string
	if err := json.Unmarshal(snapA.Columns, &colsA); err != nil {
		return nil, fmt.Errorf("解析快照A列信息失败: %w", err)
	}
	if err := json.Unmarshal(snapB.Columns, &colsB); err != nil {
		return nil, fmt.Errorf("解析快照B列信息失败: %w", err)
	}

	if !columnsEqual(colsA, colsB) {
		return nil, ErrSchemaMismatch
	}

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

	mapA := make(map[string]int)
	for i, row := range rowsA {
		h := rowHash(row)
		mapA[h] = i
	}

	mapB := make(map[string]int)
	for i, row := range rowsB {
		h := rowHash(row)
		mapB[h] = i
	}

	processedA := make(map[int]bool)
	processedB := make(map[int]bool)

	// First pass: find unchanged rows (exact hash match)
	for i, row := range rowsA {
		h := rowHash(row)
		if j, exists := mapB[h]; exists && !processedB[j] {
			rowMap := rowToMap(columns, row)
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

	// Second pass: find modified and removed rows
	for i, row := range rowsA {
		if processedA[i] {
			continue
		}

		rowMapA := rowToMap(columns, row)
		matched := false
		for j, rowB := range rowsB {
			if processedB[j] {
				continue
			}
			// Match by first column (primary key heuristic)
			if len(row) > 0 && len(rowB) > 0 && valueEqual(row[0], rowB[0]) {
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
			result.DiffRows = append(result.DiffRows, DiffRow{
				Type:     "removed",
				RowIndex: i,
				Left:     rowMapA,
			})
			result.Summary.Removed++
		}
	}

	// Third pass: find added rows
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

// rowHash returns a deterministic string for a row using JSON encoding.
func rowHash(row []interface{}) string {
	b, _ := json.Marshal(row)
	return string(b)
}

// valueEqual compares two values using JSON encoding for type-safe comparison.
func valueEqual(a, b interface{}) bool {
	aj, errA := json.Marshal(a)
	bj, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
	return string(aj) == string(bj)
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
			if !valueEqual(a[k], bv) {
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
