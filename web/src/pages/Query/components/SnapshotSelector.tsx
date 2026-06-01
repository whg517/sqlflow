import { useState, useEffect, useCallback } from "react";
import { GitCompare, Loader2, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  fetchHistory,
  type QueryHistoryItem,
} from "@/api/query";
import {
  createSnapshot,
  compareSnapshots,
  type CompareResult,
} from "@/api/snapshot";
import DiffView from "./DiffView";

interface SnapshotSelectorProps {
  open: boolean;
  onClose: () => void;
}

export default function SnapshotSelector({ open, onClose }: SnapshotSelectorProps) {
  const [history, setHistory] = useState<QueryHistoryItem[]>([]);
  const [leftId, setLeftId] = useState<number | "">("");
  const [rightId, setRightId] = useState<number | "">("");
  const [comparing, setComparing] = useState(false);
  const [diffResult, setDiffResult] = useState<CompareResult | null>(null);

  const loadHistory = useCallback(async () => {
    try {
      const res = await fetchHistory(1, 50);
      setHistory(res.data ?? []);
    } catch {
      toast.error("获取查询历史失败");
    }
  }, []);

  useEffect(() => {
    if (open) loadHistory();
  }, [open, loadHistory]);

  // Reset state when dialog closes
  useEffect(() => {
    if (!open) {
      setLeftId("");
      setRightId("");
      setDiffResult(null);
    }
  }, [open]);

  async function handleCompare() {
    if (!leftId || !rightId) {
      toast.error("请选择两条查询记录");
      return;
    }
    if (leftId === rightId) {
      toast.error("请选择不同的查询记录");
      return;
    }

    setComparing(true);
    setDiffResult(null);
    try {
      // Create snapshots for both histories
      const [snapLeft, snapRight] = await Promise.all([
        createSnapshot({ query_history_id: leftId }),
        createSnapshot({ query_history_id: rightId }),
      ]);

      // Compare
      const res = await compareSnapshots({
        left_snapshot_id: snapLeft.data.id,
        right_snapshot_id: snapRight.data.id,
      });

      setDiffResult(res.data);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "对比失败");
    } finally {
      setComparing(false);
    }
  }

  function formatHistoryLabel(item: QueryHistoryItem) {
    const time = new Date(item.created_at);
    const mm = String(time.getMonth() + 1).padStart(2, "0");
    const dd = String(time.getDate()).padStart(2, "0");
    const hh = String(time.getHours()).padStart(2, "0");
    const mi = String(time.getMinutes()).padStart(2, "0");
    const summary = item.sql_summary || item.sql_content.slice(0, 40);
    return `#${item.id} ${mm}-${dd} ${hh}:${mi} — ${summary} (${item.result_rows}行)`;
  }

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-[2px]">
      <div className="flex flex-col w-[90vw] max-w-[1100px] h-[80vh] rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-xl">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border-default)]">
          <div className="flex items-center gap-2">
            <GitCompare size={16} className="text-[var(--accent-primary)]" />
            <h2 className="text-sm font-semibold text-[var(--text-primary)]">查询结果对比</h2>
          </div>
          <Button variant="ghost" size="icon-sm" onClick={onClose}>
            <X size={16} className="text-[var(--text-muted)]" />
          </Button>
        </div>

        {/* Selector area */}
        {!diffResult && (
          <div className="px-5 py-4 space-y-4">
            <p className="text-xs text-[var(--text-secondary)]">
              选择两次查询记录，系统将自动保存快照并对比差异。仅支持列结构相同的结果。
            </p>
            <div className="flex items-end gap-4">
              <div className="flex-1 space-y-1.5">
                <label className="text-xs text-[var(--text-secondary)]">基准（左）</label>
                <Select
                  value={String(leftId)}
                  onValueChange={(v) => setLeftId(Number(v))}
                >
                  <SelectTrigger className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
                    <SelectValue placeholder="选择查询记录..." />
                  </SelectTrigger>
                  <SelectContent>
                    <ScrollArea className="max-h-[200px]">
                      {history.map((item) => (
                        <SelectItem
                          key={item.id}
                          value={String(item.id)}
                          disabled={item.id === rightId}
                          className="text-xs"
                        >
                          {formatHistoryLabel(item)}
                        </SelectItem>
                      ))}
                    </ScrollArea>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex-1 space-y-1.5">
                <label className="text-xs text-[var(--text-secondary)]">对比（右）</label>
                <Select
                  value={String(rightId)}
                  onValueChange={(v) => setRightId(Number(v))}
                >
                  <SelectTrigger className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
                    <SelectValue placeholder="选择查询记录..." />
                  </SelectTrigger>
                  <SelectContent>
                    <ScrollArea className="max-h-[200px]">
                      {history.map((item) => (
                        <SelectItem
                          key={item.id}
                          value={String(item.id)}
                          disabled={item.id === leftId}
                          className="text-xs"
                        >
                          {formatHistoryLabel(item)}
                        </SelectItem>
                      ))}
                    </ScrollArea>
                  </SelectContent>
                </Select>
              </div>
              <Button
                className="h-8 gap-1.5 bg-[var(--accent-primary)] px-4 text-xs text-white hover:bg-[var(--accent-hover)]"
                onClick={handleCompare}
                disabled={!leftId || !rightId || comparing}
              >
                {comparing ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <GitCompare size={14} />
                )}
                {comparing ? "对比中..." : "开始对比"}
              </Button>
            </div>
          </div>
        )}

        {/* Diff result */}
        {diffResult && (
          <>
            <div className="px-5 py-2 flex items-center gap-2">
              <Button
                variant="ghost"
                size="sm"
                className="h-6 text-xs text-[var(--text-secondary)]"
                onClick={() => setDiffResult(null)}
              >
                ← 返回选择
              </Button>
            </div>
            <div className="flex-1 overflow-hidden">
              <DiffView result={diffResult} />
            </div>
          </>
        )}

        {/* Empty state */}
        {!diffResult && history.length === 0 && (
          <div className="flex-1 flex items-center justify-center">
            <p className="text-sm text-[var(--text-muted)]">暂无查询历史记录</p>
          </div>
        )}
      </div>
    </div>
  );
}
