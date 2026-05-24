import { describe, it, expect } from "vitest";
import {
  getMaskTypeLabel,
  getSensitivityLabel,
  SENSITIVITY_BADGE,
  MASK_TYPE_OPTIONS,
  SENSITIVITY_OPTIONS,
} from "@/api/maskRule";

describe("MaskRule API Helpers", () => {
  // --- getMaskTypeLabel ---

  describe("getMaskTypeLabel", () => {
    it.each([
      ["phone", "手机号"],
      ["id_card", "身份证"],
      ["name", "姓名"],
      ["email", "邮箱"],
      ["bank_card", "银行卡"],
      ["address", "地址"],
      ["full", "全掩码"],
      ["custom", "自定义正则"],
    ] as [string, string][])("maps %s to %s", (type, expected) => {
      expect(getMaskTypeLabel(type)).toBe(expected);
    });

    it("returns raw value for unknown type", () => {
      expect(getMaskTypeLabel("unknown")).toBe("unknown");
    });
  });

  // --- getSensitivityLabel ---

  describe("getSensitivityLabel", () => {
    it.each([
      ["low", "低"],
      ["medium", "中"],
      ["high", "高"],
    ] as [string, string][])("maps %s to %s", (level, expected) => {
      expect(getSensitivityLabel(level)).toBe(expected);
    });

    it("returns raw value for unknown level", () => {
      expect(getSensitivityLabel("unknown")).toBe("unknown");
    });
  });

  // --- SENSITIVITY_BADGE ---

  describe("SENSITIVITY_BADGE", () => {
    it("has correct entries for low, medium, high", () => {
      expect(SENSITIVITY_BADGE.low).toEqual({
        label: "低",
        cls: "bg-emerald-500/20 text-emerald-400",
      });
      expect(SENSITIVITY_BADGE.medium).toEqual({
        label: "中",
        cls: "bg-yellow-500/20 text-yellow-400",
      });
      expect(SENSITIVITY_BADGE.high).toEqual({
        label: "高",
        cls: "bg-red-500/20 text-red-400",
      });
    });
  });

  // --- MASK_TYPE_OPTIONS ---

  describe("MASK_TYPE_OPTIONS", () => {
    it("has 8 entries", () => {
      expect(MASK_TYPE_OPTIONS).toHaveLength(8);
    });

    it("each entry has value and label", () => {
      for (const opt of MASK_TYPE_OPTIONS) {
        expect(opt.value).toBeTruthy();
        expect(opt.label).toBeTruthy();
      }
    });
  });

  // --- SENSITIVITY_OPTIONS ---

  describe("SENSITIVITY_OPTIONS", () => {
    it("has 3 entries", () => {
      expect(SENSITIVITY_OPTIONS).toHaveLength(3);
    });

    it("contains low, medium, high", () => {
      const values = SENSITIVITY_OPTIONS.map((o) => o.value);
      expect(values).toEqual(["low", "medium", "high"]);
    });
  });
});
