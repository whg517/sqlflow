import { describe, it, expect } from "vitest";
import type { DatasourceType } from "@/shared/datasource/types";
import {
  buildPayload,
  initialValues,
  validate,
  visibleFields,
} from "../datasourceForm";

// The Settings feature had exactly one test file before this, and it covered
// the dashboard API. The datasource form — credentials, connection testing and
// every driver's config shape — had none, and could not have had any: 15
// useState calls with validation, payload assembly and fetch all inlined in one
// component. These run against the same driver-declared schema the server
// sends.

const mysql: DatasourceType = {
  type: "mysql",
  query_form: "sql",
  placeholder_style: "positional",
  fields: [
    { name: "host", label: "主机", kind: "text", required: true, storage: "column" },
    { name: "port", label: "端口", kind: "number", required: true, default: "3306", storage: "column" },
    { name: "username", label: "用户名", kind: "text", required: true, storage: "column" },
    { name: "password", label: "密码", kind: "password", required: true, storage: "column", secret: true },
    { name: "database", label: "默认数据库", kind: "text", required: false, storage: "column" },
    { name: "max_open", label: "最大连接数", kind: "number", required: false, default: "10", storage: "column" },
  ],
};

const sqlite: DatasourceType = {
  type: "sqlite",
  query_form: "sql",
  placeholder_style: "positional",
  fields: [
    { name: "database", label: "SQLite 文件路径", kind: "text", required: true, storage: "column" },
  ],
};

const elasticsearch: DatasourceType = {
  type: "elasticsearch",
  query_form: "dsl",
  placeholder_style: "none",
  fields: [
    { name: "urls", label: "节点地址", kind: "text", required: true, storage: "extra" },
    {
      name: "auth_type", label: "认证方式", kind: "select", required: false,
      default: "basic", storage: "extra",
      options: [
        { value: "basic", label: "Basic Auth" },
        { value: "api_key", label: "API Key" },
        { value: "none", label: "无认证" },
      ],
    },
    {
      name: "username", label: "用户名", kind: "text", required: true, storage: "column",
      show_when: { field: "auth_type", equals: ["basic"] },
    },
    {
      name: "password", label: "密码", kind: "password", required: true, storage: "column",
      secret: true, show_when: { field: "auth_type", equals: ["basic"] },
    },
    {
      name: "es_api_key", label: "API Key", kind: "password", required: true, storage: "column",
      secret: true, show_when: { field: "auth_type", equals: ["api_key"] },
    },
    { name: "verify_certs", label: "校验证书", kind: "switch", required: false, default: "true", storage: "extra" },
  ],
};

describe("initialValues", () => {
  it("seeds every field from the schema's declared default", () => {
    expect(initialValues(mysql)).toEqual({
      host: "", port: "3306", username: "", password: "", database: "", max_open: "10",
    });
  });

  it("returns an empty form for an unknown type instead of throwing", () => {
    expect(initialValues(undefined)).toEqual({});
  });
});

describe("visibleFields", () => {
  it("shows the credential pair that matches the chosen auth type", () => {
    const basic = visibleFields(elasticsearch, { auth_type: "basic" }).map((f) => f.name);
    expect(basic).toContain("username");
    expect(basic).toContain("password");
    expect(basic).not.toContain("es_api_key");

    const apiKey = visibleFields(elasticsearch, { auth_type: "api_key" }).map((f) => f.name);
    expect(apiKey).toContain("es_api_key");
    expect(apiKey).not.toContain("username");
  });

  it("hides both when authentication is off", () => {
    const none = visibleFields(elasticsearch, { auth_type: "none" }).map((f) => f.name);
    expect(none).not.toContain("username");
    expect(none).not.toContain("es_api_key");
    expect(none).toContain("urls");
  });

  it("shows a field with no condition regardless of the other values", () => {
    expect(visibleFields(mysql, {}).map((f) => f.name)).toHaveLength(6);
  });
});

describe("validate", () => {
  it("requires only what the schema marks required", () => {
    const errors = validate(mysql, initialValues(mysql), false);
    expect(Object.keys(errors).sort()).toEqual(["host", "password", "username"]);
    // database and max_open are optional, so their blanks are not errors.
    expect(errors.database).toBeUndefined();
  });

  it("does not demand a secret again when editing", () => {
    const values = { ...initialValues(mysql), host: "db", username: "root", password: "" };
    expect(validate(mysql, values, true).password).toBeUndefined();
    expect(validate(mysql, values, false).password).toBeDefined();
  });

  it("rejects a port outside the valid range", () => {
    const base = { ...initialValues(mysql), host: "db", username: "root", password: "p" };
    expect(validate(mysql, { ...base, port: "0" }, false).port).toBe("端口范围 1-65535");
    expect(validate(mysql, { ...base, port: "70000" }, false).port).toBe("端口范围 1-65535");
    expect(validate(mysql, { ...base, port: "5432" }, false).port).toBeUndefined();
  });

  it("does not require a hidden field", () => {
    // Under basic auth the API key input is not on screen, so it cannot be
    // required — the old form got this right by branching, and this is the
    // property that replaces the branch.
    const values = { urls: "https://es:9200", auth_type: "basic", username: "u", password: "p" };
    expect(validate(elasticsearch, values, false).es_api_key).toBeUndefined();
  });

  it("requires a SQLite path and nothing else", () => {
    expect(Object.keys(validate(sqlite, initialValues(sqlite), false))).toEqual(["database"]);
  });
});

describe("buildPayload", () => {
  it("sends column fields at the top level and extra fields under extra_config", () => {
    const body = buildPayload(
      elasticsearch,
      {
        urls: "https://a:9200, https://b:9200",
        auth_type: "basic",
        username: "elastic",
        password: "secret",
        verify_certs: "true",
      },
      "prod-es",
    );

    expect(body.name).toBe("prod-es");
    expect(body.type).toBe("elasticsearch");
    expect(body.username).toBe("elastic");
    expect(body.password).toBe("secret");
    expect(body.extra_config).toEqual({
      urls: ["https://a:9200", "https://b:9200"],
      auth_type: "basic",
      verify_certs: true,
    });
    // The five es_* keys the form used to send as named fields are gone.
    expect(body.es_urls).toBeUndefined();
    expect(body.es_verify_certs).toBeUndefined();
  });

  it("omits a blank secret so an edit does not clear a stored credential", () => {
    const body = buildPayload(
      mysql,
      { host: "db", port: "3306", username: "root", password: "", database: "app", max_open: "10" },
      "prod",
    );
    expect("password" in body).toBe(false);
    expect(body.host).toBe("db");
  });

  it("coerces numbers and switches out of their string form", () => {
    const body = buildPayload(
      mysql,
      { host: "db", port: "3306", username: "root", password: "p", database: "", max_open: "25" },
      "prod",
    );
    expect(body.port).toBe(3306);
    expect(body.max_open).toBe(25);
  });

  it("leaves a hidden field out entirely", () => {
    const body = buildPayload(
      elasticsearch,
      { urls: "https://es:9200", auth_type: "api_key", es_api_key: "k", verify_certs: "false" },
      "es",
    );
    expect(body.es_api_key).toBe("k");
    expect("username" in body).toBe(false);
    expect((body.extra_config as Record<string, unknown>).verify_certs).toBe(false);
  });

  it("sends only what SQLite needs", () => {
    const body = buildPayload(sqlite, { database: "/tmp/app.db" }, "local");
    expect(body).toEqual({ name: "local", type: "sqlite", database: "/tmp/app.db" });
  });
});
