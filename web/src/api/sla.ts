import { api } from "./client";

// --- Types ---

export interface SLAConfig {
  id: number;
  priority: string;
  timeout_minutes: number;
  reminder_percent: number;
  escalate_to_role: string;
  escalate_to_user: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface SLANotification {
  id: number;
  ticket_id: number;
  notification_type: "reminder" | "escalate";
  stage: string;
  notified_user: string;
  notified_at: string;
  sla_config_id: number;
}

export interface SLATicketStatus {
  ticket_id: number;
  sla_deadline: string | null;
  sla_status: "normal" | "warning" | "breached" | "";
  time_remaining_seconds: number;
}

// --- API Functions ---

export async function listSLAConfigs(): Promise<
  { code: number; data: SLAConfig[] }
> {
  return api.get("/settings/sla");
}

export async function createSLAConfig(
  data: Omit<SLAConfig, "id" | "created_at" | "updated_at">,
): Promise<{ code: number; data: SLAConfig }> {
  return api.post("/settings/sla", data);
}

export async function updateSLAConfig(
  id: number,
  data: Omit<SLAConfig, "id" | "created_at" | "updated_at">,
): Promise<{ code: number }> {
  return api.put(`/settings/sla/${id}`, data);
}

export async function deleteSLAConfig(
  id: number,
): Promise<{ code: number }> {
  return api.del(`/settings/sla/${id}`);
}

export async function getTicketSLAStatuses(
  ticketIds: number[],
): Promise<{ code: number; data: Record<string, SLATicketStatus> }> {
  const qs = new URLSearchParams();
  qs.set("ticket_ids", ticketIds.join(","));
  return api.get(`/tickets/sla-status?${qs}`);
}

export async function listSLANotifications(
  page = 1,
  pageSize = 20,
): Promise<{
  code: number;
  data: SLANotification[];
  total: number;
  page: number;
  page_size: number;
}> {
  const qs = new URLSearchParams();
  qs.set("page", String(page));
  qs.set("page_size", String(pageSize));
  return api.get(`/sla-notifications?${qs}`);
}

// --- Helpers ---

export function getSLAStatusLabel(status: string): string {
  switch (status) {
    case "warning":
      return "即将超时";
    case "breached":
      return "已超时";
    default:
      return "正常";
  }
}

export function getSLAStatusColor(status: string): string {
  switch (status) {
    case "warning":
      return "bg-yellow-500/20 text-yellow-400";
    case "breached":
      return "bg-red-500/20 text-red-400";
    default:
      return "bg-emerald-500/20 text-emerald-400";
  }
}

export function getSLADot(status: string): string {
  switch (status) {
    case "warning":
      return "bg-yellow-400";
    case "breached":
      return "bg-red-400";
    default:
      return "bg-emerald-400";
  }
}

export function formatSLARemaining(seconds: number): string {
  if (seconds <= 0) return "已超时";
  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}
