import { describe, it, expect } from "vitest";
import {
  getActionLabel,
  getActionColor,
  formatAuditTime,
  formatExecutionTime,
  actionOptions,
} from "@/api/audit";

describe("Audit API Helpers", () => {
  // --- getActionLabel ---

  describe("getActionLabel", () => {
    it.each([
      ["SELECT", "SELECT"],
      ["UPDATE", "UPDATE"],
      ["DELETE", "DELETE"],
      ["DDL", "DDL"],
      ["EXPORT", "导出"],
      ["INSERT", "INSERT"],
    ] as [string, string][])("maps %s to %s", (action, expected) => {
      expect(getActionLabel(action)).toBe(expected);
    });

    it("returns raw value for unknown action", () => {
      expect(getActionLabel("UNKNOWN")).toBe("UNKNOWN");
    });
  });

  // --- getActionColor ---

  describe("getActionColor", () => {
    it.each([
      ["SELECT", "bg-blue-500/20 text-blue-400"],
      ["UPDATE", "bg-yellow-500/20 text-yellow-400"],
      ["DELETE", "bg-red-500/20 text-red-400"],
      ["DDL", "bg-violet-500/20 text-violet-400"],
      ["EXPORT", "bg-emerald-500/20 text-emerald-400"],
      ["INSERT", "bg-amber-500/20 text-amber-400"],
    ] as [string, string][])("maps %s to correct color", (action, expected) => {
      expect(getActionColor(action)).toBe(expected);
    });

    it("returns default color for unknown action", () => {
      expect(getActionColor("UNKNOWN")).toBe("bg-slate-500/20 text-slate-400");
    });
  });

  // --- formatAuditTime ---

  describe("formatAuditTime", () => {
    it("formats ISO date string to MM-DD HH:mm:ss", () => {
      const result = formatAuditTime("2026-05-23T10:30:45Z");
      expect(result).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
    });

    it("handles midnight", () => {
      const result = formatAuditTime("2026-05-23T00:00:00Z");
      expect(result).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
    });

    it("handles end of day", () => {
      const result = formatAuditTime("2026-05-23T23:59:59Z");
      expect(result).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
    });
  });

  // --- formatExecutionTime ---

  describe("formatExecutionTime", () => {
    it("formats milliseconds when < 1000", () => {
      expect(formatExecutionTime(500)).toBe("500ms");
      expect(formatExecutionTime(1)).toBe("1ms");
      expect(formatExecutionTime(999)).toBe("999ms");
    });

    it("formats seconds when >= 1000", () => {
      expect(formatExecutionTime(1000)).toBe("1.00s");
      expect(formatExecutionTime(1500)).toBe("1.50s");
      expect(formatExecutionTime(10500)).toBe("10.50s");
    });

    it("handles zero", () => {
      expect(formatExecutionTime(0)).toBe("0ms");
    });
  });

  // --- actionOptions ---

  describe("actionOptions", () => {
    it("contains expected actions", () => {
      expect(actionOptions).toContain("SELECT");
      expect(actionOptions).toContain("UPDATE");
      expect(actionOptions).toContain("DELETE");
      expect(actionOptions).toContain("DDL");
      expect(actionOptions).toContain("EXPORT");
    });

    it("has 5 entries", () => {
      expect(actionOptions).toHaveLength(5);
    });
  });
});
