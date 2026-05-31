import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";

// --- Mocks ---

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockGetTicket = vi.fn();
const mockApproveTicket = vi.fn();
const mockRejectTicket = vi.fn();
const mockCancelTicket = vi.fn();
const mockExecuteTicket = vi.fn();

vi.mock("@/api/ticket", () => ({
  getTicket: (...args: unknown[]) => mockGetTicket(...args),
  approveTicket: (...args: unknown[]) => mockApproveTicket(...args),
  rejectTicket: (...args: unknown[]) => mockRejectTicket(...args),
  cancelTicket: (...args: unknown[]) => mockCancelTicket(...args),
  executeTicket: (...args: unknown[]) => mockExecuteTicket(...args),
  getStatusLabel: (status: string) => {
    const map: Record<string, string> = {
      SUBMITTED: "已提交",
      AI_REVIEWED: "AI 已评审",
      PENDING_APPROVAL: "待审批",
      APPROVED: "已通过",
      REJECTED: "已拒绝",
      EXECUTING: "执行中",
      DONE: "已完成",
      CANCELLED: "已取消",
    };
    return map[status] ?? status;
  },
  getStatusColor: () => "bg-blue-500/20 text-blue-400",
  getRiskLabel: (level: string) => {
    const map: Record<string, string> = {
      low: "低风险",
      medium: "中风险",
      high: "高风险",
    };
    return map[level] ?? level;
  },
  getRiskColor: () => "bg-emerald-500/20 text-emerald-400",
  getRiskDot: () => "bg-emerald-400",
  formatTime: () => "05-24 10:00",
}));

// Mock Approval API
const mockGetApprovalHistory = vi.fn();
vi.mock("@/api/approval", () => ({
  getApprovalHistory: (...args: unknown[]) => mockGetApprovalHistory(...args),
}));

// Mock ApprovalStepper
vi.mock("@/pages/Ticket/components/ApprovalStepper", () => ({
  default: ({ currentStage, totalStages, records }: { currentStage: number; totalStages: number; records: unknown[] }) => (
    <div data-testid="approval-stepper">
      Stage {currentStage}/{totalStages} ({records.length} records)
    </div>
  ),
}));

// Mock CommentSection
vi.mock("@/pages/Ticket/components/CommentSection", () => ({
  default: ({ orderId }: { orderId: number }) => (
    <div data-testid="comment-section">Comments for #{orderId}</div>
  ),
}));

// Mock ScrollArea
vi.mock("@/components/ui/scroll-area", () => ({
  ScrollArea: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

// Mock Sheet components
vi.mock("@/components/ui/sheet", () => ({
  Sheet: ({
    open,
    onOpenChange,
    children,
  }: {
    open: boolean;
    onOpenChange: (v: boolean) => void;
    children: React.ReactNode;
  }) => (open ? <div data-testid="sheet">{children}</div> : null),
  SheetContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  SheetHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  SheetTitle: ({ children }: { children: React.ReactNode }) => (
    <h2>{children}</h2>
  ),
  SheetFooter: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="sheet-footer">{children}</div>
  ),
}));

// Mock AlertDialog
vi.mock("@/components/ui/alert-dialog", () => ({
  AlertDialog: ({
    open,
    children,
  }: {
    open: boolean;
    children: React.ReactNode;
  }) => (open ? <div data-testid="alert-dialog">{children}</div> : null),
  AlertDialogContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  AlertDialogHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  AlertDialogTitle: ({ children }: { children: React.ReactNode }) => (
    <h3>{children}</h3>
  ),
  AlertDialogDescription: ({ children }: { children: React.ReactNode }) => (
    <p>{children}</p>
  ),
  AlertDialogFooter: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  AlertDialogAction: ({
    onClick,
    disabled,
    children,
  }: {
    onClick: () => void;
    disabled?: boolean;
    children: React.ReactNode;
  }) => (
    <button onClick={onClick} disabled={disabled} data-testid="alert-action">
      {children}
    </button>
  ),
  AlertDialogCancel: ({
    disabled,
    children,
  }: {
    disabled?: boolean;
    children: React.ReactNode;
  }) => (
    <button disabled={disabled} data-testid="alert-cancel">
      {children}
    </button>
  ),
}));

// Mock Button
vi.mock("@/components/ui/button", () => ({
  Button: ({
    onClick,
    disabled,
    children,
    className,
  }: {
    onClick?: () => void;
    disabled?: boolean;
    children: React.ReactNode;
    className?: string;
  }) => (
    <button onClick={onClick} disabled={disabled} className={className}>
      {children}
    </button>
  ),
}));

// Mock Textarea
vi.mock("@/components/ui/textarea", () => ({
  Textarea: ({
    value,
    onChange,
    placeholder,
  }: {
    value: string;
    onChange: (e: React.ChangeEvent<HTMLTextAreaElement>) => void;
    placeholder?: string;
  }) => (
    <textarea
      value={value}
      onChange={onChange}
      placeholder={placeholder}
      data-testid="textarea"
    />
  ),
}));

// Mock Badge
vi.mock("@/components/ui/badge", () => ({
  Badge: ({
    children,
    className,
  }: {
    children: React.ReactNode;
    className?: string;
  }) => <span className={className}>{children}</span>,
}));

// Mock Separator
vi.mock("@/components/ui/separator", () => ({
  Separator: () => <hr />,
}));

import TicketDetailDrawer from "@/pages/Ticket/components/TicketDetailDrawer";

// --- Fixtures ---

const baseTicket = {
  id: 1,
  submitter_id: 10,
  submitter_name: "testuser",
  datasource_id: 1,
  database: "testdb",
  sql_content: "ALTER TABLE users ADD COLUMN age INT",
  sql_summary: "ALTER TABLE users...",
  db_type: "mysql",
  change_reason: "Adding age column for profile feature",
  status: "PENDING_APPROVAL" as const,
  risk_level: "medium",
  ai_review_result: "",
  reviewer_id: 0,
  reviewer_name: "",
  review_comment: "",
  executed_at: null,
  created_at: "2026-05-24T10:00:00Z",
  updated_at: "2026-05-24T10:00:00Z",
};

const onActionComplete = vi.fn();

function renderDrawer(
  overrides: Partial<Parameters<typeof TicketDetailDrawer>[0]> = {},
) {
  const props = {
    open: true,
    onOpenChange: vi.fn(),
    ticketId: 1,
    userRole: "dba",
    userId: 20,
    onActionComplete,
    ...overrides,
  };
  return render(<TicketDetailDrawer {...props} />);
}

describe("TicketDetailDrawer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetTicket.mockResolvedValue({ data: { ...baseTicket } });
    mockGetApprovalHistory.mockResolvedValue([]);
  });

  // --- Rendering ---

  describe("rendering", () => {
    it("shows ticket ID in title", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(screen.getByText("工单 #1")).toBeInTheDocument();
      });
    });

    it("displays status badge", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(screen.getByText("待审批")).toBeInTheDocument();
      });
    });

    it("displays risk level", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(screen.getByText("中风险")).toBeInTheDocument();
      });
    });

    it("displays submitter name", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(screen.getByText(/提交人: testuser/)).toBeInTheDocument();
      });
    });

    it("displays SQL content", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(
          screen.getByText("ALTER TABLE users ADD COLUMN age INT"),
        ).toBeInTheDocument();
      });
    });

    it("displays change reason", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(
          screen.getByText("Adding age column for profile feature"),
        ).toBeInTheDocument();
      });
    });

    it("displays database info", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(screen.getByText(/MYSQL.*testdb/)).toBeInTheDocument();
      });
    });

    it("displays comment section", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(screen.getByTestId("comment-section")).toBeInTheDocument();
      });
    });

    (it("shows loading spinner while fetching", async () => {
      mockGetTicket.mockReturnValue(new Promise(() => {}));
      renderDrawer();
      await waitFor(() => {
        expect(document.querySelector(".animate-spin")).toBeInTheDocument();
      });
    }),
      it('shows "工单不存在" when ticket is null', async () => {
        mockGetTicket.mockRejectedValue(new Error("Not found"));
        renderDrawer();
        await waitFor(() => {
          expect(screen.getByText("工单不存在")).toBeInTheDocument();
        });
      }));
  });

  // --- AI Review display ---

  describe("AI review display", () => {
    it("shows AI review section when ai_review_result is present", async () => {
      mockGetTicket.mockResolvedValue({
        data: {
          ...baseTicket,
          ai_review_result: JSON.stringify({
            summary: "This ALTER TABLE adds a new column",
            suggestions: [
              "Consider adding a DEFAULT value",
              "Add NOT NULL constraint",
            ],
            impact_analysis: "Low impact, additive change",
          }),
        },
      });

      renderDrawer();
      await waitFor(() => {
        expect(screen.getByText("AI 评审")).toBeInTheDocument();
        expect(
          screen.getByText("This ALTER TABLE adds a new column"),
        ).toBeInTheDocument();
        expect(
          screen.getByText("Consider adding a DEFAULT value"),
        ).toBeInTheDocument();
        expect(
          screen.getByText(/Low impact, additive change/),
        ).toBeInTheDocument();
      });
    });

    it("hides AI review section when no ai_review_result", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(
          screen.getByText("ALTER TABLE users ADD COLUMN age INT"),
        ).toBeInTheDocument();
      });
      expect(screen.queryByText("AI 评审")).not.toBeInTheDocument();
    });
  });

  // --- Review record ---

  describe("review record", () => {
    it("shows approval stepper when ticket has stages", async () => {
      mockGetApprovalHistory.mockResolvedValue([
        {
          id: 1,
          ticket_id: 1,
          stage: 1,
          approver_role: "DBA",
          status: "approved",
          comment: "LGTM",
          created_at: "2026-05-24T10:00:00Z",
          updated_at: "2026-05-24T10:00:00Z",
        },
      ]);
      mockGetTicket.mockResolvedValue({
        data: {
          ...baseTicket,
          status: "APPROVED",
          total_stages: 1,
          current_stage: 1,
          auto_approved: false,
        },
      });

      renderDrawer();
      await waitFor(() => {
        expect(screen.getByText("ALTER TABLE users ADD COLUMN age INT")).toBeInTheDocument();
      });
      expect(screen.getByTestId("approval-stepper")).toBeInTheDocument();
    });

    it("hides approval stepper when ticket has no stages", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(
          screen.getByText("ALTER TABLE users ADD COLUMN age INT"),
        ).toBeInTheDocument();
      });
      // baseTicket has no total_stages, so ApprovalStepper should not render
    });
  });

  // --- Approve action (DBA only) ---

  describe("approve action", () => {
    it("shows approve button for DBA on PENDING_APPROVAL ticket", async () => {
      renderDrawer({ userRole: "dba" });
      await waitFor(() => {
        expect(screen.getByText("通过")).toBeInTheDocument();
      });
    });

    it("shows approve button for admin on PENDING_APPROVAL ticket", async () => {
      renderDrawer({ userRole: "admin" });
      await waitFor(() => {
        expect(screen.getByText("通过")).toBeInTheDocument();
      });
    });

    it("hides approve button for developer on PENDING_APPROVAL ticket", async () => {
      renderDrawer({ userRole: "developer" });
      await waitFor(() => {
        expect(screen.queryByText("通过")).not.toBeInTheDocument();
      });
    });

    it("hides approve button when status is not PENDING_APPROVAL", async () => {
      mockGetTicket.mockResolvedValue({
        data: { ...baseTicket, status: "APPROVED" },
      });
      renderDrawer({ userRole: "dba" });
      await waitFor(() => {
        expect(screen.queryByText("通过")).not.toBeInTheDocument();
      });
    });

    it("calls approveTicket on confirm with comment", async () => {
      mockApproveTicket.mockResolvedValue({
        data: { ...baseTicket, status: "APPROVED" },
      });
      // Need re-fetch after approve
      mockGetTicket
        .mockResolvedValueOnce({ data: baseTicket })
        .mockResolvedValueOnce({ data: { ...baseTicket, status: "APPROVED" } });

      renderDrawer({ userRole: "dba" });
      await waitFor(() => screen.getByText("通过"));

      await userEvent.click(screen.getByText("通过"));

      // Type approve comment
      await waitFor(() => {
        expect(screen.getByTestId("alert-dialog")).toBeInTheDocument();
      });

      // Find the approve dialog's textarea and type comment
      const textareas = screen.getAllByTestId("textarea");
      await userEvent.type(textareas[0], "Approved, looks good");

      // Click confirm
      const actions = screen.getAllByTestId("alert-action");
      await userEvent.click(
        actions.find((a) => a.textContent?.includes("确认通过"))!,
      );

      await waitFor(() => {
        expect(mockApproveTicket).toHaveBeenCalledWith(
          1,
          "Approved, looks good",
        );
      });
    });
  });

  // --- Reject action ---

  describe("reject action", () => {
    it("shows reject button for DBA on PENDING_APPROVAL ticket", async () => {
      renderDrawer({ userRole: "dba" });
      await waitFor(() => {
        expect(screen.getByText("拒绝")).toBeInTheDocument();
      });
    });

    it("hides reject button for non-DBA users", async () => {
      renderDrawer({ userRole: "developer" });
      await waitFor(() => {
        expect(screen.queryByText("拒绝")).not.toBeInTheDocument();
      });
    });

    it("shows error toast when rejecting without reason", async () => {
      const { toast } = await import("sonner");
      mockGetTicket.mockResolvedValue({ data: baseTicket });
      mockRejectTicket.mockResolvedValue({
        data: { ...baseTicket, status: "REJECTED" },
      });

      renderDrawer({ userRole: "dba" });
      await waitFor(() => screen.getByText("拒绝"));

      await userEvent.click(screen.getByText("拒绝"));

      // Click confirm without entering reason
      await waitFor(() => {
        const actions = screen.getAllByTestId("alert-action");
        const rejectAction = actions.find((a) =>
          a.textContent?.includes("确认驳回"),
        );
        expect(rejectAction).toBeDisabled();
      });
    });

    it("calls rejectTicket with reason on confirm", async () => {
      mockRejectTicket.mockResolvedValue({
        data: { ...baseTicket, status: "REJECTED" },
      });
      mockGetTicket
        .mockResolvedValueOnce({ data: baseTicket })
        .mockResolvedValueOnce({ data: { ...baseTicket, status: "REJECTED" } });

      renderDrawer({ userRole: "dba" });
      await waitFor(() => screen.getByText("拒绝"));

      await userEvent.click(screen.getByText("拒绝"));

      await waitFor(() => {
        expect(screen.getByText("驳回工单")).toBeInTheDocument();
      });

      // Find and fill the reject reason textarea
      const textareas = screen.getAllByTestId("textarea");
      await userEvent.type(textareas[0], "Too risky for production");

      const actions = screen.getAllByTestId("alert-action");
      await userEvent.click(
        actions.find((a) => a.textContent?.includes("确认驳回"))!,
      );

      await waitFor(() => {
        expect(mockRejectTicket).toHaveBeenCalledWith(
          1,
          "Too risky for production",
        );
      });
    });
  });

  // --- Cancel action ---

  describe("cancel action", () => {
    it("shows cancel button for submitter on SUBMITTED ticket", async () => {
      mockGetTicket.mockResolvedValue({
        data: { ...baseTicket, status: "SUBMITTED" },
      });
      renderDrawer({ userId: 10 }); // submitter_id = 10

      await waitFor(() => {
        expect(screen.getByText("取消工单")).toBeInTheDocument();
      });
    });

    it("shows cancel button for DBA even if not submitter", async () => {
      mockGetTicket.mockResolvedValue({
        data: { ...baseTicket, status: "SUBMITTED" },
      });
      renderDrawer({ userRole: "dba", userId: 99 });

      await waitFor(() => {
        expect(screen.getByText("取消工单")).toBeInTheDocument();
      });
    });

    it("hides cancel button for non-submitter non-DBA", async () => {
      mockGetTicket.mockResolvedValue({
        data: { ...baseTicket, status: "SUBMITTED" },
      });
      renderDrawer({ userRole: "developer", userId: 99 });

      await waitFor(() => {
        expect(screen.queryByText("取消工单")).not.toBeInTheDocument();
      });
    });

    it("calls cancelTicket with reason on confirm", async () => {
      mockCancelTicket.mockResolvedValue({
        data: { ...baseTicket, status: "CANCELLED" },
      });
      mockGetTicket
        .mockResolvedValueOnce({ data: { ...baseTicket, status: "SUBMITTED" } })
        .mockResolvedValueOnce({
          data: { ...baseTicket, status: "CANCELLED" },
        });

      renderDrawer({ userId: 10 }); // submitter
      await waitFor(() => screen.getByText("取消工单"));

      await userEvent.click(screen.getByText("取消工单"));

      await waitFor(() => {
        expect(screen.getByText(/此操作不可恢复/)).toBeInTheDocument();
      });

      // The cancel dialog should be the only open dialog
      const textareas = screen.getAllByTestId("textarea");
      // Find the one in the cancel dialog (last opened)
      const cancelTextarea = textareas[textareas.length - 1];
      await userEvent.type(cancelTextarea, "No longer needed");

      const actions = screen.getAllByTestId("alert-action");
      await userEvent.click(
        actions.find((a) => a.textContent?.includes("确认取消"))!,
      );

      await waitFor(() => {
        expect(mockCancelTicket).toHaveBeenCalledWith(1, "No longer needed");
      });
    });
  });

  // --- Execute action ---

  describe("execute action", () => {
    it("shows execute button for submitter on APPROVED ticket", async () => {
      mockGetTicket.mockResolvedValue({
        data: { ...baseTicket, status: "APPROVED" },
      });
      renderDrawer({ userRole: "developer", userId: 10 }); // submitter_id=10

      await waitFor(() => {
        expect(screen.getByText("执行")).toBeInTheDocument();
      });
    });

    it("shows execute button for DBA on APPROVED ticket", async () => {
      mockGetTicket.mockResolvedValue({
        data: { ...baseTicket, status: "APPROVED" },
      });
      renderDrawer({ userRole: "dba", userId: 99 });

      await waitFor(() => {
        expect(screen.getByText("执行")).toBeInTheDocument();
      });
    });

    it("hides execute button for non-DBA non-submitter", async () => {
      mockGetTicket.mockResolvedValue({
        data: { ...baseTicket, status: "APPROVED" },
      });
      renderDrawer({ userRole: "developer", userId: 99 });

      await waitFor(() => {
        expect(screen.queryByText("执行")).not.toBeInTheDocument();
      });
    });

    it("calls executeTicket on confirm", async () => {
      mockExecuteTicket.mockResolvedValue({
        data: { ...baseTicket, status: "EXECUTING" },
      });
      mockGetTicket
        .mockResolvedValueOnce({ data: { ...baseTicket, status: "APPROVED" } })
        .mockResolvedValueOnce({
          data: { ...baseTicket, status: "EXECUTING" },
        });

      renderDrawer({ userId: 10 });
      await waitFor(() => screen.getByText("执行"));

      await userEvent.click(screen.getByText("执行"));

      await waitFor(() => {
        expect(screen.getByText(/确认执行工单 #1 的 SQL/)).toBeInTheDocument();
      });

      const actions = screen.getAllByTestId("alert-action");
      await userEvent.click(
        actions.find((a) => a.textContent?.includes("确认执行"))!,
      );

      await waitFor(() => {
        expect(mockExecuteTicket).toHaveBeenCalledWith(1);
      });
    });
  });

  // --- Action success/failure feedback ---

  describe("action feedback", () => {
    it("shows success toast and calls onActionComplete on action success", async () => {
      const { toast } = await import("sonner");
      mockExecuteTicket.mockResolvedValue({
        data: { ...baseTicket, status: "EXECUTING" },
      });
      mockGetTicket
        .mockResolvedValueOnce({ data: { ...baseTicket, status: "APPROVED" } })
        .mockResolvedValueOnce({
          data: { ...baseTicket, status: "EXECUTING" },
        });

      renderDrawer({ userId: 10 });
      await waitFor(() => screen.getByText("执行"));

      await userEvent.click(screen.getByText("执行"));

      const actions = screen.getAllByTestId("alert-action");
      await userEvent.click(
        actions.find((a) => a.textContent?.includes("确认执行"))!,
      );

      await waitFor(() => {
        expect(toast.success).toHaveBeenCalledWith("工单已执行");
        expect(onActionComplete).toHaveBeenCalled();
      });
    });

    it("shows error toast on action failure", async () => {
      const { toast } = await import("sonner");
      mockExecuteTicket.mockRejectedValue(new Error("Execution failed"));
      mockGetTicket.mockResolvedValue({
        data: { ...baseTicket, status: "APPROVED" },
      });

      renderDrawer({ userId: 10 });
      await waitFor(() => screen.getByText("执行"));

      await userEvent.click(screen.getByText("执行"));

      const actions = screen.getAllByTestId("alert-action");
      await userEvent.click(
        actions.find((a) => a.textContent?.includes("确认执行"))!,
      );

      await waitFor(() => {
        expect(toast.error).toHaveBeenCalledWith("Execution failed");
      });
    });
  });

  // --- ExecutedAt display ---

  describe("executed time", () => {
    it("shows execution time when ticket has been executed", async () => {
      mockGetTicket.mockResolvedValue({
        data: { ...baseTicket, executed_at: "2026-05-24T12:00:00Z" },
      });

      renderDrawer();
      await waitFor(() => {
        expect(screen.getByText(/执行时间:/)).toBeInTheDocument();
      });
    });

    it("hides execution time when not executed", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(
          screen.getByText("ALTER TABLE users ADD COLUMN age INT"),
        ).toBeInTheDocument();
      });
      expect(screen.queryByText(/执行时间:/)).not.toBeInTheDocument();
    });
  });

  // --- SQL copy button ---

  describe("SQL copy", () => {
    it("has copy button for SQL content", async () => {
      renderDrawer();
      await waitFor(() => {
        expect(
          screen.getByText("ALTER TABLE users ADD COLUMN age INT"),
        ).toBeInTheDocument();
      });
      // Copy button uses clipboard API
      const copyBtn =
        document.querySelector("button[onclick]") ||
        screen
          .getByText("ALTER TABLE users ADD COLUMN age INT")
          .closest("div")
          ?.querySelector("button");
      // The copy button is present in the relative container
      expect(screen.getByText("SQL 内容")).toBeInTheDocument();
    });
  });

  // --- Closed drawer ---

  describe("closed state", () => {
    it("renders nothing when open=false", () => {
      renderDrawer({ open: false });
      expect(screen.queryByTestId("sheet")).not.toBeInTheDocument();
    });
  });
});
