"use no memo";

import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import {
  Loader2,
  Lock,
  Clock,
  Database,
  ShieldCheck,
  AlertTriangle,
} from "lucide-react";
import { toast } from "sonner";
import {
  getSharedResult,
  verifySharePassword,
  type SharedResultPublic,
} from "@/api/share";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@/components/ui/table";

export default function SharedResultPage() {
  const { token } = useParams<{ token: string }>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<SharedResultPublic | null>(null);
  const [passwordVerified, setPasswordVerified] = useState(false);
  const [password, setPassword] = useState("");
  const [passwordLoading, setPasswordLoading] = useState(false);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    const fetch = async () => {
      setLoading(true);
      setError(null);
      try {
        const result = await getSharedResult(token!);
        if (!cancelled) setData(result);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : "获取共享数据失败");
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    fetch();
    return () => { cancelled = true; };
  }, [token, passwordVerified]);

  const handlePasswordSubmit = async () => {
    if (!password || !token) return;
    setPasswordLoading(true);
    try {
      await verifySharePassword(token, password);
      setPasswordVerified(true);
      toast.success("密码验证成功");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "密码错误");
    } finally {
      setPasswordLoading(false);
    }
  };

  // Loading state
  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg-primary)]">
        <div className="flex items-center gap-2 text-[var(--text-muted)]">
          <Loader2 size={20} className="animate-spin" />
          <span className="text-sm">加载共享数据...</span>
        </div>
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg-primary)]">
        <div className="flex flex-col items-center gap-4 max-w-md text-center">
          <div className="rounded-full bg-[var(--bg-elevated)] p-4">
            <AlertTriangle size={32} className="text-[var(--text-muted)]" />
          </div>
          <h1 className="text-lg font-medium text-[var(--text-primary)]">
            无法访问
          </h1>
          <p className="text-sm text-[var(--text-secondary)]">{error}</p>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => window.location.href = "/"}
            className="text-xs"
          >
            返回首页
          </Button>
        </div>
      </div>
    );
  }

  // Password required but not yet verified
  if (data?.has_password && !passwordVerified) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg-primary)]">
        <div className="flex flex-col items-center gap-6 max-w-sm w-full px-4">
          <div className="rounded-full bg-[var(--bg-elevated)] p-4">
            <Lock size={32} className="text-[var(--text-muted)]" />
          </div>
          <h1 className="text-lg font-medium text-[var(--text-primary)]">
            需要密码
          </h1>
          <p className="text-sm text-[var(--text-secondary)] text-center">
            此共享链接受密码保护，请输入密码以查看数据。
          </p>
          <div className="flex items-center gap-2 w-full">
            <Input
              type="password"
              placeholder="输入访问密码"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handlePasswordSubmit()}
              className="h-9 text-sm"
            />
            <Button
              size="sm"
              onClick={handlePasswordSubmit}
              disabled={passwordLoading || !password}
              className="shrink-0"
            >
              {passwordLoading ? (
                <Loader2 size={14} className="animate-spin" />
              ) : (
                "验证"
              )}
            </Button>
          </div>
        </div>
      </div>
    );
  }

  if (!data) return null;

  return (
    <div className="min-h-screen bg-[var(--bg-primary)]">
      {/* Header */}
      <div className="border-b border-[var(--border-default)] bg-[var(--bg-elevated)]">
        <div className="mx-auto max-w-7xl px-4 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex items-center gap-2">
                <ShieldCheck size={18} className="text-emerald-400" />
                <h1 className="text-sm font-medium text-[var(--text-primary)]">
                  共享查询结果
                </h1>
              </div>
              <span className="text-xs text-[var(--text-muted)]">
                {data.row_count} 行 × {data.columns.length} 列
              </span>
            </div>
            <div className="flex items-center gap-3 text-xs text-[var(--text-muted)]">
              {data.datasource_name && (
                <span className="flex items-center gap-1">
                  <Database size={12} />
                  {data.datasource_name}
                </span>
              )}
              <span className="flex items-center gap-1">
                <Clock size={12} />
                过期：{new Date(data.expires_at).toLocaleString()}
              </span>
            </div>
          </div>
          {data.sql_summary && (
            <p className="mt-2 text-xs text-[var(--text-secondary)] font-mono truncate">
              {data.sql_summary}
            </p>
          )}
        </div>
      </div>

      {/* Table */}
      <div className="mx-auto max-w-7xl px-4 py-4">
        <div className="overflow-x-auto rounded-md border border-[var(--border-default)]">
          <Table>
            <TableHeader>
              <TableRow className="bg-[var(--bg-elevated)]">
                {data.columns.map((col) => (
                  <TableHead
                    key={col}
                    className="whitespace-nowrap px-3 py-2 text-xs font-medium text-[var(--text-secondary)]"
                  >
                    {col}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.rows.map((row, i) => (
                <TableRow key={i} className="border-t border-[var(--border-default)]">
                  {data.columns.map((col) => (
                    <TableCell
                      key={col}
                      className="whitespace-nowrap px-3 py-1.5 text-xs font-mono text-[var(--text-primary)]"
                    >
                      {row[col] === null || row[col] === undefined
                        ? "NULL"
                        : String(row[col])}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        {data.rows.length === 0 && (
          <div className="flex items-center justify-center py-12 text-sm text-[var(--text-muted)]">
            暂无数据
          </div>
        )}
      </div>
    </div>
  );
}
