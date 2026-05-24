import { describe, it, expect } from "vitest";
import { formatCommentTime } from "@/api/comment";

describe("Comment API Helpers", () => {
  describe("formatCommentTime", () => {
    it('returns "刚刚" for less than 1 minute ago', () => {
      const now = new Date();
      const tenSecondsAgo = new Date(now.getTime() - 10 * 1000).toISOString();
      expect(formatCommentTime(tenSecondsAgo)).toBe("刚刚");
    });

    it("returns minutes for less than 1 hour ago", () => {
      const now = new Date();
      const fiveMinAgo = new Date(now.getTime() - 5 * 60 * 1000).toISOString();
      expect(formatCommentTime(fiveMinAgo)).toBe("5 分钟前");
    });

    it("returns hours for less than 24 hours ago", () => {
      const now = new Date();
      const threeHoursAgo = new Date(
        now.getTime() - 3 * 3600 * 1000,
      ).toISOString();
      expect(formatCommentTime(threeHoursAgo)).toBe("3 小时前");
    });

    it("returns days for less than 7 days ago", () => {
      const now = new Date();
      const twoDaysAgo = new Date(
        now.getTime() - 2 * 86400 * 1000,
      ).toISOString();
      expect(formatCommentTime(twoDaysAgo)).toBe("2 天前");
    });

    it("returns formatted date for more than 7 days ago", () => {
      const now = new Date();
      const tenDaysAgo = new Date(
        now.getTime() - 10 * 86400 * 1000,
      ).toISOString();
      const result = formatCommentTime(tenDaysAgo);
      expect(result).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}$/);
    });
  });
});
