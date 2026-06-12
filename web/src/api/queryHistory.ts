import { api } from "./client";

// --- Types ---

export interface FrequentQuery {
  sql_content: string;
  sql_hash: string;
  snippet: string;
  execution_count: number;
  last_executed_at: string;
}

// --- API Functions ---

export async function getFrequentQueries(): Promise<{
  code: number;
  data: FrequentQuery[];
}> {
  return api.get("/query/history/frequent");
}
