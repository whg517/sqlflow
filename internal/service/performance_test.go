package service

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

func TestPerformanceService_ListSlowQueries(t *testing.T) {
	testDB := setupIntegrationDB(t)
	svc := NewQueryHistoryService(testDB)

	userID := seedIntegrationUser(t, testDB, "perf_user", "developer")
	dsID := seedIntegrationDatasource(t, testDB, "perf-ds")

	// Insert queries with various execution times
	records := []struct {
		sql           string
		executionTime int64
	}{
		{"SELECT * FROM users", 50},
		{"SELECT * FROM orders", 1500},
		{"SELECT * FROM products", 3000},
		{"SELECT * FROM logs", 500},
		{"SELECT * FROM analytics", 5000},
	}
	for _, r := range records {
		h := &model.QueryHistory{
			UserID: userID, DatasourceID: dsID, Database: "testdb",
			SQLContent: r.sql, SQLSummary: r.sql, DBType: "mysql",
			ExecutionTime: r.executionTime,
		}
		if err := svc.CreateHistory(context.Background(), h); err != nil {
			t.Fatalf("CreateHistory: %v", err)
		}
	}

	t.Run("default_threshold_1000ms", func(t *testing.T) {
		list, total, err := svc.ListSlowQueries(context.Background(), SlowQueryParams{
			Threshold: 1000, Page: 1, PageSize: 10,
		})
		if err != nil {
			t.Fatalf("ListSlowQueries: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3 (queries >= 1000ms)", total)
		}
		if len(list) != 3 {
			t.Fatalf("len(list) = %d, want 3", len(list))
		}
		// Should be ordered by execution_time DESC
		if list[0].ExecutionTime < list[1].ExecutionTime {
			t.Errorf("expected DESC order: %d >= %d", list[0].ExecutionTime, list[1].ExecutionTime)
		}
	})

	t.Run("custom_threshold_500ms", func(t *testing.T) {
		_, total, err := svc.ListSlowQueries(context.Background(), SlowQueryParams{
			Threshold: 500, Page: 1, PageSize: 10,
		})
		if err != nil {
			t.Fatalf("ListSlowQueries: %v", err)
		}
		if total != 4 {
			t.Errorf("total = %d, want 4 (queries >= 500ms)", total)
		}
	})

	t.Run("filter_by_datasource", func(t *testing.T) {
		_, total, err := svc.ListSlowQueries(context.Background(), SlowQueryParams{
			Threshold: 1000, Page: 1, PageSize: 10, DatasourceID: dsID,
		})
		if err != nil {
			t.Fatalf("ListSlowQueries: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
	})

	t.Run("filter_by_nonexistent_datasource", func(t *testing.T) {
		_, total, err := svc.ListSlowQueries(context.Background(), SlowQueryParams{
			Threshold: 1000, Page: 1, PageSize: 10, DatasourceID: 99999,
		})
		if err != nil {
			t.Fatalf("ListSlowQueries: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0", total)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		list, total, err := svc.ListSlowQueries(context.Background(), SlowQueryParams{
			Threshold: 1000, Page: 1, PageSize: 2,
		})
		if err != nil {
			t.Fatalf("ListSlowQueries: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(list) != 2 {
			t.Errorf("len(list) = %d, want 2", len(list))
		}
	})

	t.Run("empty_result", func(t *testing.T) {
		list, total, err := svc.ListSlowQueries(context.Background(), SlowQueryParams{
			Threshold: 10000, Page: 1, PageSize: 10,
		})
		if err != nil {
			t.Fatalf("ListSlowQueries: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0", total)
		}
		if list != nil {
			t.Errorf("list should be nil for empty result, got %v", list)
		}
	})
}

func TestPerformanceService_GetPerformanceStats(t *testing.T) {
	testDB := setupIntegrationDB(t)
	svc := NewQueryHistoryService(testDB)

	userID := seedIntegrationUser(t, testDB, "perf_stats_user", "developer")
	dsID := seedIntegrationDatasource(t, testDB, "perf-stats-ds")

	// Insert queries
	records := []struct {
		sql           string
		executionTime int64
	}{
		{"SELECT * FROM users", 100},
		{"SELECT * FROM orders", 2000},
		{"SELECT * FROM products", 3000},
		{"SELECT * FROM logs", 50},
	}
	for _, r := range records {
		h := &model.QueryHistory{
			UserID: userID, DatasourceID: dsID, Database: "testdb",
			SQLContent: r.sql, SQLSummary: r.sql, DBType: "mysql",
			ExecutionTime: r.executionTime,
		}
		if err := svc.CreateHistory(context.Background(), h); err != nil {
			t.Fatalf("CreateHistory: %v", err)
		}
	}

	stats, err := svc.GetPerformanceStats(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetPerformanceStats: %v", err)
	}

	if stats.TotalQueries != 4 {
		t.Errorf("TotalQueries = %d, want 4", stats.TotalQueries)
	}
	if stats.SlowQueries != 2 {
		t.Errorf("SlowQueries = %d, want 2", stats.SlowQueries)
	}
	if stats.AvgTime <= 0 {
		t.Errorf("AvgTime = %d, want > 0", stats.AvgTime)
	}

	// Slow query rate = 2/4 * 100 = 50.0
	expectedRate := float64(50)
	if stats.SlowQueryRate < expectedRate-0.1 || stats.SlowQueryRate > expectedRate+0.1 {
		t.Errorf("SlowQueryRate = %.2f, want ~%.2f", stats.SlowQueryRate, expectedRate)
	}

	// Should have daily trend
	if len(stats.DailyTrend) == 0 {
		t.Error("DailyTrend should not be empty")
	}

	// Should have datasource stats
	if len(stats.DatasourceStats) == 0 {
		t.Error("DatasourceStats should not be empty")
	} else {
		ds := stats.DatasourceStats[0]
		if ds.DatasourceName == "" {
			t.Error("DatasourceName should not be empty")
		}
	}

	// Should have top slow queries
	if len(stats.TopSlowQueries) == 0 {
		t.Error("TopSlowQueries should not be empty")
	}
	if len(stats.TopSlowQueries) > 10 {
		t.Errorf("TopSlowQueries len = %d, want <= 10", len(stats.TopSlowQueries))
	}
	// Top slow query should be the 3000ms one
	if stats.TopSlowQueries[0].ExecutionTime != 3000 {
		t.Errorf("Top slow query ExecutionTime = %d, want 3000", stats.TopSlowQueries[0].ExecutionTime)
	}
}

func TestPerformanceService_EmptyStats(t *testing.T) {
	testDB := setupIntegrationDB(t)
	svc := NewQueryHistoryService(testDB)

	// No data seeded
	stats, err := svc.GetPerformanceStats(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetPerformanceStats: %v", err)
	}

	if stats.TotalQueries != 0 {
		t.Errorf("TotalQueries = %d, want 0", stats.TotalQueries)
	}
	if stats.SlowQueries != 0 {
		t.Errorf("SlowQueries = %d, want 0", stats.SlowQueries)
	}
	if stats.SlowQueryRate != 0 {
		t.Errorf("SlowQueryRate = %.2f, want 0", stats.SlowQueryRate)
	}
}
