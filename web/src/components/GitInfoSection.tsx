import { useState, useEffect } from "react";
import {
  GitBranch,
  GitPullRequest,
  ExternalLink,
  Plus,
  Trash2,
  Loader2,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  listGitLinks,
  createGitLink,
  deleteGitLink,
  shortenHash,
  type GitLink,
  type GitLinkType,
} from "@/api/git";

interface GitInfoSectionProps {
  ticketId: number;
  readOnly?: boolean;
}

// --- Add Git Link Dialog ---

interface AddGitLinkDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  ticketId: number;
  onSuccess: () => void;
}

function AddGitLinkDialog({
  open,
  onOpenChange,
  ticketId,
  onSuccess,
}: AddGitLinkDialogProps) {
  const [loading, setLoading] = useState(false);
  const [linkType, setLinkType] = useState<GitLinkType>("commit");

  // Common fields
  const [commitHash, setCommitHash] = useState("");
  const [commitMsg, setCommitMsg] = useState("");
  const [authorName, setAuthorName] = useState("");
  const [repoURL, setRepoURL] = useState("");
  const [branch, setBranch] = useState("");

  // PR fields
  const [prNumber, setPrNumber] = useState("");
  const [prTitle, setPrTitle] = useState("");
  const [prURL, setPrURL] = useState("");

  function resetForm() {
    setLinkType("commit");
    setCommitHash("");
    setCommitMsg("");
    setAuthorName("");
    setRepoURL("");
    setBranch("");
    setPrNumber("");
    setPrTitle("");
    setPrURL("");
  }

  async function handleSubmit() {
    if (!commitHash && !prNumber) {
      toast.error("Commit Hash 和 PR 编号不能同时为空");
      return;
    }
    if (linkType === "pr" && !prNumber) {
      toast.error("请填写 PR 编号");
      return;
    }

    setLoading(true);
    try {
      await createGitLink({
        entity_type: "ticket",
        entity_id: ticketId,
        link_type: linkType,
        commit_hash: commitHash,
        commit_message: commitMsg,
        author_name: authorName,
        repo_url: repoURL,
        branch: branch,
        pr_number: prNumber ? parseInt(prNumber, 10) : undefined,
        pr_title: prTitle,
        pr_url: prURL,
      });
      toast.success("Git 关联已添加");
      resetForm();
      onOpenChange(false);
      onSuccess();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "添加失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) resetForm(); onOpenChange(v); }}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-sm">
            <GitBranch size={16} />
            添加 Git 关联
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-3 py-2">
          {/* Link Type */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-[var(--text-secondary)]">
              关联类型
            </label>
            <Select
              value={linkType}
              onValueChange={(v) => setLinkType(v as GitLinkType)}
            >
              <SelectTrigger className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="commit">Commit</SelectItem>
                <SelectItem value="pr">Pull Request</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Commit Hash (for both commit and PR) */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-[var(--text-secondary)]">
              Commit Hash {linkType === "commit" && <span className="text-red-400">*</span>}
            </label>
            <Input
              value={commitHash}
              onChange={(e) => setCommitHash(e.target.value)}
              placeholder="abc1234..."
              className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
            />
          </div>

          {/* Commit Message */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-[var(--text-secondary)]">
              Commit Message
            </label>
            <Input
              value={commitMsg}
              onChange={(e) => setCommitMsg(e.target.value)}
              placeholder="feat: add user table"
              className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
            />
          </div>

          {/* PR fields */}
          {linkType === "pr" && (
            <>
              <div className="space-y-1.5">
                <label className="text-xs font-medium text-[var(--text-secondary)]">
                  PR 编号 <span className="text-red-400">*</span>
                </label>
                <Input
                  value={prNumber}
                  onChange={(e) => setPrNumber(e.target.value)}
                  placeholder="42"
                  type="number"
                  className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-xs font-medium text-[var(--text-secondary)]">
                  PR 标题
                </label>
                <Input
                  value={prTitle}
                  onChange={(e) => setPrTitle(e.target.value)}
                  placeholder="Add user table"
                  className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-xs font-medium text-[var(--text-secondary)]">
                  PR 链接
                </label>
                <Input
                  value={prURL}
                  onChange={(e) => setPrURL(e.target.value)}
                  placeholder="https://github.com/org/repo/pull/42"
                  className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
                />
              </div>
            </>
          )}

          {/* Common fields continued */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-[var(--text-secondary)]">
              作者
            </label>
            <Input
              value={authorName}
              onChange={(e) => setAuthorName(e.target.value)}
              placeholder="Author Name"
              className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-[var(--text-secondary)]">
                仓库地址
              </label>
              <Input
                value={repoURL}
                onChange={(e) => setRepoURL(e.target.value)}
                placeholder="https://github.com/org/repo"
                className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-[var(--text-secondary)]">
                分支
              </label>
              <Input
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
                placeholder="main"
                className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
              />
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button
            size="sm"
            variant="ghost"
            className="h-8 text-xs text-[var(--text-secondary)]"
            onClick={() => {
              resetForm();
              onOpenChange(false);
            }}
          >
            取消
          </Button>
          <Button
            size="sm"
            className="h-8 gap-1.5 bg-[var(--accent-primary)] px-4 text-xs text-white hover:bg-[var(--accent-hover)]"
            onClick={handleSubmit}
            disabled={loading}
          >
            {loading ? (
              <Loader2 size={12} className="animate-spin" />
            ) : null}
            添加
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// --- GitLinkItem ---

interface GitLinkItemProps {
  link: GitLink;
  onDelete: (id: number) => void;
  deleteDisabled: boolean;
}

function GitLinkItem({ link, onDelete, deleteDisabled }: GitLinkItemProps) {
  const shortHash = shortenHash(link.commit_hash);

  // Build external URL for commit
  const commitURL =
    link.repo_url && link.commit_hash
      ? `${link.repo_url.replace(/\/$/, "")}/commit/${link.commit_hash}`
      : null;

  return (
    <div className="group flex items-start gap-3 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3 transition-colors hover:border-[var(--border-subtle)]">
      {/* Icon */}
      <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[var(--bg-base)]">
        {link.link_type === "pr" ? (
          <GitPullRequest size={14} className="text-violet-400" />
        ) : (
          <GitBranch size={14} className="text-emerald-400" />
        )}
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1 space-y-1">
        {/* PR title or commit message */}
        <div className="flex items-center gap-2">
          {link.link_type === "pr" && link.pr_number > 0 ? (
            <span className="text-xs font-medium text-[var(--text-primary)]">
              PR #{link.pr_number}
              {link.pr_title ? `: ${link.pr_title}` : ""}
            </span>
          ) : (
            <span className="truncate text-xs text-[var(--text-primary)]">
              {link.commit_message || shortHash}
            </span>
          )}
          <Badge
            variant="outline"
            className="h-4 px-1.5 text-[10px] font-normal border-[var(--border-default)] text-[var(--text-muted)]"
          >
            {link.link_type === "pr" ? "PR" : "Commit"}
          </Badge>
        </div>

        {/* Meta row */}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-[var(--text-muted)]">
          {shortHash && (
            <span className="font-mono">
              {commitURL ? (
                <a
                  href={commitURL}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-emerald-500 hover:underline"
                  onClick={(e) => e.stopPropagation()}
                >
                  {shortHash}
                  <ExternalLink size={10} />
                </a>
              ) : (
                shortHash
              )}
            </span>
          )}
          {link.author_name && <span>{link.author_name}</span>}
          {link.branch && <span className="flex items-center gap-1"><GitBranch size={10} />{link.branch}</span>}
        </div>

        {/* PR link */}
        {link.link_type === "pr" && link.pr_url && (
          <a
            href={link.pr_url}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-[11px] text-violet-400 hover:underline"
            onClick={(e) => e.stopPropagation()}
          >
            <ExternalLink size={10} />
            {link.pr_url}
          </a>
        )}
      </div>

      {/* Delete button */}
      {!deleteDisabled && (
        <button
          className="mt-0.5 rounded p-1 text-[var(--text-muted)] opacity-0 transition-all hover:bg-red-500/10 hover:text-red-400 group-hover:opacity-100"
          onClick={(e) => {
            e.stopPropagation();
            onDelete(link.id);
          }}
        >
          <Trash2 size={12} />
        </button>
      )}
    </div>
  );
}

// --- GitInfoSection (main export) ---

export default function GitInfoSection({
  ticketId,
  readOnly = false,
}: GitInfoSectionProps) {
  const [links, setLinks] = useState<GitLink[]>([]);
  const [loading, setLoading] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);

  async function fetchLinks() {
    if (!ticketId) return;
    setLoading(true);
    try {
      const res = await listGitLinks("ticket", ticketId);
      setLinks(res.data ?? []);
    } catch (err) {
      console.error("Failed to fetch git links:", err);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    fetchLinks();
  }, [ticketId]);

  async function handleDelete(id: number) {
    try {
      await deleteGitLink(id);
      toast.success("Git 关联已删除");
      fetchLinks();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败");
    }
  }

  return (
    <>
      <div>
        <div className="mb-2 flex items-center justify-between">
          <label className="text-xs font-medium text-[var(--text-secondary)]">
            Git 关联
          </label>
          {!readOnly && (
            <Button
              size="sm"
              variant="ghost"
              className="h-6 gap-1 px-2 text-[11px] text-[var(--text-muted)] hover:text-[var(--accent-primary)]"
              onClick={() => setDialogOpen(true)}
            >
              <Plus size={12} />
              添加
            </Button>
          )}
        </div>

        {loading ? (
          <div className="flex items-center justify-center py-4">
            <Loader2 size={16} className="animate-spin text-[var(--text-muted)]" />
          </div>
        ) : links.length === 0 ? (
          <p className="py-3 text-center text-xs text-[var(--text-muted)]">
            暂无 Git 关联
          </p>
        ) : (
          <div className="space-y-2">
            {links.map((link) => (
              <GitLinkItem
                key={link.id}
                link={link}
                onDelete={handleDelete}
                deleteDisabled={readOnly}
              />
            ))}
          </div>
        )}
      </div>

      <AddGitLinkDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        ticketId={ticketId}
        onSuccess={fetchLinks}
      />
    </>
  );
}
