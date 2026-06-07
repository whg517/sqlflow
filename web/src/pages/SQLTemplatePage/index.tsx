import { useState, useEffect, useCallback } from "react";
import {
  Code2,
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
  createTemplate,
  updateTemplate,
  deleteTemplate,
  listTemplates,
  renderTemplate,
  parseParamsJSON,
  type SQLTemplate,
  type CreateTemplateRequest,
  type RenderResult,
} from "@/api/sql-template";

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
  const params = parseParamsJSON(template.params_json);
  const [values, setValues] = useState<Record<string, string>>(() => {
    const m: Record<string, string> = {};
    for (const p of params) {
      m[p.name] = p.default;
    }
    return m;
  });
  const [result, setResult] = useState<RenderResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [copiedSQL, setCopiedSQL] = useState(false);

  const handleRender = async () => {
    setLoading(true);
    try {
      const res = await renderTemplate(template.id, values);
      setResult(res);
    } catch {
      // error handled by api client
    } finally {
      setLoading(false);
    }
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
          </div>

          {result && (
            <div className="space-y-3">
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <h4 className="text-sm font-medium text-[var(--text-primary)]">
                    渲染结果
                  </h4>
                  <Button size="sm" variant="ghost" onClick={copySQL}>
                    {copiedSQL ? (
                      <Check size={12} className="mr-1" />
                    ) : (
                      <Copy size={12} className="mr-1" />
                    )}
                    {copiedSQL ? "已复制" : "复制"}
                  </Button>
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
    <div className="mx-auto max-w-[1200px] space-y-6 page-transition">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          SQL 模板库
        </h1>
        <Button onClick={() => setShowCreate(true)}>
          <Plus size={14} className="mr-1.5" />
          新建模板
        </Button>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-xs">
          <Search
            size={14}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]"
          />
          <Select value={category} onValueChange={(v) => { setCategory(v === "__all__" ? "" : v); setPage(1); }}>
            <SelectTrigger className="pl-8">
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
        <Button size="sm" variant="outline" onClick={fetchTemplates}>
          <RefreshCw size={14} className="mr-1.5" />
          刷新
        </Button>
      </div>

      {/* Template List */}
      <Card>
        <CardContent className="p-0">
          {loading ? (
            <div className="flex h-40 items-center justify-center">
              <Loader2 size={20} className="animate-spin text-[var(--text-muted)]" />
            </div>
          ) : templates.length === 0 ? (
            <div className="flex h-40 flex-col items-center justify-center gap-2 text-[var(--text-muted)]">
              <Code2 size={32} strokeWidth={1} />
              <span className="text-sm">暂无 SQL 模板</span>
              <span className="text-xs">创建一个模板来复用常用 SQL 片段</span>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow className="border-b border-[var(--border-default)]">
                  <TableHead className="text-[var(--text-secondary)]">名称</TableHead>
                  <TableHead className="text-[var(--text-secondary)]">数据库</TableHead>
                  <TableHead className="text-[var(--text-secondary)]">分类</TableHead>
                  <TableHead className="text-[var(--text-secondary)]">参数</TableHead>
                  <TableHead className="text-[var(--text-secondary)]">可见性</TableHead>
                  <TableHead className="text-[var(--text-secondary)]">更新时间</TableHead>
                  <TableHead className="text-right text-[var(--text-secondary)]">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {templates.map((tpl) => {
                  const params = parseParamsJSON(tpl.params_json);
                  return (
                    <TableRow key={tpl.id}>
                      <TableCell>
                        <div className="font-medium text-[var(--text-primary)]">
                          {tpl.name}
                        </div>
                        {tpl.description && (
                          <div className="mt-0.5 text-xs text-[var(--text-muted)]">
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
                            {params.map((p) => (
                              <code
                                key={p.name}
                                className="rounded bg-[var(--bg-base)] px-1.5 py-0.5 text-xs font-mono text-[var(--text-muted)]"
                              >
                                {p.name}
                              </code>
                            ))}
                          </div>
                        ) : (
                          <span className="text-xs text-[var(--text-muted)]">—</span>
                        )}
                      </TableCell>
                      <TableCell>
                        {tpl.is_public ? (
                          <span className="flex items-center gap-1 text-xs text-blue-400">
                            <Globe size={12} /> 公开
                          </span>
                        ) : (
                          <span className="flex items-center gap-1 text-xs text-[var(--text-muted)]">
                            <Lock size={12} /> 私有
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-xs text-[var(--text-secondary)]">
                        {new Date(tpl.updated_at).toLocaleString("zh-CN")}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            title="预览 SQL"
                            onClick={() => setRenderTemplateState(tpl)}
                          >
                            <Eye size={14} />
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            title="渲染模板"
                            onClick={() => setRenderTemplateState(tpl)}
                          >
                            <Play size={14} />
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            title="编辑"
                            onClick={() => setEditTemplate(tpl)}
                          >
                            <Pencil size={14} />
                          </Button>
                          <Button
                            size="sm"
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
          )}
        </CardContent>
      </Card>

      {/* Pagination */}
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
            {page} / {totalPages}（共 {total} 条）
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
