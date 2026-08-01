import { api } from "@/shared/api/client";

// QueryForm decides which editor and request payload a data source needs.
// It is a separate axis from the capability flags: Elasticsearch and MySQL are
// both queryable, but nothing about their editors is the same.
export type QueryForm = "sql" | "document" | "dsl";

// DatasourceCapabilities mirrors driver.Descriptor on the server. It is the
// single source of truth for what a data source type can do — the UI reads it
// instead of branching on type names, so adding a driver needs no UI change.
export interface DatasourceCapabilities {
  type: string;
  query_form: QueryForm;
  query: boolean;
  ticket_exec: boolean;
  metadata: boolean;
  table_permission: boolean;
  field_masking: boolean;
  sql_parse: boolean;
  export: boolean;
  explain: boolean;
  parameterized: boolean;
}

interface CapabilitiesResponse {
  code: number;
  message: string;
  data: DatasourceCapabilities;
}

export async function fetchDatasourceCapabilities(
  datasourceId: number,
): Promise<DatasourceCapabilities> {
  const res = await api.get<CapabilitiesResponse>(
    `/datasources/${datasourceId}/capabilities`,
  );
  return res.data;
}
