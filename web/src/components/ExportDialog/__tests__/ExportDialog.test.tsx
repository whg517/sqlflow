import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";

import { ExportDialog, type ExportTaskLike } from "../ExportDialog";
import type { ExportColumn } from "@/lib/export-utils";

// --- Mocks ---

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
}));

// --- Fixtures ---

const TEST_COLUMNS: ExportColumn[] = [
  { key: "id", label: "ID" },
  { key: "username", label: "用户" },
  { key: "action", label: "操作" },
  { key: "database", label: "数据库" },
  { key: "created_at", label: "时间" },
];

const mockSyncExport = vi.fn();
const mockAsyncExport = vi.fn();
const mockGetTask = vi.fn();
const mockDownloadTask = vi.fn();

function renderDialog(overrides = {}) {
  const props = {
    open: true,
    onOpenChange: vi.fn(),
    exportType: "audit" as const,
    columns: TEST_COLUMNS,
    filenamePrefix: "audit_logs",
    syncExport: mockSyncExport,
    asyncExport: mockAsyncExport,
    getTask: mockGetTask,
    downloadTask: mockDownloadTask,
    ...overrides,
  };
  return render(<ExportDialog {...props} />);
}

describe("ExportDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // --- Rendering ---

  describe("rendering", () => {
    it("renders dialog title and description", () => {
      renderDialog();
      expect(screen.getByText("导出数据")).toBeInTheDocument();
      expect(screen.getByText("选择导出格式和需要导出的字段")).toBeInTheDocument();
    });

    it("renders CSV and Excel format options", () => {
      renderDialog();
      expect(screen.getByText("CSV")).toBeInTheDocument();
      expect(screen.getByText("Excel")).toBeInTheDocument();
      expect(screen.getByText(".xlsx 格式，支持样式和筛选")).toBeInTheDocument();
    });

    it("renders all column checkboxes", () => {
      renderDialog();
      expect(screen.getByText("ID")).toBeInTheDocument();
      expect(screen.getByText("用户")).toBeInTheDocument();
      expect(screen.getByText("操作")).toBeInTheDocument();
      expect(screen.getByText("数据库")).toBeInTheDocument();
      expect(screen.getByText("时间")).toBeInTheDocument();
    });

    it("renders select all / clear buttons", () => {
      renderDialog();
      expect(screen.getByText("全选")).toBeInTheDocument();
      expect(screen.getByText("清空")).toBeInTheDocument();
    });

    it("renders export and cancel buttons", () => {
      renderDialog();
      expect(screen.getByText("导出 CSV")).toBeInTheDocument();
      expect(screen.getByText("取消")).toBeInTheDocument();
    });

    it("shows selected count", () => {
      renderDialog();
      expect(screen.getByText(/已选 5\/5/)).toBeInTheDocument();
    });

    it("defaults to CSV format", () => {
      renderDialog();
      const csvRadio = screen.getByDisplayValue("csv");
      expect(csvRadio).toBeChecked();
    });

    it("does not render when closed", () => {
      renderDialog({ open: false });
      expect(screen.queryByText("导出数据")).not.toBeInTheDocument();
    });
  });

  // --- Format selection ---

  describe("format selection", () => {
    it("switches to Excel format", async () => {
      renderDialog();
      const xlsxRadio = screen.getByDisplayValue("xlsx");
      await userEvent.click(xlsxRadio);
      expect(xlsxRadio).toBeChecked();
      expect(screen.getByText("导出 Excel")).toBeInTheDocument();
    });

    it("switches back to CSV format", async () => {
      renderDialog();
      const xlsxRadio = screen.getByDisplayValue("xlsx");
      await userEvent.click(xlsxRadio);
      const csvRadio = screen.getByDisplayValue("csv");
      await userEvent.click(csvRadio);
      expect(csvRadio).toBeChecked();
      expect(screen.getByText("导出 CSV")).toBeInTheDocument();
    });
  });

  // --- Column selection ---

  describe("column selection", () => {
    it("deselects a column on click", async () => {
      renderDialog();
      // Click on the label for "操作" column
      const label = screen.getByText("操作");
      await userEvent.click(label);
      expect(screen.getByText(/已选 4\/5/)).toBeInTheDocument();
    });

    it("clears all columns", async () => {
      renderDialog();
      await userEvent.click(screen.getByText("清空"));
      expect(screen.getByText(/已选 0\/5/)).toBeInTheDocument();
      expect(screen.getByText("请至少选择一个导出字段")).toBeInTheDocument();
    });

    it("selects all columns", async () => {
      renderDialog();
      // Clear first
      await userEvent.click(screen.getByText("清空"));
      expect(screen.getByText(/已选 0\/5/)).toBeInTheDocument();
      // Then select all
      await userEvent.click(screen.getByText("全选"));
      expect(screen.getByText(/已选 5\/5/)).toBeInTheDocument();
    });

    it("disables export when no columns selected", async () => {
      renderDialog();
      await userEvent.click(screen.getByText("清空"));
      const exportBtn = screen.getByText("导出 CSV").closest("button")!;
      expect(exportBtn).toBeDisabled();
    });
  });

  // --- Export flow ---

  describe("export flow", () => {
    it("calls syncExport with format and columns on export click", async () => {
      const blob = new Blob(["id,username\n1,admin"], { type: "text/csv" });
      mockSyncExport.mockResolvedValueOnce(blob);

      renderDialog();
      await userEvent.click(screen.getByText("导出 CSV"));

      await waitFor(() => {
        expect(mockSyncExport).toHaveBeenCalledWith("csv", TEST_COLUMNS.map((c) => c.key));
      });
    });

    it("shows info toast when blob is empty", async () => {
      const { toast } = await import("sonner");
      mockSyncExport.mockResolvedValueOnce(new Blob([], { type: "text/csv" }));

      renderDialog();
      await userEvent.click(screen.getByText("导出 CSV"));

      await waitFor(() => {
        expect(toast.info).toHaveBeenCalledWith("没有可导出的数据");
      });
    });

    it("shows error toast on export failure", async () => {
      const { toast } = await import("sonner");
      mockSyncExport.mockRejectedValueOnce(new Error("Server error"));

      renderDialog();
      await userEvent.click(screen.getByText("导出 CSV"));

      await waitFor(() => {
        expect(toast.error).toHaveBeenCalledWith("Server error");
      });
    });

    it("triggers async export when sync fails with size limit error", async () => {
      const { toast } = await import("sonner");
      mockSyncExport.mockRejectedValueOnce(new Error("超过10000行限制"));
      const asyncTask: ExportTaskLike = {
        id: 42,
        status: "processing",
        filename: "audit_logs.csv",
        total_rows: 0,
        file_bytes: 0,
      };
      mockAsyncExport.mockResolvedValueOnce(asyncTask);
      mockGetTask.mockResolvedValueOnce({
        ...asyncTask,
        status: "completed",
        total_rows: 15000,
        file_bytes: 1024000,
      } as ExportTaskLike);
      mockDownloadTask.mockResolvedValueOnce(new Blob(["data"], { type: "text/csv" }));

      renderDialog();
      await userEvent.click(screen.getByText("导出 CSV"));

      await waitFor(() => {
        expect(mockAsyncExport).toHaveBeenCalled();
      });

      await waitFor(() => {
        expect(toast.success).toHaveBeenCalledWith("导出完成！共 15000 条数据");
      }, { timeout: 5000 });
    });

    it("calls syncExport with xlsx format when Excel is selected", async () => {
      const blob = new Blob(["xlsx data"], {
        type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      });
      mockSyncExport.mockResolvedValueOnce(blob);

      renderDialog();
      await userEvent.click(screen.getByDisplayValue("xlsx"));
      await userEvent.click(screen.getByText("导出 Excel"));

      await waitFor(() => {
        expect(mockSyncExport).toHaveBeenCalledWith("xlsx", TEST_COLUMNS.map((c) => c.key));
      });
    });
  });
});
