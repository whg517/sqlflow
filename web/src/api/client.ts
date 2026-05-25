import { toast } from "sonner";

const API_BASE = "/api";
const TIMEOUT_MS = 30_000;

const STATUS_MESSAGES: Record<number, string> = {
  400: "请求参数有误",
  404: "请求的资源不存在",
  409: "操作冲突，请刷新后重试",
  429: "操作过于频繁，请稍后再试",
};

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

async function extractErrorMessage(res: Response): Promise<string> {
  const data = await res.json().catch(() => ({}));
  return data.message || data.error || "";
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

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);

  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      method,
      headers,
      body: body != null ? JSON.stringify(body) : undefined,
      signal: controller.signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      toast.error("请求超时，请检查网络后重试");
      throw new Error("请求超时");
    }
    if (err instanceof TypeError) {
      toast.error("网络连接失败，请检查网络");
      throw new Error("网络连接失败");
    }
    throw err;
  } finally {
    clearTimeout(timeoutId);
  }

  if (res.status === 401) {
    await extractErrorMessage(res);
    handleUnauthorized();
    throw new Error("Unauthorized");
  }

  if (res.status === 403) {
    window.location.href = "/403";
    throw new Error("Forbidden");
  }

  if (res.status >= 500) {
    const serverMsg = await extractErrorMessage(res);
    toast.error(serverMsg || "服务器错误，请稍后重试");
    throw new Error(serverMsg || `Server error: ${res.status}`);
  }

  if (!res.ok) {
    const serverMsg = await extractErrorMessage(res);
    const userMsg = STATUS_MESSAGES[res.status] || serverMsg || `请求失败 (${res.status})`;
    toast.error(userMsg);
    throw new Error(serverMsg || `Request failed: ${res.status}`);
  }

  return res.json();
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body: unknown) => request<T>("POST", path, body),
  put: <T>(path: string, body: unknown) => request<T>("PUT", path, body),
  del: <T>(path: string) => request<T>("DELETE", path),
};
