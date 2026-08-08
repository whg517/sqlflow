package query

import (
	"context"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/whg517/sqlflow/internal/db/ent"
	entdatasource "github.com/whg517/sqlflow/internal/db/ent/datasource"
	entqueryhistory "github.com/whg517/sqlflow/internal/db/ent/queryhistory"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"
)

// SlowQueryParams holds parameters for listing slow queries.
type SlowQueryParams struct {
	Threshold    int64 // ms, default 1000
	Page         int
	PageSize     int
	DatasourceID int64
	StartDate    string // optional, format: 2006-01-02
	EndDate      string // optional, format: 2006-01-02
}

// DailyTrend represents per-day aggregated stats.
type DailyTrend struct {
	Date      string `json:"date"`
	Count     int    `json:"count"`
	AvgTime   int64  `json:"avg_time"`
	SlowCount int    `json:"slow_count"`
}

// DatasourceStats represents per-datasource aggregated stats.
type DatasourceStats struct {
	DatasourceID   int64  `json:"datasource_id"`
	DatasourceName string `json:"datasource_name"`
	Count          int    `json:"count"`
	AvgTime        int64  `json:"avg_time"`
}

// TopSlowQuery represents a top slow query entry.
type TopSlowQuery struct {
	ID             int64     `json:"id"`
	SQLSummary     string    `json:"sql_summary"`
	ExecutionTime  int64     `json:"execution_time"`
	DatasourceName string    `json:"datasource_name"`
	CreatedAt      time.Time `json:"created_at"`
}

// PerformanceStats holds aggregated performance statistics.
type PerformanceStats struct {
	TotalQueries    int               `json:"total_queries"`
	SlowQueries     int               `json:"slow_queries"`
	AvgTime         int64             `json:"avg_time"`
	SlowQueryRate   float64           `json:"slow_query_rate"`
	DailyTrend      []DailyTrend      `json:"daily_trend"`
	DatasourceStats []DatasourceStats `json:"datasource_stats"`
	TopSlowQueries  []TopSlowQuery    `json:"top_slow_queries"`
}

// ListSlowQueries returns paginated slow queries with optional filters.
func (s *HistoryService) ListSlowQueries(ctx context.Context, actor ExportActor, params SlowQueryParams) ([]model.QueryHistory, int, error) {
	p := sqlutil.ParsePagination(params.Page, params.PageSize)

	threshold := params.Threshold
	if threshold <= 0 {
		threshold = defaultSlowQueryThresholdMs
	}

	q := s.client.QueryHistory.Query().
		Where(entqueryhistory.ExecutionTimeGTE(threshold))

	// The owner predicate the sibling entrance already applies.
	//
	// This read used to have none, while ListHistory scoped the same table to
	// the caller — and query_history carries SQLContent verbatim, so every
	// other user's statement text was readable by anyone authenticated.
	wide, err := s.mayReadEveryHistory(ctx, actor)
	if err != nil {
		return nil, 0, err
	}
	if !wide {
		q = q.Where(entqueryhistory.UserIDEQ(actor.UserID))
	}
	if params.DatasourceID > 0 {
		q = q.Where(entqueryhistory.DatasourceIDEQ(params.DatasourceID))
	}
	if from, ok := parseDayStart(params.StartDate); ok {
		q = q.Where(entqueryhistory.CreatedAtGTE(from))
	}
	if to, ok := parseDayStart(params.EndDate); ok {
		// The caller passes a date; comparing against it directly would drop
		// everything recorded on the end date itself.
		q = q.Where(entqueryhistory.CreatedAtLT(to.AddDate(0, 0, 1)))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count slow queries: %w", err)
	}

	rows, err := q.
		Order(ent.Desc(entqueryhistory.FieldExecutionTime)).
		Limit(p.PageSize).
		Offset(p.Offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query slow queries: %w", err)
	}

	list := make([]model.QueryHistory, 0, len(rows))
	for _, h := range rows {
		list = append(list, model.QueryHistory{
			ID:            int64(h.ID),
			UserID:        h.UserID,
			DatasourceID:  h.DatasourceID,
			Database:      h.Database,
			SQLContent:    h.SQLContent,
			SQLSummary:    h.SQLSummary,
			DBType:        h.DbType,
			ExecutionTime: h.ExecutionTime,
			ResultRows:    h.ResultRows,
			AffectedRows:  h.AffectedRows,
			CreatedAt:     h.CreatedAt,
		})
	}
	return list, total, nil
}

// defaultSlowQueryThresholdMs is what counts as slow when the caller says
// nothing.
//
// Shared with the statistics query below, which classified slow queries with
// its own literal — the two could disagree, and the dashboard would then
// contradict the list it links to.
const defaultSlowQueryThresholdMs = 1000

// parseDayStart reads a YYYY-MM-DD filter value.
//
// An empty or malformed value means "no bound" rather than an error: these
// arrive as optional query parameters, and the previous version appended the
// string straight into the comparison, which PostgreSQL rejected outright for
// an empty value.
func parseDayStart(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// GetPerformanceStats returns aggregated performance statistics for the given number of days.
func (s *HistoryService) GetPerformanceStats(ctx context.Context, actor ExportActor, days int) (*PerformanceStats, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	inWindow := s.client.QueryHistory.Query().Where(entqueryhistory.CreatedAtGTE(since))

	// Same boundary as the list. The counts are milder than statement text, but
	// topSlowQueries below returns SQLSummary, and one entrance answering a
	// narrower question than its sibling is how the two drifted apart.
	wide, err := s.mayReadEveryHistory(ctx, actor)
	if err != nil {
		return nil, err
	}
	if !wide {
		inWindow = inWindow.Where(entqueryhistory.UserIDEQ(actor.UserID))
	}

	totalQueries, err := inWindow.Clone().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count queries: %w", err)
	}
	slowQueries, err := inWindow.Clone().
		Where(entqueryhistory.ExecutionTimeGTE(defaultSlowQueryThresholdMs)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count slow queries: %w", err)
	}

	var avgTime int64
	slowRate := float64(0)
	if totalQueries > 0 {
		slowRate = float64(slowQueries) / float64(totalQueries) * 100

		// AVG over an empty set is NULL and will not scan into a number, so the
		// aggregate only runs when there is something to average.
		var avg []struct {
			Avg float64 `json:"avg"`
		}
		if err := inWindow.Clone().
			Aggregate(ent.Mean(entqueryhistory.FieldExecutionTime)).
			Scan(ctx, &avg); err != nil {
			return nil, fmt.Errorf("average execution time: %w", err)
		}
		if len(avg) > 0 {
			avgTime = int64(avg[0].Avg)
		}
	}

	dailyTrend, err := s.dailyPerformanceTrend(ctx, actor, wide, since)
	if err != nil {
		return nil, err
	}
	dsStats, err := s.datasourcePerformance(ctx, since)
	if err != nil {
		return nil, err
	}
	topSlow, err := s.topSlowQueries(ctx, actor, wide, since)
	if err != nil {
		return nil, err
	}

	return &PerformanceStats{
		TotalQueries:    totalQueries,
		SlowQueries:     slowQueries,
		AvgTime:         avgTime,
		SlowQueryRate:   slowRate,
		DailyTrend:      dailyTrend,
		DatasourceStats: dsStats,
		TopSlowQueries:  topSlow,
	}, nil
}

// dailyPerformanceTrend buckets the window by day.
func (s *HistoryService) dailyPerformanceTrend(ctx context.Context, actor ExportActor, wide bool, since time.Time) ([]DailyTrend, error) {
	var rows []DailyTrend
	q := s.client.QueryHistory.Query().Where(entqueryhistory.CreatedAtGTE(since))
	if !wide {
		q = q.Where(entqueryhistory.UserIDEQ(actor.UserID))
	}
	err := q.
		Modify(func(sel *entsql.Selector) {
			day := "to_char(" + sel.C(entqueryhistory.FieldCreatedAt) + ", 'YYYY-MM-DD')"
			exec := sel.C(entqueryhistory.FieldExecutionTime)
			// The shared threshold, not a literal. The constant's own comment
			// said it was shared with the statistics query "so the two could not
			// disagree"; two of the three call sites converged and this one kept
			// its 1000, which is the drift the comment describes.
			slow := fmt.Sprintf("COUNT(*) FILTER (WHERE %s >= %d)", exec, defaultSlowQueryThresholdMs)
			sel.Select().AppendSelect(
				entsql.As(day, "date"),
				entsql.As("COUNT(*)", "count"),
				entsql.As("CAST(COALESCE(AVG("+exec+"), 0) AS BIGINT)", "avg_time"),
				entsql.As(slow, "slow_count"),
			).GroupBy(day).OrderBy("date")
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("get daily trend: %w", err)
	}
	return rows, nil
}

// datasourcePerformance breaks the window down per datasource.
func (s *HistoryService) datasourcePerformance(ctx context.Context, since time.Time) ([]DatasourceStats, error) {
	var rows []DatasourceStats
	err := s.client.QueryHistory.Query().
		Where(entqueryhistory.CreatedAtGTE(since)).
		Modify(func(sel *entsql.Selector) {
			ds := entsql.Table(entdatasource.Table).As("ds")
			sel.LeftJoin(ds).On(sel.C(entqueryhistory.FieldDatasourceID), ds.C(entdatasource.FieldID))
			exec := sel.C(entqueryhistory.FieldExecutionTime)
			sel.Select().AppendSelect(
				entsql.As(sel.C(entqueryhistory.FieldDatasourceID), "datasource_id"),
				entsql.As("COALESCE("+ds.C(entdatasource.FieldName)+", '未知')", "datasource_name"),
				entsql.As("COUNT(*)", "count"),
				entsql.As("CAST(COALESCE(AVG("+exec+"), 0) AS BIGINT)", "avg_time"),
			).GroupBy(sel.C(entqueryhistory.FieldDatasourceID), ds.C(entdatasource.FieldName)).
				OrderExpr(entsql.Expr("count DESC"))
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("get datasource stats: %w", err)
	}
	return rows, nil
}

// topSlowQueries returns the slowest statements in the window.
func (s *HistoryService) topSlowQueries(ctx context.Context, actor ExportActor, wide bool, since time.Time) ([]TopSlowQuery, error) {
	var rows []TopSlowQuery
	q := s.client.QueryHistory.Query().Where(entqueryhistory.CreatedAtGTE(since))
	if !wide {
		q = q.Where(entqueryhistory.UserIDEQ(actor.UserID))
	}
	err := q.
		Modify(func(sel *entsql.Selector) {
			ds := entsql.Table(entdatasource.Table).As("ds")
			sel.LeftJoin(ds).On(sel.C(entqueryhistory.FieldDatasourceID), ds.C(entdatasource.FieldID))
			sel.Select(
				sel.C(entqueryhistory.FieldID),
				sel.C(entqueryhistory.FieldSQLSummary),
				sel.C(entqueryhistory.FieldExecutionTime),
				sel.C(entqueryhistory.FieldCreatedAt),
			).AppendSelect(
				entsql.As("COALESCE("+ds.C(entdatasource.FieldName)+", '未知')", "datasource_name"),
			).OrderBy(entsql.Desc(sel.C(entqueryhistory.FieldExecutionTime))).
				Limit(topSlowQueryLimit)
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("get top slow queries: %w", err)
	}
	return rows, nil
}

// topSlowQueryLimit bounds the slowest-query list on the dashboard.
const topSlowQueryLimit = 10
