import { api } from "./client";

// --- Types ---

export type GitLinkType = "commit" | "pr";

export interface GitLink {
  id: number;
  entity_type: "ticket" | "audit_log";
  entity_id: number;
  link_type: GitLinkType;
  commit_hash: string;
  commit_message: string;
  author_name: string;
  author_email: string;
  pr_number: number;
  pr_title: string;
  pr_url: string;
  repo_url: string;
  branch: string;
  created_by: number;
  created_at: string;
}

export interface CreateGitLinkRequest {
  entity_type: "ticket" | "audit_log";
  entity_id: number;
  link_type: GitLinkType;
  commit_hash?: string;
  commit_message?: string;
  author_name?: string;
  author_email?: string;
  pr_number?: number;
  pr_title?: string;
  pr_url?: string;
  repo_url?: string;
  branch?: string;
}

export interface GitLinkListResponse {
  code: number;
  message: string;
  data: GitLink[];
}

export interface GitLinkResponse {
  code: number;
  message: string;
  data: GitLink;
}

// --- API Functions ---

export async function listGitLinks(
  entityType: string,
  entityId: number,
): Promise<GitLinkListResponse> {
  const qs = new URLSearchParams({
    entity_type: entityType,
    entity_id: String(entityId),
  });
  return api.get<GitLinkListResponse>(`/git-links?${qs.toString()}`);
}

export async function createGitLink(
  req: CreateGitLinkRequest,
): Promise<GitLinkResponse> {
  return api.post<GitLinkResponse>("/git-links", req);
}

export async function deleteGitLink(id: number): Promise<{ code: number; message: string }> {
  return api.del<{ code: number; message: string }>(`/git-links/${id}`);
}

// --- Helpers ---

/** Shorten commit hash for display (e.g. "abc123456789" → "abc1234") */
export function shortenHash(hash: string): string {
  if (!hash) return "";
  return hash.length > 7 ? hash.slice(0, 7) : hash;
}
