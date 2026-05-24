import { toast } from "sonner";

const API_BASE = "/api";

function getToken(): string | null {
  return localStorage.getItem("token");
}

function handleUnauthorized() {
  localStorage.removeItem("token");
  localStorage.removeItem("refresh_token");
  // Avoid infinite redirect if already on login page
  if (window.location.pathname !== "/login") {
    toast.error("登录已过期，请重新登录");
    window.location.href = "/login";
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: body != null ? JSON.stringify(body) : undefined,
  });

  if (res.status === 401) {
    const data = await res.json().catch(() => ({}));
    handleUnauthorized();
    throw new Error(data.error || "Unauthorized");
  }

  if (res.status === 403) {
    window.location.href = "/403";
    throw new Error("Forbidden");
  }

  if (res.status >= 500) {
    toast.error("服务器错误，请稍后重试");
    throw new Error(`Server error: ${res.status}`);
  }

  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.message || `Request failed: ${res.status}`);
  }

  return res.json();
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body: unknown) => request<T>("POST", path, body),
  put: <T>(path: string, body: unknown) => request<T>("PUT", path, body),
  del: <T>(path: string) => request<T>("DELETE", path),
};
