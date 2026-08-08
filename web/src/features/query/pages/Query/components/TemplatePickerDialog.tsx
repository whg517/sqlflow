import { useEffect, useMemo, useState } from "react";
import {
  fetchDatasourceTypes,
  type DatasourceType,
} from "@/shared/datasource/types";
import { datasourceLabel } from "@/shared/datasource/typePresentation";
import { Braces, Loader2, Search } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/shared/components/ui/badge";
import { Button } from "@/shared/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/shared/components/ui/dialog";
import { Input } from "@/shared/components/ui/input";
import { ScrollArea } from "@/shared/components/ui/scroll-area";
import {
  listTemplates,
  parseParamsJSON,
  renderTemplate,
  type SQLTemplate,
} from "@/features/query/api/sql-template";
import { useQueryStore } from "@/store/queryStore";

interface TemplatePickerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Whether a template can be opened in the query workbench.
//
// The server accepts a template for any registered data source type, but
// openTemplateAsTab only populates the tab's `sql` field. Document and DSL
// modes read their own fields (mongoFilter, esQueryBody), so a MongoDB or
// Elasticsearch template would open into an empty editor. Until the store
// routes a template payload by query form, those types stay preview-only.
// The condition is the query form, which is what the comment above always
// described — it just used to be spelled as three type names, so a fourth
// SQL-form driver would have been excluded for no reason anyone could see.
function isUsableQueryTemplate(
  template: SQLTemplate,
  types: DatasourceType[],
): boolean {
  const form = types.find((t) => t.type === template.db_type)?.query_form;
  return form === "sql" && /^(select|with)\b/i.test(template.sql_content.trim());
}

export default function TemplatePickerDialog({
  open,
  onOpenChange,
}: TemplatePickerDialogProps) {
  const openTemplateAsTab = useQueryStore((state) => state.openTemplateAsTab);
  const [templates, setTemplates] = useState<SQLTemplate[]>([]);
  const [selected, setSelected] = useState<SQLTemplate | null>(null);
  const [values, setValues] = useState<Record<string, string>>({});
  const [keyword, setKeyword] = useState("");
  const [loading, setLoading] = useState(false);
  const [rendering, setRendering] = useState(false);
  const [error, setError] = useState("");
  // Which types speak SQL is the drivers' answer, not a list kept here.
  const [types, setTypes] = useState<DatasourceType[]>([]);

  useEffect(() => {
    fetchDatasourceTypes().then(setTypes).catch(() => setTypes([]));
  }, []);

  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() => {
      setLoading(true);
      setError("");
      listTemplates("", 1, 100)
        .then((response) => {
          setTemplates(response.items ?? []);
        })
        .catch((requestError: unknown) => {
          setError(
            requestError instanceof Error
              ? requestError.message
              : "模板列表加载失败",
          );
        })
        .finally(() => setLoading(false));
    });
    return () => cancelAnimationFrame(frame);
  }, [open]);

  const filteredTemplates = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    if (!query) return templates;
    return templates.filter((template) =>
      [
        template.name,
        template.description,
        template.category,
        template.db_type,
      ]
        .filter(Boolean)
        .some((value) => value.toLowerCase().includes(query)),
    );
  }, [keyword, templates]);

  const params = selected ? parseParamsJSON(selected.params_json) : [];

  const selectTemplate = (template: SQLTemplate) => {
    if (!isUsableQueryTemplate(template, types)) return;
    const initialValues: Record<string, string> = {};
    for (const param of parseParamsJSON(template.params_json)) {
      if (param.default !== "") {
        initialValues[param.name] = param.default;
      }
    }
    setSelected(template);
    setValues(initialValues);
    setError("");
  };

  const closeDialog = () => {
    setSelected(null);
    setValues({});
    setKeyword("");
    setError("");
    onOpenChange(false);
  };

  const handleUseTemplate = async () => {
    if (!selected) return;
    setRendering(true);
    setError("");
    try {
      const result = await renderTemplate(selected.id, values);
      openTemplateAsTab(
        selected.name,
        result.rendered_sql,
        selected.db_type,
        result.param_values,
        selected.id,
      );
      closeDialog();
      toast.success("模板已生成新的查询标签");
    } catch (renderError: unknown) {
      setError(
        renderError instanceof Error ? renderError.message : "模板渲染失败",
      );
    } finally {
      setRendering(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => !nextOpen && closeDialog()}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Braces size={17} className="text-[var(--accent-primary)]" />
            使用 SQL 模板
          </DialogTitle>
        </DialogHeader>

        <div className="grid min-h-[430px] grid-cols-[minmax(0,1.1fr)_minmax(280px,0.9fr)] overflow-hidden rounded-lg border border-[var(--border-default)]">
          <div className="flex min-w-0 flex-col border-r border-[var(--border-default)]">
            <div className="border-b border-[var(--border-default)] p-3">
              <div className="relative">
                <Search
                  size={14}
                  className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-muted)]"
                />
                <Input
                  aria-label="搜索 SQL 模板"
                  value={keyword}
                  onChange={(event) => setKeyword(event.target.value)}
                  placeholder="搜索模板名称、说明或分类"
                  className="pl-8"
                />
              </div>
            </div>

            <ScrollArea className="h-[370px]">
              {loading ? (
                <div className="flex h-40 items-center justify-center gap-2 text-sm text-[var(--text-muted)]">
                  <Loader2 size={15} className="animate-spin" />
                  加载模板…
                </div>
              ) : error && !selected ? (
                <div className="m-3 rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">
                  {error}
                </div>
              ) : filteredTemplates.length === 0 ? (
                <div className="flex h-40 flex-col items-center justify-center gap-2 text-sm text-[var(--text-muted)]">
                  <Braces size={24} className="opacity-50" />
                  {templates.length === 0
                    ? "暂无可用模板，请先到 SQL 模板页面创建"
                    : "没有匹配的模板"}
                </div>
              ) : (
                <div className="space-y-1 p-2">
                  {filteredTemplates.map((template) => {
                    const usable = isUsableQueryTemplate(template, types);
                    const active = selected?.id === template.id;
                    return (
                      <button
                        key={template.id}
                        type="button"
                        disabled={!usable}
                        onClick={() => selectTemplate(template)}
                        className={`w-full rounded-md border p-3 text-left transition-colors ${
                          active
                            ? "border-[var(--accent-primary)] bg-[var(--accent-primary)]/10"
                            : "border-transparent hover:border-[var(--border-default)] hover:bg-[var(--bg-elevated)]"
                        } disabled:cursor-not-allowed disabled:opacity-45`}
                      >
                        <div className="flex items-start justify-between gap-2">
                          <div className="min-w-0">
                            <div className="truncate text-sm font-medium text-[var(--text-primary)]">
                              {template.name}
                            </div>
                            <div className="mt-1 line-clamp-2 text-xs text-[var(--text-muted)]">
                              {template.description || template.sql_content}
                            </div>
                          </div>
                          <Badge variant="outline" className="shrink-0 text-[10px]">
                            {datasourceLabel(template.db_type)}
                          </Badge>
                        </div>
                        <div className="mt-2 flex items-center gap-2 text-[10px] text-[var(--text-muted)]">
                          <span>{template.category || "general"}</span>
                          <span>·</span>
                          <span>
                            {parseParamsJSON(template.params_json).length} 个参数
                          </span>
                          {!usable && (
                            <>
                              <span>·</span>
                              <span>仅支持预览/工单</span>
                            </>
                          )}
                        </div>
                      </button>
                    );
                  })}
                </div>
              )}
            </ScrollArea>
          </div>

          <div className="flex min-w-0 flex-col bg-[var(--bg-base)]/45">
            {selected ? (
              <>
                <div className="border-b border-[var(--border-default)] p-4">
                  <div className="text-sm font-medium text-[var(--text-primary)]">
                    {selected.name}
                  </div>
                  <pre className="mt-2 max-h-28 overflow-auto whitespace-pre-wrap rounded-md bg-[var(--bg-base)] p-2.5 font-mono text-xs text-[var(--text-secondary)]">
                    {selected.sql_content}
                  </pre>
                </div>
                <ScrollArea className="h-[245px]">
                  <div className="space-y-3 p-4">
                    {params.length === 0 ? (
                      <div className="rounded-md border border-dashed border-[var(--border-default)] p-4 text-center text-xs text-[var(--text-muted)]">
                        该模板没有参数，可直接生成查询
                      </div>
                    ) : (
                      params.map((param) => (
                        <div key={param.name} className="space-y-1.5">
                          <label className="flex items-center gap-1.5 text-xs text-[var(--text-secondary)]">
                            <code className="font-mono">{param.name}</code>
                            {param.default && (
                              <span className="text-[var(--text-muted)]">
                                默认：{param.default}
                              </span>
                            )}
                          </label>
                          <Input
                            aria-label={`模板参数 ${param.name}`}
                            value={values[param.name] ?? ""}
                            onChange={(event) =>
                              setValues((current) => ({
                                ...current,
                                [param.name]: event.target.value,
                              }))
                            }
                            placeholder={param.default || "请输入参数值"}
                          />
                        </div>
                      ))
                    )}
                  </div>
                </ScrollArea>
                <div className="mt-auto border-t border-[var(--border-default)] p-3">
                  {error && (
                    <div className="mb-2 rounded-md bg-red-500/10 px-2.5 py-2 text-xs text-red-400">
                      {error}
                    </div>
                  )}
                  <Button
                    className="w-full"
                    disabled={rendering}
                    onClick={handleUseTemplate}
                  >
                    {rendering && (
                      <Loader2 size={14} className="mr-1.5 animate-spin" />
                    )}
                    生成查询
                  </Button>
                </div>
              </>
            ) : (
              <div className="flex flex-1 flex-col items-center justify-center gap-2 px-6 text-center">
                <Braces size={30} className="text-[var(--text-muted)] opacity-60" />
                <div className="text-sm font-medium text-[var(--text-secondary)]">
                  选择一个只读模板
                </div>
                <div className="text-xs leading-5 text-[var(--text-muted)]">
                  支持 MySQL 与 PostgreSQL 的 SELECT/WITH 模板。
                  <br />
                  参数会由数据库驱动安全绑定。
                </div>
              </div>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
