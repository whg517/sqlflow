import { api } from "./client";

// --- Types ---

export interface WebhookSubscription {
  id: number;
  name: string;
  url: string; // masked
  events: string[]; // JSON array
  enabled: boolean;
  failure_count: number;
  last_triggered_at: string | null;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CreateSubscriptionResponse extends WebhookSubscription {
  secret: string; // Only returned once on creation
}

export interface CreateSubscriptionRequest {
  name: string;
  url: string;
  events: string[];
}

export interface UpdateSubscriptionRequest {
  name?: string;
  url?: string;
  events?: string[];
}

// --- Event Definitions ---

export const WEBHOOK_EVENTS = [
  { key: "ticket.created", label: "工单创建", category: "ticket" },
  { key: "ticket.approved", label: "审批通过", category: "ticket" },
  { key: "ticket.rejected", label: "审批驳回", category: "ticket" },
  { key: "ticket.executed", label: "执行完成", category: "ticket" },
  { key: "sla.warning", label: "SLA 预警", category: "sla" },
  { key: "sla.breached", label: "SLA 违约", category: "sla" },
] as const;

export const EVENT_CATEGORIES = {
  ticket: { label: "工单事件", color: "blue" },
  sla: { label: "SLA 事件", color: "yellow" },
} as const;

export type WebhookEventKey = (typeof WEBHOOK_EVENTS)[number]["key"];

// --- API Functions ---

export async function listSubscriptions(): Promise<{
  code: number;
  data: WebhookSubscription[];
}> {
  return api.get("/admin/webhooks/subscriptions");
}

export async function getSubscription(
  id: number,
): Promise<{ code: number; data: WebhookSubscription }> {
  return api.get(`/admin/webhooks/subscriptions/${id}`);
}

export async function createSubscription(
  req: CreateSubscriptionRequest,
): Promise<{ code: number; data: CreateSubscriptionResponse }> {
  return api.post("/admin/webhooks/subscriptions", req);
}

export async function updateSubscription(
  id: number,
  req: UpdateSubscriptionRequest,
): Promise<{ code: number; data: WebhookSubscription }> {
  return api.put(`/admin/webhooks/subscriptions/${id}`, req);
}

export async function deleteSubscription(
  id: number,
): Promise<{ code: number; data: string }> {
  return api.delete(`/admin/webhooks/subscriptions/${id}`);
}

export async function toggleSubscription(
  id: number,
): Promise<{ code: number; data: WebhookSubscription }> {
  return api.post(`/admin/webhooks/subscriptions/${id}/toggle`);
}

export async function testSubscription(
  id: number,
): Promise<{ code: number; data: string }> {
  return api.post(`/admin/webhooks/subscriptions/${id}/test`);
}

// --- URL Validation ---

const HTTPS_PREFIX = "https://";
const HTTP_PREFIX = "http://";

export interface UrlValidationResult {
  error: string | null;
  warning: string | null;
}

export function validateWebhookURL(url: string): UrlValidationResult {
  if (!url.trim()) return { error: "请输入 Webhook URL", warning: null };
  if (!url.startsWith(HTTP_PREFIX) && !url.startsWith(HTTPS_PREFIX)) {
    return { error: "URL 必须以 http:// 或 https:// 开头", warning: null };
  }
  try {
    new URL(url);
  } catch {
    return { error: "URL 格式无效", warning: null };
  }
  if (url.length > 2048) return { error: "URL 长度不能超过 2048 个字符", warning: null };
  if (!url.startsWith(HTTPS_PREFIX)) {
    return { error: null, warning: "建议使用 HTTPS 以确保安全性" };
  }
  return { error: null, warning: null };
}

// --- Failure Status Helpers ---

export function getFailureStatus(count: number): {
  level: "normal" | "warning" | "danger" | "critical";
  color: string;
  label: string;
} {
  if (count === 0) return { level: "normal", color: "emerald", label: "正常" };
  if (count <= 3) return { level: "warning", color: "yellow", label: `${count} 次失败` };
  if (count <= 9) return { level: "danger", color: "orange", label: `${count} 次失败` };
  return { level: "critical", color: "red", label: `${count} 次失败（已禁用）` };
}

export function maskUrl(url: string): string {
  try {
    const parsed = new URL(url);
    const pathParts = parsed.pathname.split("/");
    if (pathParts.length > 2) {
      const lastPart = pathParts[pathParts.length - 1];
      if (lastPart.length > 8) {
        pathParts[pathParts.length - 1] =
          lastPart.slice(0, 4) + "..." + lastPart.slice(-4);
      }
    }
    return `${parsed.protocol}//${parsed.host}${pathParts.join("/")}`;
  } catch {
    return url.length > 40 ? url.slice(0, 20) + "..." + url.slice(-20) : url;
  }
}
