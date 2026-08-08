import type {
  ConfigField,
  DatasourceType,
} from "@/shared/datasource/types";

// The datasource connection form, as data.
//
// Everything here used to be `if (form.type === "elasticsearch")` and friends,
// 28 times across a 1321-line component with no tests. Keeping it in pure
// functions over a driver-declared schema is what makes it testable at all, and
// is the same shape queryModes.ts already uses for the query editor: one object
// per axis the driver declares, no branching on type name.

export type FormValues = Record<string, string>;

/** initialValues seeds a blank form from the schema's declared defaults. */
export function initialValues(type: DatasourceType | undefined): FormValues {
  const values: FormValues = {};
  for (const field of type?.fields ?? []) {
    values[field.name] = field.default ?? "";
  }
  return values;
}

/**
 * visibleFields drops the fields whose condition is not met.
 *
 * Elasticsearch is why this exists: an API key input is meaningless under basic
 * auth. The form used to express that by comparing the type name to
 * "elasticsearch" and then the auth type to "api_key", in two places each.
 */
export function visibleFields(
  type: DatasourceType | undefined,
  values: FormValues,
): ConfigField[] {
  return (type?.fields ?? []).filter((field) => {
    if (!field.show_when) return true;
    return field.show_when.equals.includes(values[field.show_when.field] ?? "");
  });
}

/**
 * validate reports one message per field that is not acceptable.
 *
 * `editing` matters only for secrets: a stored credential is never read back,
 * so an empty box means "leave it alone" rather than "clear it".
 */
export function validate(
  type: DatasourceType | undefined,
  values: FormValues,
  editing: boolean,
): Record<string, string> {
  const errors: Record<string, string> = {};

  for (const field of visibleFields(type, values)) {
    const raw = (values[field.name] ?? "").trim();

    if (field.required && raw === "") {
      if (field.secret && editing) continue;
      errors[field.name] = `请输入${field.label}`;
      continue;
    }
    if (raw === "") continue;

    if (field.kind === "number") {
      const n = Number(raw);
      if (!Number.isInteger(n)) {
        errors[field.name] = `${field.label}必须是整数`;
      } else if (field.name === "port" && (n < 1 || n > 65535)) {
        errors[field.name] = "端口范围 1-65535";
      } else if (n < 0) {
        errors[field.name] = `${field.label}不能为负数`;
      }
    }
  }

  return errors;
}

/**
 * buildPayload turns the form into a create/update request body.
 *
 * Where each value goes is the schema's answer, not this function's: `column`
 * fields are top-level, `extra` fields are collected into extra_config, which
 * is the split the server has enforced since Elasticsearch's five columns were
 * removed. A blank secret is omitted so an edit does not wipe a stored
 * credential.
 */
export function buildPayload(
  type: DatasourceType | undefined,
  values: FormValues,
  name: string,
): Record<string, unknown> {
  const body: Record<string, unknown> = { name: name.trim(), type: type?.type ?? "" };
  const extra: Record<string, unknown> = {};

  for (const field of visibleFields(type, values)) {
    const raw = (values[field.name] ?? "").trim();
    if (field.secret && raw === "") continue;

    const value = coerce(field, raw);
    if (field.storage === "extra") {
      extra[field.name] = value;
    } else {
      body[field.name] = value;
    }
  }

  if (Object.keys(extra).length > 0) {
    body.extra_config = extra;
  }
  return body;
}

/** coerce turns the string a form holds into the JSON type the field declares. */
function coerce(field: ConfigField, raw: string): unknown {
  switch (field.kind) {
    case "number":
      return Number(raw) || 0;
    case "switch":
      return raw === "true";
    default:
      // A comma-separated list is still one text input to the user; the only
      // field that needs it today is Elasticsearch's node list, and the driver
      // is what decodes it.
      if (field.name === "urls") {
        return raw
          .split(",")
          .map((u) => u.trim())
          .filter(Boolean);
      }
      return raw;
  }
}

/**
 * valuesFromDatasource fills the form from a stored datasource.
 *
 * Column fields are read off the row and extra fields out of extra_config, by
 * name — the same split buildPayload writes with, so a round-trip is symmetric
 * without either side knowing which driver it is looking at. Secrets come back
 * blank because the server never returns them.
 */
export function valuesFromDatasource(
  type: DatasourceType | undefined,
  ds: Record<string, unknown>,
): FormValues {
  const extra = parseExtraConfig(ds.extra_config);
  const values: FormValues = {};

  for (const field of type?.fields ?? []) {
    if (field.secret) {
      values[field.name] = "";
      continue;
    }
    const raw = field.storage === "extra" ? extra[field.name] : ds[field.name];
    values[field.name] = stringify(raw) || (field.default ?? "");
  }
  return values;
}

function parseExtraConfig(raw: unknown): Record<string, unknown> {
  if (typeof raw === "object" && raw !== null) return raw as Record<string, unknown>;
  if (typeof raw !== "string" || raw.trim() === "") return {};
  try {
    const parsed: unknown = JSON.parse(raw);
    return typeof parsed === "object" && parsed !== null
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
}

function stringify(raw: unknown): string {
  if (raw === undefined || raw === null) return "";
  if (Array.isArray(raw)) return raw.join(", ");
  if (typeof raw === "boolean") return raw ? "true" : "false";
  if (typeof raw === "number") return raw === 0 ? "" : String(raw);
  return String(raw);
}

/**
 * describeAddress summarises where a datasource points, for the list view.
 *
 * Data-driven rather than type-driven: whatever the row has. The old version
 * asked whether the type was "sqlite", then whether it was "elasticsearch",
 * then fell through to host:port — three answers that happen to be what these
 * three shapes carry.
 */
export function describeAddress(ds: Record<string, unknown>): string {
  const host = stringify(ds.host);
  if (host !== "") {
    const port = stringify(ds.port);
    return port === "" ? host : `${host}:${port}`;
  }
  const extra = parseExtraConfig(ds.extra_config);
  const urls = stringify(extra.urls);
  if (urls !== "") return urls;
  return stringify(ds.database) || "—";
}
