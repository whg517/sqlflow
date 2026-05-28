import { api } from "./client";
import type {
  CoverageSummary,
  ModuleListResponse,
  ModuleListParams,
  FileListResponse,
  FileListParams,
} from "@/types/coverage";

export async function getCoverageSummary(
  project: string,
  testType?: string,
): Promise<{ code: number; data: CoverageSummary }> {
  const params = new URLSearchParams();
  if (testType) params.set("test_type", testType);
  const qs = params.toString();
  return api.get(`/v1/coverage/${project}/summary${qs ? `?${qs}` : ""}`);
}

export async function getModuleList(
  project: string,
  params?: ModuleListParams,
): Promise<{ code: number; data: ModuleListResponse }> {
  const sp = new URLSearchParams();
  if (params?.test_type) sp.set("test_type", params.test_type);
  if (params?.sort) sp.set("sort", params.sort);
  if (params?.status) sp.set("status", params.status);
  if (params?.page) sp.set("page", String(params.page));
  if (params?.page_size) sp.set("page_size", String(params.page_size));
  const qs = sp.toString();
  return api.get(`/v1/coverage/${project}/modules${qs ? `?${qs}` : ""}`);
}

export async function getFileList(
  project: string,
  params: FileListParams,
): Promise<{ code: number; data: FileListResponse }> {
  const sp = new URLSearchParams();
  sp.set("module_path", params.module_path);
  if (params.test_type) sp.set("test_type", params.test_type);
  if (params.sort) sp.set("sort", params.sort);
  if (params.page) sp.set("page", String(params.page));
  if (params?.page_size) sp.set("page_size", String(params.page_size));
  return api.get(`/v1/coverage/${project}/files?${sp.toString()}`);
}
