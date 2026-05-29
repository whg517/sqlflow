"use no memo";

import { useState } from "react";
import { Share2, Loader2, Lock, Clock, Copy, Check } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { createShare, type SharedResultResponse } from "@/api/share";

interface ShareButtonProps {
  columns: string[];
  rows: Record<string, unknown>[];
  sqlSummary?: string;
  datasourceName?: string;
}

const EXPIRY_OPTIONS = [
  { value: "1", label: "1 小时" },
  { value: "6", label: "6 小时" },
  { value: "24", label: "24 小时" },
  { value: "48", label: "48 小时" },
  { value: "168", label: "7 天" },
];

export default function ShareButton({
  columns,
  rows,
  sqlSummary,
  datasourceName,
}: ShareButtonProps) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [password, setPassword] = useState("");
  const [enablePassword, setEnablePassword] = useState(false);
  const [expiresInHours, setExpiresInHours] = useState("24");
  const [createdShare, setCreatedShare] = useState<SharedResultResponse | null>(null);
  const [copied, setCopied] = useState(false);

  const handleShare = async () => {
    setLoading(true);
    try {
      const result = await createShare({
        columns,
        rows,
        expires_in_hours: parseInt(expiresInHours),
        password: enablePassword ? password : undefined,
        sql_summary: sqlSummary,
        datasource_name: datasourceName,
      });
      setCreatedShare(result);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "创建共享链接失败");
    } finally {
      setLoading(false);
    }
  };

  const copyLink = () => {
    if (!createdShare) return;
    const link = `${window.location.origin}/s/${createdShare.token}`;
    navigator.clipboard.writeText(link).then(() => {
      setCopied(true);
      toast.success("链接已复制到剪贴板");
      setTimeout(() => setCopied(false), 2000);
    });
  };

  const handleClose = () => {
    setOpen(false);
    setCreatedShare(null);
    setPassword("");
    setEnablePassword(false);
    setExpiresInHours("24");
    setCopied(false);
  };

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        className="h-7 gap-1 text-xs"
        onClick={() => setOpen(true)}
        disabled={rows.length === 0}
      >
        <Share2 size={12} />
        共享
      </Button>

      <Dialog open={open} onOpenChange={(v) => v || handleClose()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Share2 size={16} />
              共享查询结果
            </DialogTitle>
          </DialogHeader>

          {!createdShare ? (
            <>
              <div className="flex flex-col gap-4 py-2">
                {/* Expiry */}
                <div className="flex flex-col gap-1.5">
                  <Label className="text-xs flex items-center gap-1">
                    <Clock size={12} />
                    过期时间
                  </Label>
                  <Select value={expiresInHours} onValueChange={setExpiresInHours}>
                    <SelectTrigger className="h-8 text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {EXPIRY_OPTIONS.map((opt) => (
                        <SelectItem key={opt.value} value={opt.value}>
                          {opt.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {/* Password */}
                <div className="flex flex-col gap-1.5">
                  <div className="flex items-center justify-between">
                    <Label className="text-xs flex items-center gap-1">
                      <Lock size={12} />
                      密码保护
                    </Label>
                    <Switch
                      checked={enablePassword}
                      onCheckedChange={setEnablePassword}
                      className="scale-75"
                    />
                  </div>
                  {enablePassword && (
                    <Input
                      type="password"
                      placeholder="设置访问密码"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      className="h-8 text-xs"
                    />
                  )}
                </div>

                {/* Info */}
                <div className="rounded-md bg-[var(--bg-elevated)] border border-[var(--border-default)] px-3 py-2 text-xs text-[var(--text-secondary)]">
                  共享 {rows.length} 行 × {columns.length} 列数据。共享链接创建后将强制应用脱敏规则。
                </div>
              </div>

              <DialogFooter>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleClose}
                  className="text-xs"
                >
                  取消
                </Button>
                <Button
                  size="sm"
                  onClick={handleShare}
                  disabled={loading}
                  className="text-xs gap-1"
                >
                  {loading ? (
                    <Loader2 size={12} className="animate-spin" />
                  ) : (
                    <Share2 size={12} />
                  )}
                  创建共享链接
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <div className="flex flex-col gap-4 py-2">
                {/* Success */}
                <div className="rounded-md bg-emerald-500/10 border border-emerald-500/20 px-3 py-3 text-sm text-emerald-400">
                  ✓ 共享链接已创建
                </div>

                {/* Link */}
                <div className="flex flex-col gap-1.5">
                  <Label className="text-xs">共享链接</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      readOnly
                      value={`${window.location.origin}/s/${createdShare.token}`}
                      className="h-8 text-xs font-mono"
                    />
                    <Button
                      variant="secondary"
                      size="sm"
                      className="h-8 px-2 text-xs shrink-0"
                      onClick={copyLink}
                    >
                      {copied ? <Check size={12} /> : <Copy size={12} />}
                    </Button>
                  </div>
                </div>

                {/* Details */}
                <div className="text-xs text-[var(--text-secondary)] space-y-1">
                  <div>数据：{createdShare.row_count} 行</div>
                  <div>过期：{new Date(createdShare.expires_at).toLocaleString()}</div>
                  {createdShare.token && (
                    <div className="font-mono">Token: {createdShare.token.slice(0, 8)}...</div>
                  )}
                </div>
              </div>

              <DialogFooter>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleClose}
                  className="text-xs"
                >
                  关闭
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
