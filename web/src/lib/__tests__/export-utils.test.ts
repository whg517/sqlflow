import { describe, it, expect, vi } from "vitest";
import {
  downloadBlob,
  formatFileSize,
  getExportExtension,
  getExportMimeType,
  buildExportFilename,
  type ExportFormat,
} from "../export-utils";

describe("export-utils", () => {
  describe("formatFileSize", () => {
    it("formats bytes", () => {
      expect(formatFileSize(0)).toBe("0 B");
      expect(formatFileSize(512)).toBe("512 B");
    });

    it("formats kilobytes", () => {
      expect(formatFileSize(1024)).toBe("1.0 KB");
      expect(formatFileSize(1536)).toBe("1.5 KB");
    });

    it("formats megabytes", () => {
      expect(formatFileSize(1024 * 1024)).toBe("1.0 MB");
      expect(formatFileSize(2.5 * 1024 * 1024)).toBe("2.5 MB");
    });
  });

  describe("getExportExtension", () => {
    it("returns .csv for csv format", () => {
      expect(getExportExtension("csv")).toBe(".csv");
    });

    it("returns .xlsx for xlsx format", () => {
      expect(getExportExtension("xlsx")).toBe(".xlsx");
    });
  });

  describe("getExportMimeType", () => {
    it("returns text/csv for csv format", () => {
      expect(getExportMimeType("csv")).toBe("text/csv");
    });

    it("returns xlsx mime type for xlsx format", () => {
      expect(getExportMimeType("xlsx")).toBe(
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      );
    });
  });

  describe("buildExportFilename", () => {
    it("builds csv filename", () => {
      const name = buildExportFilename("audit_logs", "csv");
      expect(name).toMatch(/^audit_logs_\d{4}-\d{2}-\d{2}\.csv$/);
    });

    it("builds xlsx filename", () => {
      const name = buildExportFilename("tickets", "xlsx");
      expect(name).toMatch(/^tickets_\d{4}-\d{2}-\d{2}\.xlsx$/);
    });
  });

  describe("downloadBlob", () => {
    it("creates a download link and triggers it", () => {
      const blob = new Blob(["test"], { type: "text/csv" });
      const createObjectURLSpy = vi.spyOn(URL, "createObjectURL");
      const revokeObjectURLSpy = vi.spyOn(URL, "revokeObjectURL");

      downloadBlob(blob, "test.csv");

      expect(createObjectURLSpy).toHaveBeenCalledWith(blob);
      expect(revokeObjectURLSpy).toHaveBeenCalled();

      createObjectURLSpy.mockRestore();
      revokeObjectURLSpy.mockRestore();
    });
  });
});
