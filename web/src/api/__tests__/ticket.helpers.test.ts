import { describe, it, expect, beforeEach } from "vitest";
import {
  getStatusLabel,
  getStatusColor,
  getRiskLabel,
  getRiskColor,
  getRiskDot,
  formatTime,
  type TicketStatus,
} from "@/api/ticket";

// --- Tests ---

describe("Ticket API Helpers", () => {
  // --- getStatusLabel ---

  describe("getStatusLabel", () => {
    it.each([
      ["SUBMITTED", "已提交"],
      ["AI_REVIEWED", "AI 已评审"],
      ["PENDING_APPROVAL", "待审批"],
      ["APPROVED", "已通过"],
      ["REJECTED", "已拒绝"],
      ["EXECUTING", "执行中"],
      ["DONE", "已完成"],
      ["CANCELLED", "已取消"],
    ] as [TicketStatus, string][])("maps %s to %s", (status, expected) => {
      expect(getStatusLabel(status)).toBe(expected);
    });

    it("returns raw value for unknown status", () => {
      expect(getStatusLabel("UNKNOWN" as TicketStatus)).toBe("UNKNOWN");
    });
  });

  // --- getStatusColor ---

  describe("getStatusColor", () => {
    it("returns correct color class for each status", () => {
      expect(getStatusLabel("SUBMITTED")).toBeDefined();
      expect(getStatusLabel("APPROVED")).toBeDefined();
      expect(getStatusLabel("REJECTED")).toBeDefined();
    });

    it("each status has a unique color class", () => {
      const statuses: TicketStatus[] = [
        "SUBMITTED",
        "AI_REVIEWED",
        "PENDING_APPROVAL",
        "APPROVED",
        "REJECTED",
        "EXECUTING",
        "DONE",
        "CANCELLED",
      ];
      const colors = statuses.map((s) => getStatusColor(s));
      const uniqueColors = new Set(colors);
      expect(uniqueColors.size).toBe(statuses.length);
    });

    it("returns default color for unknown status", () => {
      expect(getStatusColor("UNKNOWN" as TicketStatus)).toBe(
        "bg-gray-500/20 text-gray-400",
      );
    });
  });

  // --- getRiskLabel ---

  describe("getRiskLabel", () => {
    it.each([
      ["low", "低风险"],
      ["medium", "中风险"],
      ["high", "高风险"],
    ])("maps %s to %s", (level, expected) => {
      expect(getRiskLabel(level)).toBe(expected);
    });

    it("returns raw value for unknown risk level", () => {
      expect(getRiskLabel("critical")).toBe("critical");
    });
  });

  // --- getRiskColor ---

  describe("getRiskColor", () => {
    it.each([
      ["low", "bg-emerald-500/20 text-emerald-400"],
      ["medium", "bg-yellow-500/20 text-yellow-400"],
      ["high", "bg-red-500/20 text-red-400"],
    ])("maps %s to correct color", (level, expected) => {
      expect(getRiskColor(level)).toBe(expected);
    });

    it("returns default color for unknown risk level", () => {
      expect(getRiskColor("critical")).toBe("bg-gray-500/20 text-gray-400");
    });
  });

  // --- getRiskDot ---

  describe("getRiskDot", () => {
    it.each([
      ["low", "bg-emerald-400"],
      ["medium", "bg-yellow-400"],
      ["high", "bg-red-400"],
    ])("maps %s to correct dot color", (level, expected) => {
      expect(getRiskDot(level)).toBe(expected);
    });

    it("returns default dot color for unknown risk level", () => {
      expect(getRiskDot("critical")).toBe("bg-gray-400");
    });
  });

  // --- formatTime ---

  describe("formatTime", () => {
    it("formats ISO date string to MM-DD HH:mm", () => {
      expect(formatTime("2026-05-23T10:30:00Z")).toMatch(
        /^\d{2}-\d{2} \d{2}:\d{2}$/,
      );
    });

    it("formats date with single-digit month/day with leading zeros", () => {
      const result = formatTime("2026-01-05T08:05:00Z");
      // formatTime uses local timezone (getMonth/getDate/getHours/getMinutes)
      const d = new Date("2026-01-05T08:05:00Z");
      const month = String(d.getMonth() + 1).padStart(2, "0");
      const day = String(d.getDate()).padStart(2, "0");
      expect(result).toMatch(new RegExp(`^${month}-${day} \\d{2}:05$`));
    });

    it("formats midnight correctly", () => {
      const result = formatTime("2026-05-23T00:00:00Z");
      expect(result).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}$/);
    });

    it("formats end of day correctly", () => {
      const result = formatTime("2026-05-23T23:59:59Z");
      expect(result).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}$/);
    });
  });
});
