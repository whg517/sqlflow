import { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import {
  Plus,
  Trash2,
  Pencil,
  Play,
  Eye,
  Loader2,
  Search,
  RefreshCw,
  Globe,
  Lock,
  Copy,
  Check,
  ArrowRight,
  Layers3,
} from "lucide-react";
import { Card, CardContent } from "@/shared/components/ui/card";
import { Badge } from "@/shared/components/ui/badge";
import { Button } from "@/shared/components/ui/button";
import { Input } from "@/shared/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/shared/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/shared/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/shared/components/ui/table";
import {
  createTemplate,
  updateTemplate,
  deleteTemplate,
  listTemplates,
  renderTemplate,
  parseParamsJSON,
  type SQLTemplate,
  type CreateTemplateRequest,
  type RenderResult,
} from "@/features/query/api/sql-template";
import PageHeader from "@/shared/components/PageHeader";
import { useQueryStore } from "@/store/queryStore";

// --- Template Form Dialog (Create/Edit) ---

function TemplateFormDialog({
  template,
  onClose,
  onSaved,
}: {
  template: SQLTemplate | null; // null = create mode
  onClose: () => void;
  onSaved: () => void;
}) {
  const isEdit = template !== null;
  const [name, setName] = useState(template?.name ?? "");
  const [description, setDescription] = useState(template?.description ?? "");
  const [sqlContent, setSqlContent] = useState(template?.sql_content ?? "");
  const [dbType, setDbType] = useState(template?.db_type ?? "mysql");
  const [category, setCategory] = useState(template?.category ?? "general");
  const [isPublic, setIsPublic] = useState(template?.is_public ?? false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async () => {
    setError("");
    if (!name.trim()) {
      setError("模板名称不能为空");
      return;
    }
    if (!sqlContent.trim()) {
      setError("SQL 内容不能为空");
      return;
    }

    setLoading(true);
    try {
      const req: CreateTemplateRequest = {
        name: name.trim(),
        description: description.trim(),
        sql_content: sqlContent.trim(),
        db_type: dbType,
        category: category.trim() || "general",
        is_public: isPublic,
      };

      if (isEdit && template) {
        await updateTemplate(template.id, req);
      } else {
        await createTemplate(req);
      }
      onSaved();
      onClose();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "保存失败";
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open onOpenChange={() => onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? "编辑 SQL 模板" : "新建 SQL 模板"}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 pt-2">
          {error && (
            <div className="rounded-lg bg-red-500/10 px-3 py-2 text-sm text-red-400">
              {error}
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-[var(--text-primary)]">
                模板名称 *
              </label>
              <Input
                placeholder="例如：查询用户信息"
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={100}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-[var(--text-primary)]">
                数据库类型
              </label>
              <Select value={dbType} onValueChange={setDbType}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="mysql">MySQL</SelectItem>
                  <SelectItem value="postgresql">PostgreSQL</SelectItem>
                  <SelectItem value="mongodb">MongoDB</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-[var(--text-primary)]">
                分类
              </label>
              <Input
                placeholder="例如：general, query, dml"
                value={category}
                onChange={(e) => setCategory(e.target.value)}
              />
            </div>
            <div className="flex items-end pb-1">
              <label className="flex cursor-pointer items-center gap-2 text-sm text-[var(--text-primary)]">
                <input
                  type="checkbox"
                  checked={isPublic}
                  onChange={(e) => setIsPublic(e.target.checked)}
                  className="h-4 w-4 rounded border-[var(--border-default)]"
                />
                <Globe size={14} className="text-[var(--text-muted)]" />
                公开（其他用户可见）
              </label>
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-[var(--text-primary)]">
              描述
            </label>
            <Input
              placeholder="模板用途说明（可选）"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-[var(--text-primary)]">
              SQL 内容 *{" "}
              <span className="text-xs text-[var(--text-muted)]">
                使用 {"{{param_name}}"} 或 {"{{param_name:default}}"} 作为占位符
              </span>
            </label>
            <textarea
              className="min-h-[180px] w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 py-2 font-mono text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent-primary)]"
              placeholder={"SELECT * FROM users WHERE name = {{name}} AND status = {{status:active}}"}
              value={sqlContent}
              onChange={(e) => setSqlContent(e.target.value)}
              spellCheck={false}
            />
          </div>

          <div className="flex justify-end gap-3 pt-2">
            <Button variant="ghost" onClick={onClose}>
              取消
            </Button>
            <Button onClick={handleSubmit} disabled={loading}>
              {loading && <Loader2 size={14} className="mr-1.5 animate-spin" />}
              {isEdit ? "保存" : "创建"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// --- Render Preview Dialog ---

function RenderDialog({
  template,
  onClose,
}: {
  template: SQLTemplate;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const openTemplateAsTab = useQueryStore((state) => state.openTemplateAsTab);
  const params = parseParamsJSON(template.params_json);
  const [values, setValues] = useState<Record<string, string>>(() => {
    const m: Record<string, string> = {};
    for (const p of params) {
      if (p.default !== "") {
        m[p.name] = p.default;
      }
    }
    return m;
  });
  const [result, setResult] = useState<RenderResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [copiedSQL, setCopiedSQL] = useState(false);
  const [renderError, setRenderError] = useState("");
  // The shape of the body is the predicate, not the driver's name. A pair of
  // type names excluded SQLite, which is as SQL as the other two, and would
  // have excluded the next SQL driver as well. A document or DSL template
  // starts with "{", so it fails this test without anyone listing it.
  const canUseInQuery = /^(select|with)\b/i.test(template.sql_content.trim());

  const handleRender = async () => {
    setLoading(true);
    setRenderError("");
    try {
      const res = await renderTemplate(template.id, values);
      setResult(res);
    } catch (error) {
      setResult(null);
      setRenderError(
        error instanceof Error ? error.message : "模板渲染失败，请检查参数",
      );
    } finally {
      setLoading(false);
    }
  };

  const useInQuery = () => {
    if (!result || !canUseInQuery) return;
    openTemplateAsTab(
      template.name,
      result.rendered_sql,
      template.db_type,
      result.param_values,
      template.id,
    );
    onClose();
    navigate("/query");
    toast.success("模板已带入查询工作台");
  };

  const copySQL = async () => {
    if (!result) return;
    try {
      await navigator.clipboard.writeText(result.rendered_sql);
      setCopiedSQL(true);
      setTimeout(() => setCopiedSQL(false), 2000);
    } catch {
      // fallback
      const ta = document.createElement("textarea");
      ta.value = result.rendered_sql;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
      setCopiedSQL(true);
      setTimeout(() => setCopiedSQL(false), 2000);
    }
  };

  return (
    <Dialog open onOpenChange={() => onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Play size={16} className="text-[var(--accent-primary)]" />
            渲染预览 — {template.name}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 pt-2">
          {renderError && (
            <div className="rounded-lg bg-red-500/10 px-3 py-2 text-sm text-red-400">
              {renderError}
            </div>
          )}

          {params.length === 0 ? (
            <div className="rounded-lg bg-[var(--bg-base)] p-4 text-center text-sm text-[var(--text-muted)]">
              该模板没有占位符，无需填写参数
            </div>
          ) : (
            <div className="space-y-3">
              <h4 className="text-sm font-medium text-[var(--text-primary)]">
                参数值
              </h4>
              <div className="grid grid-cols-2 gap-3">
                {params.map((p) => (
                  <div key={p.name} className="space-y-1">
                    <label className="flex items-center gap-1.5 text-xs text-[var(--text-secondary)]">
                      <code className="rounded bg-[var(--bg-base)] px-1.5 py-0.5 font-mono">
                        {p.name}
                      </code>
                      {p.default && (
                        <span className="text-[var(--text-muted)]">
                          默认: {p.default}
                        </span>
                      )}
                    </label>
                    <Input
                      placeholder={p.default || "输入参数值"}
                      value={values[p.name] ?? ""}
                      onChange={(e) =>
                        setValues((prev) => ({
                          ...prev,
                          [p.name]: e.target.value,
                        }))
                      }
                    />
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="flex items-center gap-3">
            <Button onClick={handleRender} disabled={loading}>
              {loading ? (
                <Loader2 size={14} className="mr-1.5 animate-spin" />
              ) : (
                <Play size={14} className="mr-1.5" />
              )}
              渲染
            </Button>
            {!canUseInQuery && (
              <span className="text-xs text-[var(--text-muted)]">
                {template.db_type === "mongodb"
                  ? "MongoDB 模板当前支持渲染预览和复制"
                  : "变更类模板请复制 SQL 后通过工单提交"}
              </span>
            )}
          </div>

          {result && (
            <div className="space-y-3">
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <h4 className="text-sm font-medium text-[var(--text-primary)]">
                    渲染结果
                  </h4>
                  <div className="flex items-center gap-2">
                    <Button size="sm" variant="ghost" onClick={copySQL}>
                      {copiedSQL ? (
                        <Check size={12} className="mr-1" />
                      ) : (
                        <Copy size={12} className="mr-1" />
                      )}
                      {copiedSQL ? "已复制" : "复制"}
                    </Button>
                    {canUseInQuery && (
                      <Button size="sm" onClick={useInQuery}>
                        在查询中使用
                        <ArrowRight size={12} className="ml-1" />
                      </Button>
                    )}
                  </div>
                </div>
                <pre className="max-h-[200px] overflow-auto rounded-lg bg-[var(--bg-base)] p-3 text-sm font-mono text-[var(--text-primary)]">
                  {result.rendered_sql}
                </pre>
              </div>

              {result.param_values.length > 0 && (
                <div className="space-y-1.5">
                  <h4 className="text-sm font-medium text-[var(--text-primary)]">
                    参数值
                  </h4>
                  <div className="flex flex-wrap gap-2">
                    {result.param_values.map((v, i) => (
                      <Badge
                        key={i}
                        variant="outline"
                        className="border-[var(--border-default)] font-mono text-xs"
                      >
                        {template.db_type === "postgresql"
                          ? `$${i + 1}`
                          : `?`}
                        = {String(v)}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}

              {template.db_type === "mongodb" && (
                <div className="space-y-1.5">
                  <h4 className="text-sm font-medium text-[var(--text-primary)]">
                    MongoDB JSON
                  </h4>
                  <pre className="max-h-[120px] overflow-auto rounded-lg bg-[var(--bg-base)] p-3 text-sm font-mono text-[var(--text-primary)]">
                    {result.sql}
                  </pre>
                </div>
              )}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

// --- Main Page ---

export default function SQLTemplatePage() {
  const [templates, setTemplates] = useState<SQLTemplate[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);
  const [category, setCategory] = useState("");
  const [keyword, setKeyword] = useState("");
  const [loading, setLoading] = useState(true);
  const [editTemplate, setEditTemplate] = useState<SQLTemplate | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [renderTemplateState, setRenderTemplateState] = useState<SQLTemplate | null>(null);

  const fetchTemplates = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listTemplates(category, page, pageSize);
      setTemplates(Array.isArray(res.items) ? res.items : []);
      setTotal(res.total ?? 0);
    } catch (err) {
      console.error("Failed to fetch templates:", err);
    } finally {
      setLoading(false);
    }
  }, [category, page, pageSize]);

  useEffect(() => {
    fetchTemplates();
  }, [fetchTemplates]);

  const totalPages = Math.ceil(total / pageSize);
  const visibleTemplates = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    if (!query) return templates;
    return templates.filter((template) =>
      [template.name, template.description, template.category, template.db_type]
        .filter(Boolean)
        .some((value) => value.toLowerCase().includes(query)),
    );
  }, [keyword, templates]);

  const handleDelete = async (id: number) => {
    if (!confirm("确定要删除此模板吗？")) return;
    try {
      await deleteTemplate(id);
      fetchTemplates();
    } catch {
      // error handled by api client
    }
  };

  const handleSaved = () => {
    fetchTemplates();
  };

  const dbTypeLabel = (t: string) => {
    const map: Record<string, string> = {
      mysql: "MySQL",
      postgresql: "PostgreSQL",
      mongodb: "MongoDB",
    };
    return map[t] || t;
  };

  const dbTypeColor = (t: string) => {
    const map: Record<string, string> = {
      mysql: "border-blue-500/30 bg-blue-500/10 text-blue-400",
      postgresql: "border-emerald-500/30 bg-emerald-500/10 text-emerald-400",
      mongodb: "border-orange-500/30 bg-orange-500/10 text-orange-400",
    };
    return map[t] || "";
  };

  return (
    <div className="mx-auto max-w-[1360px] space-y-6 page-transition">
      <PageHeader
        eyebrow="Knowledge assets"
        title="SQL 模板"
        description="沉淀团队常用查询与变更语句，通过参数化模板减少重复编写，并让高频操作保持一致。"
        meta={
          <span className="rounded-full border border-[var(--border-default)] bg-[var(--bg-surface)] px-2.5 py-1 text-[10px] font-medium text-[var(--text-tertiary)] shadow-[var(--shadow-xs)]">
            {total} 个模板
          </span>
        }
        actions={
          <Button
            size="default"
            onClick={() => setShowCreate(true)}
            className="shadow-[0_8px_20px_var(--accent-shadow)]"
          >
            <Plus size={15} />
            新建模板
          </Button>
        }
      />

      <Card className="gap-0 overflow-hidden rounded-xl border-[var(--border-subtle)] bg-[var(--bg-surface)] py-0 shadow-[var(--shadow-sm)]">
        <div className="flex flex-col gap-3 border-b border-[var(--border-subtle)] px-5 py-4 xl:flex-row xl:items-center">
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <Input
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
              placeholder="搜索模板名称、说明或分类"
              startIcon={<Search />}
              className="max-w-[360px] bg-[var(--bg-elevated)]"
            />
            <Select
              value={category}
              onValueChange={(value) => {
                setCategory(value === "__all__" ? "" : value);
                setPage(1);
              }}
            >
              <SelectTrigger className="h-9 min-w-[132px] bg-[var(--bg-elevated)]">
                <SelectValue placeholder="全部分类" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">全部分类</SelectItem>
                <SelectItem value="general">通用</SelectItem>
                <SelectItem value="query">查询</SelectItem>
                <SelectItem value="dml">DML</SelectItem>
                <SelectItem value="ddl">DDL</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center justify-between gap-3 xl:justify-end">
            <span className="text-xs text-[var(--text-muted)]">
              {loading ? "正在同步模板…" : `当前显示 ${visibleTemplates.length} 条`}
            </span>
            <Button
              size="sm"
              variant="ghost"
              onClick={fetchTemplates}
              aria-label="刷新模板"
              className="h-8"
            >
              <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
              刷新
            </Button>
          </div>
        </div>

        <CardContent className="p-0">
          {loading ? (
            <div className="space-y-3 p-6">
              {[0, 1, 2, 3].map((item) => (
                <div
                  key={item}
                  className="grid h-14 animate-pulse grid-cols-[2fr_1fr_1fr] items-center gap-5 rounded-lg border border-[var(--border-subtle)] px-4"
                >
                  <div className="h-3 w-2/3 rounded-full bg-[var(--bg-elevated)]" />
                  <div className="h-3 w-1/2 rounded-full bg-[var(--bg-elevated)]" />
                  <div className="h-3 w-1/3 rounded-full bg-[var(--bg-elevated)]" />
                </div>
              ))}
            </div>
          ) : templates.length === 0 ? (
            <div className="relative overflow-hidden px-6 py-10">
              <div className="pointer-events-none absolute left-1/2 top-0 size-[420px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-[var(--accent-muted)] blur-3xl" />
              <div className="relative mx-auto max-w-3xl text-center">
                <div className="mx-auto flex size-16 items-center justify-center rounded-2xl border border-[var(--border-default)] bg-gradient-to-br from-[var(--accent-muted)] to-transparent text-[var(--accent-primary)] shadow-[var(--shadow-md)]">
                  <Layers3 size={28} strokeWidth={1.7} />
                </div>
                <h2 className="mt-5 text-lg font-semibold tracking-tight text-[var(--text-primary)]">
                  建立团队的第一个 SQL 模板
                </h2>
                <p className="mx-auto mt-2 max-w-lg text-sm leading-6 text-[var(--text-secondary)]">
                  把常用 SQL 变成可复用、可参数化的团队资产。模板不会直接执行，
                  你可以先渲染预览，再带入 SQL 工作台。
                </p>
                <Button
                  className="mt-5 shadow-[0_8px_20px_var(--accent-shadow)]"
                  onClick={() => setShowCreate(true)}
                >
                  <Plus size={15} />
                  创建第一个模板
                  <ArrowRight size={14} />
                </Button>

                <div className="mt-9 grid gap-3 text-left md:grid-cols-3">
                  {[
                    ["01", "编写 SQL", "使用占位符定义可复用参数"],
                    ["02", "渲染检查", "填入参数并预览最终语句"],
                    ["03", "团队复用", "公开模板供其他成员使用"],
                  ].map(([step, title, detail]) => (
                    <div
                      key={step}
                      className="rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-elevated)]/70 p-4"
                    >
                      <div className="text-[10px] font-semibold tracking-[0.18em] text-[var(--accent-primary)]">
                        STEP {step}
                      </div>
                      <div className="mt-2 text-sm font-medium text-[var(--text-primary)]">
                        {title}
                      </div>
                      <div className="mt-1 text-xs leading-5 text-[var(--text-tertiary)]">
                        {detail}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          ) : visibleTemplates.length === 0 ? (
            <div className="flex min-h-[320px] flex-col items-center justify-center px-6 text-center">
              <div className="flex size-12 items-center justify-center rounded-xl bg-[var(--bg-elevated)] text-[var(--text-muted)]">
                <Search size={20} />
              </div>
              <h2 className="mt-4 text-sm font-semibold text-[var(--text-primary)]">
                没有找到匹配的模板
              </h2>
              <p className="mt-1 text-xs text-[var(--text-tertiary)]">
                尝试修改关键词或切换模板分类。
              </p>
              <Button
                size="sm"
                variant="outline"
                className="mt-4"
                onClick={() => setKeyword("")}
              >
                清除搜索
              </Button>
            </div>
          ) : (
            <div className="table-responsive">
              <Table>
                <TableHeader>
                  <TableRow className="border-b border-[var(--border-subtle)] bg-[var(--bg-elevated)]/55 hover:bg-[var(--bg-elevated)]/55">
                    <TableHead className="pl-5 text-[var(--text-secondary)]">名称</TableHead>
                    <TableHead className="text-[var(--text-secondary)]">数据库</TableHead>
                    <TableHead className="text-[var(--text-secondary)]">分类</TableHead>
                    <TableHead className="text-[var(--text-secondary)]">参数</TableHead>
                    <TableHead className="text-[var(--text-secondary)]">可见性</TableHead>
                    <TableHead className="text-[var(--text-secondary)]">更新时间</TableHead>
                    <TableHead className="pr-5 text-right text-[var(--text-secondary)]">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visibleTemplates.map((tpl) => {
                    const params = parseParamsJSON(tpl.params_json);
                    return (
                      <TableRow
                        key={tpl.id}
                        className="border-[var(--border-subtle)] hover:bg-[var(--bg-elevated)]/55"
                      >
                        <TableCell className="pl-5">
                          <div className="font-medium text-[var(--text-primary)]">
                            {tpl.name}
                          </div>
                          {tpl.description && (
                            <div className="mt-1 max-w-sm truncate text-xs text-[var(--text-tertiary)]">
                              {tpl.description}
                            </div>
                          )}
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className={dbTypeColor(tpl.db_type)}>
                            {dbTypeLabel(tpl.db_type)}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className="border-[var(--border-default)]">
                            {tpl.category}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          {params.length > 0 ? (
                            <div className="flex flex-wrap gap-1">
                              {params.map((param) => (
                                <code
                                  key={param.name}
                                  className="rounded-md bg-[var(--bg-elevated)] px-1.5 py-0.5 font-mono text-xs text-[var(--text-tertiary)]"
                                >
                                  {param.name}
                                </code>
                              ))}
                            </div>
                          ) : (
                            <span className="text-xs text-[var(--text-muted)]">—</span>
                          )}
                        </TableCell>
                        <TableCell>
                          {tpl.is_public ? (
                            <span className="flex items-center gap-1.5 text-xs text-[var(--accent-primary)]">
                              <Globe size={12} /> 公开
                            </span>
                          ) : (
                            <span className="flex items-center gap-1.5 text-xs text-[var(--text-tertiary)]">
                              <Lock size={12} /> 私有
                            </span>
                          )}
                        </TableCell>
                        <TableCell className="text-xs text-[var(--text-secondary)]">
                          {new Date(tpl.updated_at).toLocaleString("zh-CN")}
                        </TableCell>
                        <TableCell className="pr-5 text-right">
                          <div className="flex items-center justify-end gap-0.5">
                            <Button
                              size="icon-sm"
                              variant="ghost"
                              title="预览 SQL"
                              onClick={() => setRenderTemplateState(tpl)}
                            >
                              <Eye size={14} />
                            </Button>
                            <Button
                              size="icon-sm"
                              variant="ghost"
                              title="渲染模板"
                              onClick={() => setRenderTemplateState(tpl)}
                            >
                              <Play size={14} />
                            </Button>
                            <Button
                              size="icon-sm"
                              variant="ghost"
                              title="编辑"
                              onClick={() => setEditTemplate(tpl)}
                            >
                              <Pencil size={14} />
                            </Button>
                            <Button
                              size="icon-sm"
                              variant="ghost"
                              className="text-red-400 hover:bg-red-500/10 hover:text-red-300"
                              title="删除"
                              onClick={() => handleDelete(tpl.id)}
                            >
                              <Trash2 size={14} />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {totalPages > 1 && (
        <div className="flex items-center justify-between rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] px-4 py-3 shadow-[var(--shadow-xs)]">
          <span className="text-xs text-[var(--text-muted)]">
            共 {total} 条记录
          </span>
          <div className="flex items-center gap-2">
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
        </div>
      )}

      {/* Dialogs */}
      {showCreate && (
        <TemplateFormDialog
          template={null}
          onClose={() => setShowCreate(false)}
          onSaved={handleSaved}
        />
      )}
      {editTemplate && (
        <TemplateFormDialog
          template={editTemplate}
          onClose={() => setEditTemplate(null)}
          onSaved={handleSaved}
        />
      )}
      {renderTemplateState && (
        <RenderDialog
          template={renderTemplateState}
          onClose={() => setRenderTemplateState(null)}
        />
      )}
    </div>
  );
}
