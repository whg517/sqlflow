import { api } from "./client";

// --- Types ---

export interface ExplainRow {
  id: number;
  select_type: string;
  table: string;
  partitions: string | null;
  type: string;
  possible_keys: string | null;
  key: string | null;
  key_len: string | null;
  ref: string | null;
  rows: number;
  filtered: number;
  extra: string | null;
}

export interface ExplainResult {
  query: string;
  datasource_id: number;
  plan: ExplainRow[];
  formatted: string;
}

interface ExplainResponse {
  code: number;
  message: string;
  data: ExplainResult;
}

// --- API ---

export async function explainQuery(
  sql: string,
  datasourceId: number,
  database?: string,
): Promise<ExplainResult> {
  const res = await api.post<ExplainResponse>("/query/explain", {
    sql,
    datasource_id: datasourceId,
    database: database ?? "",
  });
  return res.data;
}
