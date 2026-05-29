"use no memo";

import { useEffect, useState } from "react";
import {
  Link2,
  Clock,
  Trash2,
  Loader2,
  ExternalLink,
  Copy,
  Check,
  Database,
  ShieldCheck,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  listMyShares,
  revokeShare,
  type SharedResultResponse,
} from "@/api/share";

export default function ShareListPanel() {
  const [shares, setShares] = useState<SharedResultResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [copiedId, setCopiedId] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    const fetch = async () => {
      setLoading(true);
      try {
        const list = await listMyShares();
        if (!cancelled) setShares(list);
      } catch {
        if (!cancelled) toast.error("获取共享列表失败");
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    fetch();
    return () => { cancelled = true; };
  }, []);

  const handleRevoke = async (id: number) => {
    try {
      await revokeShare(id);
      setShares((prev) => prev.map((s) => (s.id === id ? { ...s, revoked: true } : s)));
      toast.success("已撤销共享链接");
    } catch {
      toast.error("撤销失败");
    }
  };

  const copyLink = (token: string, id: number) => {
    const link = `${window.location.origin}/s/${token}`;
    navigator.clipboard.writeText(link).then(() => {
      setCopiedId(id);
      toast.success("链接已复制");
      setTimeout(() => setCopiedId(null), 2000);
    });
  };

  const isExpired = (expiresAt: string) => new Date(expiresAt) < new Date();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 size={20} className="animate-spin text-[var(--text-muted)]" />
        <span className="ml-2 text-sm text-[var(--text-muted)]">
          加载中...
        </span>
      </div>
    );
  }

  if (shares.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-[var(--text-muted)]">
        <Link2 size={24} className="mb-2 opacity-50" />
        <span className="text-sm">暂无共享链接</span>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      {shares.map((share) => {
        const expired = isExpired(share.expires_at);
        const statusBadge = share.revoked
          ? (
            <Badge variant="danger" className="text-[10px]">已撤销</Badge>
          )
          : expired
            ? (
              <Badge variant="warning" className="text-[10px]">已过期</Badge>
            )
            : (
              <Badge variant="secondary" className="text-[10px]">有效</Badge>
            );

        return (
          <div
            key={share.id}
            className="flex items-center gap-3 rounded-md border border-[var(--border-default)] px-3 py-2 hover:bg-[var(--bg-elevated)]/50 transition-colors"
          >
            {/* Info */}
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                {statusBadge}
                {share.datasource_name && (
                  <span className="text-xs text-[var(--text-secondary)] flex items-center gap-1">
                    <Database size={10} />
                    {share.datasource_name}
                  </span>
                )}
                <span className="text-xs text-[var(--text-secondary)] flex items-center gap-1">
                  <ShieldCheck size={10} />
                  {share.row_count} 行
                </span>
              </div>
              <div className="mt-1 flex items-center gap-3 text-[11px] text-[var(--text-muted)]">
                <span className="flex items-center gap-1">
                  <Clock size={10} />
                  过期：{new Date(share.expires_at).toLocaleString()}
                </span>
                <span className="font-mono truncate">
                  {share.token.slice(0, 12)}...
                </span>
              </div>
              {share.sql_summary && (
                <p className="mt-1 text-[11px] text-[var(--text-muted)] font-mono truncate">
                  {share.sql_summary}
                </p>
              )}
            </div>

            {/* Actions */}
            <div className="flex items-center gap-1 shrink-0">
              {!share.revoked && !expired && (
                <>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 px-1.5 text-xs"
                    onClick={() => copyLink(share.token, share.id)}
                  >
                    {copiedId === share.id ? <Check size={12} /> : <Copy size={12} />}
                  </Button>
                  <a
                    href={`/s/${share.token}`}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 px-1.5 text-xs"
                    >
                      <ExternalLink size={12} />
                    </Button>
                  </a>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 px-1.5 text-xs text-red-400 hover:text-red-300"
                    onClick={() => handleRevoke(share.id)}
                  >
                    <Trash2 size={12} />
                  </Button>
                </>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
