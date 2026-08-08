import { api } from "@/shared/api/client";

// What a data source type needs in order to connect, as the driver declares it.
//
// This is the fourth axis, alongside capability, query form and result shape.
// Without it the Settings form had to know: it branched on type name 28 times,
// held the type list twice (a hardcoded option per type and a default-port map),
// and carried five Elasticsearch fields the backend had already collapsed into
// extra_config. A registered driver the form had not been told about could not
// even be selected.

export type FieldKind = "text" | "password" | "number" | "select" | "switch";

/** Which half of the request payload a value belongs in. */
export type FieldStorage = "column" | "extra";

export interface ConfigOption {
  value: string;
  label: string;
}

/** Hides a field unless another field currently holds one of these values. */
export interface FieldCondition {
  field: string;
  equals: string[];
}

export interface ConfigField {
  name: string;
  label: string;
  kind: FieldKind;
  required: boolean;
  placeholder?: string;
  help?: string;
  default?: string;
  options?: ConfigOption[];
  storage: FieldStorage;
  /** Stored encrypted and never read back: blank on edit means "unchanged". */
  secret?: boolean;
  show_when?: FieldCondition;
}

/** How a parameterised template writes its bind tokens. */
export type PlaceholderStyle = "numbered" | "positional" | "none";

export interface DatasourceType {
  type: string;
  query_form: string;
  fields: ConfigField[];
  placeholder_style: PlaceholderStyle;
}

/** placeholderToken renders the bind token for a 1-based position. */
export function placeholderToken(style: PlaceholderStyle, position: number): string {
  switch (style) {
    case "numbered":
      return `$${position}`;
    case "positional":
      return "?";
    default:
      return "—";
  }
}

export async function fetchDatasourceTypes(): Promise<DatasourceType[]> {
  const res = await api.get<{ code: number; message: string; data: DatasourceType[] }>(
    "/datasource-types",
  );
  return res.data ?? [];
}
