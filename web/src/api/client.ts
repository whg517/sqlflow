import { toast } from "sonner";

const API_BASE = "/api";
const TIMEOUT_MS = 30_000;

const STATUS_MESSAGES: Record<number, string> = {
  400: "请求参数有误",
  404: "请求的资源不存在",
  409: "操作冲突，请刷新后重试",
  429: "操作过于频繁，请稍后再试",
};

// --- Token storage ---

function getAccessToken(): string | null {
  return localStorage.getItem("token");
}

function getRefreshToken(): string | null {
  return localStorage.getItem("refresh_token");
}

function setTokens(accessToken: string, refreshToken: string) {
  localStorage.setItem("token", accessToken);
  localStorage.setItem("refresh_token", refreshToken);
}

function clearTokens() {
  localStorage.removeItem("token");
  localStorage.removeItem("refresh_token");
}

// --- Token refresh queue ---

type TokenSubscriber = (token: string) => void;
let isRefreshing = false;
let refreshSubscribers: TokenSubscriber[] = [];

function subscribeTokenRefresh(cb: TokenSubscriber) {
  refreshSubscribers.push(cb);
}

function notifySubscribers(token: string) {
  refreshSubscribers.forEach((cb) => cb(token));
  refreshSubscribers = [];
}

function rejectSubscribers() {
  refreshSubscribers = [];
}

async function refreshAccessToken(): Promise<string> {
  const refreshToken = getRefreshToken();
  if (!refreshToken) {
    throw new Error("No refresh token available");
  }

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 10_000);

  let res: Response;
  try {
    res = await fetch(`${API_BASE}/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
      signal: controller.signal,
    });
  } finally {
    clearTimeout(timeoutId);
  }

  if (!res.ok) {
    throw new Error("Refresh token request failed");
  }

  const data = await res.json();
  const newAccessToken = data.data?.access_token;
  const newRefreshToken = data.data?.refresh_token;

  if (!newAccessToken || !newRefreshToken) {
    throw new Error("Invalid refresh response");
  }

  setTokens(newAccessToken, newRefreshToken);
  return newAccessToken;
}

// --- Unauthorized handler ---

function handleUnauthorized() {
  clearTokens();
  if (window.location.pathname !== "/login") {
    toast.error("登录已过期，请重新登录");
    window.location.href = "/login";
  }
}

// --- Error extraction ---

async function extractErrorMessage(res: Response): Promise<string> {
  const data = await res.json().catch(() => ({}));
  return data.message || data.error || "";
}

// --- Core request function ---

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const token = getAccessToken();
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

  // --- 401: attempt token refresh ---
  if (res.status === 401) {
    // Don't retry the refresh endpoint itself
    if (path === "/auth/refresh") {
      handleUnauthorized();
      throw new Error("Unauthorized");
    }

    // If another request is already refreshing, queue this one
    if (isRefreshing) {
      return new Promise<T>((resolve, reject) => {
        subscribeTokenRefresh(async (newToken: string) => {
          try {
            // Retry original request with the new token
            headers["Authorization"] = `Bearer ${newToken}`;
            resolve(request<T>(method, path, body));
          } catch (err) {
            reject(err);
          }
        });
      });
    }

    // Kick off a refresh
    isRefreshing = true;

    try {
      const newToken = await refreshAccessToken();
      // Notify all queued requests with the new token
      notifySubscribers(newToken);
      // Retry the original request with new token
      return request<T>(method, path, body);
    } catch {
      // Refresh failed — reject queued subscribers and redirect
      rejectSubscribers();
      handleUnauthorized();
      throw new Error("Unauthorized");
    } finally {
      isRefreshing = false;
    }
  }

  // --- 403 Forbidden ---
  if (res.status === 403) {
    window.location.href = "/403";
    throw new Error("Forbidden");
  }

  // --- 5xx Server Error ---
  if (res.status >= 500) {
    const serverMsg = await extractErrorMessage(res);
    toast.error(serverMsg || "服务器错误，请稍后重试");
    throw new Error(serverMsg || `Server error: ${res.status}`);
  }

  // --- Other client errors ---
  if (!res.ok) {
    const serverMsg = await extractErrorMessage(res);
    const userMsg =
      STATUS_MESSAGES[res.status] || serverMsg || `请求失败 (${res.status})`;
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
  /** Fetch a binary response (Blob) with auth headers. */
  async getBlob(path: string): Promise<Blob> {
    const token = getAccessToken();
    const headers: Record<string, string> = {};
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);
    let res: Response;
    try {
      res = await fetch(`${API_BASE}${path}`, {
        method: "GET",
        headers,
        signal: controller.signal,
      });
    } finally {
      clearTimeout(timeoutId);
    }
    if (res.status === 401) {
      handleUnauthorized();
      throw new Error("Unauthorized");
    }
    if (res.status === 403) {
      throw new Error("Forbidden");
    }
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      const msg = data.message || `导出失败 (${res.status})`;
      throw new Error(msg);
    }
    return res.blob();
  },
};
