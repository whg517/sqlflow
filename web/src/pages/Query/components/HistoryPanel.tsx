import { useState, useEffect, useCallback, useRef } from "react";
import { Clock, Trash2, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import {
  fetchHistory,
  deleteHistory,
  clearHistory,
  type QueryHistoryItem,
} from "@/api/query";
import { useQueryStore } from "@/store/queryStore";

export default function HistoryPanel() {
  const open = useQueryStore((s) => s.historyOpen);
  const setOpen = useQueryStore((s) => s.setHistoryOpen);
  const restoreHistoryAsTab = useQueryStore((s) => s.restoreHistoryAsTab);

  const [items, setItems] = useState<QueryHistoryItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);
  const [clearing, setClearing] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetchHistory(1, 200);
      setItems(res.data ?? []);
    } catch {
      toast.error("获取查询历史失败");
    } finally {
      setLoading(false);
    }
  }, []);

  // Load data when panel opens — use ref to track previous open state
  // and trigger fetch via microtask to avoid synchronous setState in effect
  const prevOpenRef = useRef(false);
  useEffect(() => {
    if (open && !prevOpenRef.current) {
      // Schedule fetch outside synchronous effect body
      const id = requestAnimationFrame(() => {
        load();
      });
      return () => cancelAnimationFrame(id);
    }
    prevOpenRef.current = open;
  }, [open, load]);

  // Close on click outside
  useEffect(() => {
    if (!open) return;
    function handleClickOutside(e: MouseEvent) {
      const target = e.target as HTMLElement;
      if (!target.closest("[data-history-panel]")) {
        setOpen(false);
      }
    }
    // Delay to avoid the opening click from immediately closing
    const timer = setTimeout(() => {
      document.addEventListener("mousedown", handleClickOutside);
    }, 0);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("mousedown", handleClickOutside);
    };
  }, [open, setOpen]);

  async function handleDelete(id: number, e: React.MouseEvent) {
    e.stopPropagation();
    try {
      await deleteHistory(id);
      setItems((prev) => prev.filter((i) => i.id !== id));
    } catch {
      toast.error("删除失败");
    }
  }

  async function handleClear() {
    setClearing(true);
    try {
      await clearHistory();
      setItems([]);
      toast.success("已清空查询历史");
    } catch {
      toast.error("清空失败");
    } finally {
      setClearing(false);
      setConfirmClear(false);
    }
  }

  if (!open) return null;

  return (
    <>
      <div
        data-history-panel
        className="absolute right-0 top-full z-20 mt-1 w-[380px] rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-lg"
      >
        <div className="flex items-center justify-between border-b border-[var(--border-default)] px-3 py-2">
          <span className="text-xs font-medium text-[var(--text-primary)]">
            查询历史
          </span>
          <div className="flex items-center gap-1">
            {items.length > 0 && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 gap-1 px-2 text-xs text-[var(--text-muted)] hover:text-red-400"
                onClick={() => setConfirmClear(true)}
              >
                清空历史
              </Button>
            )}
            <button
              className="rounded p-1 text-[var(--text-muted)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]"
              onClick={() => setOpen(false)}
            >
              <X size={14} />
            </button>
          </div>
        </div>
        <ScrollArea className="max-h-[400px]">
          {loading ? (
            <div className="flex items-center justify-center py-8 text-xs text-[var(--text-muted)]">
              加载中...
            </div>
          ) : items.length === 0 ? (
            <div className="flex items-center justify-center py-8 text-xs text-[var(--text-muted)]">
              暂无查询历史
            </div>
          ) : (
            <div className="py-1">
              {items.map((item) => (
                <div
                  key={item.id}
                  className="group flex cursor-pointer items-start gap-2 px-3 py-2 transition-colors hover:bg-[var(--bg-elevated)]/50"
                  onClick={() =>
                    restoreHistoryAsTab(
                      item.sql_content,
                      item.datasource_id,
                      item.database,
                    )
                  }
                >
                  <Clock
                    size={12}
                    className="mt-1 shrink-0 text-[var(--text-muted)]"
                  />
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-xs text-[var(--text-primary)]">
                      {item.sql_summary}
                    </p>
                    <div className="mt-0.5 flex items-center gap-2 text-[10px] text-[var(--text-muted)]">
                      <span>{item.execution_time}ms</span>
                      <span>{item.result_rows} 行</span>
                      <span>
                        {new Date(item.created_at).toLocaleString("zh-CN")}
                      </span>
                    </div>
                  </div>
                  <button
                    className="shrink-0 rounded p-1 opacity-0 transition-opacity hover:bg-[var(--bg-elevated)] hover:text-red-400 group-hover:opacity-100"
                    onClick={(e) => handleDelete(item.id, e)}
                    title="删除"
                  >
                    <Trash2 size={11} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </ScrollArea>
      </div>

      {/* Clear confirm dialog */}
      <AlertDialog open={confirmClear} onOpenChange={setConfirmClear}>
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              确认清空
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要清空所有查询历史吗？此操作不可恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleClear}
              disabled={clearing}
              className="bg-red-600 text-white hover:bg-red-700"
            >
              {clearing ? "清空中..." : "确认清空"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
