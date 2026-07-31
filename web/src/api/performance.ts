import { api } from "./client";

// --- Types ---

export interface SlowQueryParams {
  threshold?: number;
  page?: number;
  page_size?: number;
  datasource_id?: number;
  start_date?: string;
  end_date?: string;
}

export interface SlowQueryItem {
  id: number;
  user_id: number;
  datasource_id: number;
  database: string;
  sql_content: string;
  sql_summary: string;
  db_type: string;
  execution_time: number;
  result_rows: number;
  affected_rows: number;
  created_at: string;
}

export interface SlowQueryResponse {
  code: number;
  message: string;
  data: SlowQueryItem[];
  page: number;
  page_size: number;
  total: number;
}

export interface DailyTrend {
  date: string;
  count: number;
  avg_time: number;
  slow_count: number;
}

export interface DatasourceStats {
  datasource_id: number;
  datasource_name: string;
  count: number;
  avg_time: number;
}

export interface TopSlowQuery {
  id: number;
  sql_summary: string;
  execution_time: number;
  datasource_name: string;
  created_at: string;
}

export interface PerformanceStats {
  total_queries: number;
  slow_queries: number;
  avg_time: number;
  slow_query_rate: number;
  daily_trend: DailyTrend[];
  datasource_stats: DatasourceStats[];
  top_slow_queries: TopSlowQuery[];
}

export interface PerformanceStatsResponse {
  code: number;
  message: string;
  data: Omit<
    PerformanceStats,
    "daily_trend" | "datasource_stats" | "top_slow_queries"
  > & {
    daily_trend: DailyTrend[] | null;
    datasource_stats: DatasourceStats[] | null;
    top_slow_queries: TopSlowQuery[] | null;
  };
}

// --- API Functions ---

export async function getSlowQueries(
  params: SlowQueryParams = {},
): Promise<SlowQueryResponse> {
  const qs = new URLSearchParams();
  if (params.threshold) qs.set("threshold", String(params.threshold));
  if (params.page) qs.set("page", String(params.page));
  if (params.page_size) qs.set("page_size", String(params.page_size));
  if (params.datasource_id)
    qs.set("datasource_id", String(params.datasource_id));
  if (params.start_date) qs.set("start_date", params.start_date);
  if (params.end_date) qs.set("end_date", params.end_date);
  const res = await api.get<
    Omit<SlowQueryResponse, "data"> & { data: SlowQueryItem[] | null }
  >(`/query/performance/slow?${qs.toString()}`);
  return {
    ...res,
    data: Array.isArray(res.data) ? res.data : [],
  };
}

export async function getPerformanceStats(
  days = 7,
): Promise<PerformanceStats> {
  const res = await api.get<PerformanceStatsResponse>(
    `/query/performance/stats?days=${days}`,
  );
  return {
    ...res.data,
    daily_trend: Array.isArray(res.data.daily_trend)
      ? res.data.daily_trend
      : [],
    datasource_stats: Array.isArray(res.data.datasource_stats)
      ? res.data.datasource_stats
      : [],
    top_slow_queries: Array.isArray(res.data.top_slow_queries)
      ? res.data.top_slow_queries
      : [],
  };
}
