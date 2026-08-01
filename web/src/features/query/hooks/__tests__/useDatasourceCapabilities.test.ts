import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import {
  useDatasourceCapabilities,
  __resetCapabilitiesCache,
} from "@/features/query/hooks/useDatasourceCapabilities";
import * as capabilitiesApi from "@/features/query/api/capabilities";

const esCapabilities: capabilitiesApi.DatasourceCapabilities = {
  type: "elasticsearch",
  query_form: "dsl",
  query: true,
  ticket_exec: false,
  metadata: true,
  table_permission: false,
  field_masking: true,
  sql_parse: false,
  export: true,
  explain: false,
  parameterized: false,
};

describe("useDatasourceCapabilities", () => {
  beforeEach(() => __resetCapabilitiesCache());
  afterEach(() => vi.restoreAllMocks());

  it("reports the driver-declared query form", async () => {
    vi.spyOn(capabilitiesApi, "fetchDatasourceCapabilities").mockResolvedValue(
      esCapabilities,
    );

    const { result } = renderHook(() => useDatasourceCapabilities(1));

    await waitFor(() => {
      expect(result.current.capabilities?.query_form).toBe("dsl");
    });
    expect(result.current.capabilities?.explain).toBe(false);
    expect(result.current.capabilities?.ticket_exec).toBe(false);
  });

  it("caches per datasource so tab switching does not refetch", async () => {
    const spy = vi
      .spyOn(capabilitiesApi, "fetchDatasourceCapabilities")
      .mockResolvedValue(esCapabilities);

    const first = renderHook(() => useDatasourceCapabilities(7));
    await waitFor(() => expect(first.result.current.capabilities).not.toBeNull());

    const second = renderHook(() => useDatasourceCapabilities(7));
    await waitFor(() =>
      expect(second.result.current.capabilities).not.toBeNull(),
    );

    expect(spy).toHaveBeenCalledTimes(1);
  });

  it("returns null without requesting when no datasource is selected", () => {
    const spy = vi.spyOn(capabilitiesApi, "fetchDatasourceCapabilities");
    const { result } = renderHook(() => useDatasourceCapabilities(null));
    expect(result.current.capabilities).toBeNull();
    expect(spy).not.toHaveBeenCalled();
  });

  it("surfaces a failure instead of silently degrading", async () => {
    vi.spyOn(capabilitiesApi, "fetchDatasourceCapabilities").mockRejectedValue(
      new Error("boom"),
    );

    const { result } = renderHook(() => useDatasourceCapabilities(2));
    await waitFor(() => expect(result.current.error).toBe("boom"));
    expect(result.current.capabilities).toBeNull();
  });
});
