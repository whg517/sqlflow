import { api } from "./client";

// --- Types ---

export interface User {
  id: number;
  username: string;
  role: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface ListUsersResponse {
  code: number;
  message: string;
  data: {
    users: User[];
    total: number;
  };
}

export interface CreateUserRequest {
  username: string;
  password: string;
  role: string;
}

export interface UpdateUserRequest {
  username?: string;
  role?: string;
  status?: string;
}

export interface ResetPasswordRequest {
  password: string;
}

export interface UserActionResponse {
  code: number;
  message: string;
  data?: User;
}

// --- API Functions ---

export async function listUsers(
  page = 1,
  pageSize = 20,
): Promise<ListUsersResponse> {
  return api.get<ListUsersResponse>(
    `/users?page=${page}&page_size=${pageSize}`,
  );
}

export async function createUser(
  req: CreateUserRequest,
): Promise<UserActionResponse> {
  return api.post<UserActionResponse>("/users", req);
}

export async function updateUser(
  id: number,
  req: UpdateUserRequest,
): Promise<UserActionResponse> {
  return api.put<UserActionResponse>(`/users/${id}`, req);
}

export async function deleteUser(id: number): Promise<UserActionResponse> {
  return api.del<UserActionResponse>(`/users/${id}`);
}

export async function resetPassword(
  id: number,
  req: ResetPasswordRequest,
): Promise<UserActionResponse> {
  return api.put<UserActionResponse>(`/users/${id}/reset-password`, req);
}

// --- Helpers ---

export const ROLE_LABEL_MAP: Record<string, string> = {
  admin: "管理员",
  dba: "DBA",
  developer: "开发人员",
};

export const ROLE_BADGE_CLASS: Record<string, string> = {
  admin: "bg-orange-500/20 text-orange-400",
  dba: "bg-violet-500/20 text-violet-400",
  developer: "bg-blue-500/20 text-blue-400",
};

export const ROLE_OPTIONS = [
  { value: "admin", label: "管理员" },
  { value: "dba", label: "DBA" },
  { value: "developer", label: "开发人员" },
];
