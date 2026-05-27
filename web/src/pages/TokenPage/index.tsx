import { useState, useEffect, useCallback } from "react";
import {
  Key,
  Plus,
  Trash2,
  Copy,
  Check,
  AlertTriangle,
  ShieldCheck,
  Activity,
  Loader2,
  Info,
  RefreshCw,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  createToken,
  listMyTokens,
  listAllTokens,
  revokeMyToken,
  revokeAnyToken,
  getTokenStats,
  SCOPE_OPTIONS,
  formatScopes,
  formatDate,
  isExpired,
  daysUntilExpiry,
  type APIToken,
  type CreateTokenResponse,
  type TokenStats,
} from "@/api/token";
import { api } from "@/api/client";

// --- Stat Card ---

function StatCard({
  icon: Icon,
  label,
  value,
  color,
  bg,
}: {
  icon: React.ElementType;
  label: string;
  value: string | number;
  color: string;
  bg: string;
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 py-5">
        <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${bg}`}>
          <Icon size={20} className={color} />
        </div>
        <div>
          <div className="text-2xl font-bold text-[var(--text-primary)]">{value}</div>
          <div className="text-sm text-[var(--text-secondary)]">{label}</div>
        </div>
      </CardContent>
    </Card>
  );
}

// --- Create Token Dialog ---

function CreateTokenDialog({ onCreated }: { onCreated: (res: CreateTokenResponse) => void }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [expiresDays, setExpiresDays] = useState("365");
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const toggleScope = (scope: string) => {
    setSelectedScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]
    );
  };

  const handleSubmit = async () => {
    setError("");
    if (!name.trim()) {
      setError("Token 名称不能为空");
      return;
    }
    if (selectedScopes.length === 0) {
      setError("至少选择一个权限范围");
      return;
    }

    setLoading(true);
    try {
      const res = await createToken({
        name: name.trim(),
        description: description.trim() || undefined,
        scopes: selectedScopes,
        expires_days: parseInt(expiresDays, 10),
      });
      setOpen(false);
      onCreated(res);
      // Reset form
      setName("");
      setDescription("");
      setSelectedScopes([]);
      setExpiresDays("365");
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "创建失败";
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <Plus size={14} className="mr-1.5" />
          创建 Token
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>创建 API Token</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 pt-2">
          {error && (
            <div className="rounded-lg bg-red-500/10 px-3 py-2 text-sm text-red-400">
              {error}
            </div>
          )}

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-[var(--text-primary)]">Token 名称 *</label>
            <Input
              placeholder="例如：CI/CD 部署"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={50}
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-[var(--text-primary)]">描述</label>
            <Input
              placeholder="Token 用途说明（可选）"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-[var(--text-primary)]">有效期</label>
            <Select value={expiresDays} onValueChange={setExpiresDays}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="7">7 天</SelectItem>
                <SelectItem value="30">30 天</SelectItem>
                <SelectItem value="90">90 天</SelectItem>
                <SelectItem value="180">180 天</SelectItem>
                <SelectItem value="365">1 年</SelectItem>
                <SelectItem value="730">2 年</SelectItem>
                <SelectItem value="3650">10 年</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-[var(--text-primary)]">
              权限范围 *
            </label>
            <div className="space-y-2 rounded-lg border border-[var(--border-default)] p-3">
              {SCOPE_OPTIONS.map((scope) => (
                <label
                  key={scope.value}
                  className="flex cursor-pointer items-start gap-2.5 rounded-md p-1.5 transition-colors hover:bg-[var(--bg-base)]"
                >
                  <input
                    type="checkbox"
                    className="mt-0.5 h-4 w-4 shrink-0 rounded border-[var(--border-default)]"
                    checked={selectedScopes.includes(scope.value)}
                    onChange={() => toggleScope(scope.value)}
                  />
                  <div className="min-w-0">
                    <div className="text-sm font-medium text-[var(--text-primary)]">
                      {scope.label}
                    </div>
                    <div className="text-xs text-[var(--text-muted)]">{scope.desc}</div>
                  </div>
                </label>
              ))}
            </div>
          </div>

          <div className="flex justify-end gap-3 pt-2">
            <Button variant="ghost" onClick={() => setOpen(false)}>
              取消
            </Button>
            <Button onClick={handleSubmit} disabled={loading}>
              {loading && <Loader2 size={14} className="mr-1.5 animate-spin" />}
              创建
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// --- New Token Display ---

function NewTokenDisplay({
  response,
  onClose,
}: {
  response: CreateTokenResponse;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  if (dismissed) return null;

  const copyToken = async () => {
    try {
      await navigator.clipboard.writeText(response.token);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      // fallback
      console.error("Clipboard copy failed, trying fallback:", err);
      const ta = document.createElement("textarea");
      ta.value = response.token;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <Card className="border-amber-500/30 bg-amber-500/5">
      <CardContent className="space-y-3 p-4">
        <div className="flex items-center gap-2">
          <AlertTriangle size={16} className="shrink-0 text-amber-400" />
          <span className="text-sm font-medium text-amber-400">
            请立即复制 Token，关闭后将无法再次查看
          </span>
        </div>
        <div className="flex items-center gap-2">
          <code className="flex-1 overflow-x-auto rounded-lg bg-[var(--bg-base)] px-3 py-2 text-sm font-mono text-[var(--text-primary)]">
            {response.token}
          </code>
          <Button size="sm" variant="outline" onClick={copyToken}>
            {copied ? <Check size={14} /> : <Copy size={14} />}
          </Button>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-xs text-[var(--text-muted)]">
            {response.name} · 过期时间: {formatDate(response.expires_at)}
          </span>
          <Button size="sm" variant="ghost" onClick={() => { setDismissed(true); onClose(); }}>
            我已保存
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

// --- Token Table ---

function TokenTable({
  tokens,
  loading,
  onRevoke,
  onRefresh,
}: {
  tokens: APIToken[];
  loading: boolean;
  
  onRevoke: (id: number) => void;
  onRefresh: () => void;
}) {
  const [revokingId, setRevokingId] = useState<number | null>(null);

  const handleRevoke = async (id: number) => {
    if (!confirm("确定要撤销此 Token 吗？撤销后使用此 Token 的集成将立即失效。")) return;
    setRevokingId(id);
    try {
      await onRevoke(id);
      onRefresh();
    } catch (err) {
    } finally {
      setRevokingId(null);
    }
  };

  if (loading) {
    return (
      <div className="flex h-40 items-center justify-center">
        <Loader2 size={20} className="animate-spin text-[var(--text-muted)]" />
      </div>
    );
  }

  if (tokens.length === 0) {
    return (
      <div className="flex h-40 flex-col items-center justify-center gap-2 text-[var(--text-muted)]">
        <Key size={32} strokeWidth={1} />
        <span className="text-sm">暂无 API Token</span>
        <span className="text-xs">创建一个 Token 来接入外部系统集成</span>
      </div>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow className="border-b border-[var(--border-default)]">
          <TableHead className="text-[var(--text-secondary)]">名称</TableHead>
          <TableHead className="text-[var(--text-secondary)]">前缀</TableHead>
          <TableHead className="text-[var(--text-secondary)]">权限范围</TableHead>
          <TableHead className="text-[var(--text-secondary)]">过期时间</TableHead>
          <TableHead className="text-[var(--text-secondary)]">最后使用</TableHead>
          <TableHead className="text-right text-[var(--text-secondary)]">操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {tokens.map((token) => {
          const expired = !token.is_active || isExpired(token.expires_at);
          const daysLeft = daysUntilExpiry(token.expires_at);
          return (
            <TableRow key={token.id}>
              <TableCell>
                <div className="flex items-center gap-2">
                  <span className="font-medium text-[var(--text-primary)]">{token.name}</span>
                  {expired ? (
                    <Badge variant="outline" className="border-red-500/30 bg-red-500/10 text-red-400">
                      已失效
                    </Badge>
                  ) : daysLeft <= 30 ? (
                    <Badge variant="outline" className="border-amber-500/30 bg-amber-500/10 text-amber-400">
                      即将过期
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 text-emerald-400">
                      有效
                    </Badge>
                  )}
                </div>
                {token.description && (
                  <div className="mt-0.5 text-xs text-[var(--text-muted)]">{token.description}</div>
                )}
              </TableCell>
              <TableCell>
                <code className="rounded bg-[var(--bg-base)] px-1.5 py-0.5 text-xs font-mono text-[var(--text-muted)]">
                  {token.token_prefix}...
                </code>
              </TableCell>
              <TableCell>
                <span className="text-xs text-[var(--text-secondary)]">
                  {formatScopes(token.scopes)}
                </span>
              </TableCell>
              <TableCell className="text-xs text-[var(--text-secondary)]">
                {formatDate(token.expires_at)}
                {!expired && (
                  <span className="ml-1 text-[var(--text-muted)]">
                    ({daysLeft}天)
                  </span>
                )}
              </TableCell>
              <TableCell>
                <div className="text-xs text-[var(--text-secondary)]">
                  {token.last_used_at ? formatDate(token.last_used_at) : "从未使用"}
                </div>
                {token.use_count > 0 && (
                  <div className="text-xs text-[var(--text-muted)]">
                    {token.use_count} 次调用
                  </div>
                )}
              </TableCell>
              <TableCell className="text-right">
                {token.is_active && (
                  <Button
                    size="sm"
                    variant="ghost"
                    className="text-red-400 hover:bg-red-500/10 hover:text-red-300"
                    onClick={() => handleRevoke(token.id)}
                    disabled={revokingId === token.id}
                  >
                    {revokingId === token.id ? (
                      <Loader2 size={14} className="animate-spin" />
                    ) : (
                      <Trash2 size={14} />
                    )}
                  </Button>
                )}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}

// --- Admin All Tokens Tab ---

function AdminTokenTab() {
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);

  const fetchTokens = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listAllTokens(page, 20);
      setTokens(res.items);
      setTotal(res.total);
    } catch (err) {
      console.error("Failed to fetch admin tokens:", err);
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchTokens();
  }, [fetchTokens]);

  const totalPages = Math.ceil(total / 20);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <span className="text-sm text-[var(--text-secondary)]">
          共 {total} 个 Token
        </span>
        <Button size="sm" variant="outline" onClick={fetchTokens}>
          <RefreshCw size={14} className="mr-1.5" />
          刷新
        </Button>
      </div>
      <TokenTable
        tokens={tokens}
        loading={loading}
        onRevoke={revokeAnyToken}
        onRefresh={fetchTokens}
      />
      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            上一页
          </Button>
          <span className="text-xs text-[var(--text-muted)]">
            {page} / {totalPages}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            下一页
          </Button>
        </div>
      )}
    </div>
  );
}

// --- Main Page ---

export default function TokenPage() {
  const [isAdmin, setIsAdmin] = useState(false);
  const [activeTab, setActiveTab] = useState<"mine" | "all">("mine");
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [stats, setStats] = useState<TokenStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [newToken, setNewToken] = useState<CreateTokenResponse | null>(null);

  const fetchMyTokens = useCallback(async () => {
    setLoading(true);
    try {
      const [tokenList, tokenStats] = await Promise.all([
        listMyTokens(),
        getTokenStats(),
      ]);

      setTokens(tokenList);
      setStats(tokenStats);
    } catch (err) {
      console.error("Failed to fetch my tokens:", err);
    } finally {
      setLoading(false);
    }
  }, []);

  // Fetch current user role
  useEffect(() => {
    api
      .get<{ code: number; data: { role: string } }>('/auth/me')
      .then((res) => {
        setIsAdmin(res.data?.role === 'admin');
      })
      .catch((err) => { console.error("Failed to fetch user info:", err); });
  }, []);

  useEffect(() => {
    fetchMyTokens();
  }, [fetchMyTokens]);

  const handleCreated = (res: CreateTokenResponse) => {
    setNewToken(res);
    fetchMyTokens();
  };

  return (
    <div className="mx-auto max-w-[1200px] space-y-6 page-transition">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          API Token 管理
        </h1>
        <CreateTokenDialog onCreated={handleCreated} />
      </div>

      {/* New token alert */}
      {newToken && (
        <NewTokenDisplay
          response={newToken}
          onClose={() => setNewToken(null)}
        />
      )}

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-3 gap-5">
          <StatCard icon={Key} label="Token 总数" value={stats.total_tokens} color="text-blue-500" bg="bg-blue-500/10" />
          <StatCard icon={ShieldCheck} label="有效 Token" value={stats.active_tokens} color="text-emerald-500" bg="bg-emerald-500/10" />
          <StatCard icon={Activity} label="总调用次数" value={stats.total_usage} color="text-violet-500" bg="bg-violet-500/10" />
        </div>
      )}

      {/* Tabs */}
      {isAdmin && (
        <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-1">
          <div className="flex gap-1">
            <button
              onClick={() => setActiveTab("mine")}
              className={`flex-1 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                activeTab === "mine"
                  ? "bg-[var(--bg-elevated)] text-[var(--text-primary)] shadow-sm"
                  : "text-[var(--text-muted)] hover:text-[var(--text-secondary)]"
              }`}
            >
              我的 Token
            </button>
            <button
              onClick={() => setActiveTab("all")}
              className={`flex-1 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                activeTab === "all"
                  ? "bg-[var(--bg-elevated)] text-[var(--text-primary)] shadow-sm"
                  : "text-[var(--text-muted)] hover:text-[var(--text-secondary)]"
              }`}
            >
              全部 Token（管理）
            </button>
          </div>
        </div>
      )}

      {/* Content */}
      {activeTab === "all" && isAdmin ? (
        <AdminTokenTab />
      ) : (
        <Card>
          <CardContent className="p-4">
            <TokenTable
              tokens={tokens}
              loading={loading}
              onRevoke={revokeMyToken}
              onRefresh={fetchMyTokens}
            />
          </CardContent>
        </Card>
      )}

      {/* Usage Guide */}
      <Card>
        <CardContent className="space-y-4 p-4">
          <h3 className="flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
            <Info size={14} />
            使用说明
          </h3>
          <div className="space-y-2 text-xs text-[var(--text-secondary)]">
            <p>
              API Token 用于外部系统集成（如 CI/CD 流水线、监控脚本、第三方工具），可替代 JWT 进行 API 认证。
            </p>
            <p>
              <strong>认证方式：</strong>在 HTTP 请求头中使用 <code className="rounded bg-[var(--bg-base)] px-1.5 py-0.5 font-mono">Authorization: Bearer sqlflow_xxxx...</code>
            </p>
            <p>
              <strong>安全建议：</strong>
            </p>
            <ul className="ml-4 list-disc space-y-1">
              <li>Token 创建后仅显示一次，请立即保存到安全的密钥管理工具中</li>
              <li>为不同场景创建独立 Token，设置最小必要权限和最短有效期</li>
              <li>如发现 Token 泄露，请立即撤销并创建新 Token</li>
              <li>建议定期轮换 Token，避免使用长期有效的 Token</li>
            </ul>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
