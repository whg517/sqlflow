import { useState, useEffect, useCallback, useRef } from "react";
import { toast } from "sonner";
import { Braces, Clock, Link2 } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { api } from "@/api/client";
import {
  executeQuery,
  fetchESIndices,
  streamAIReview,
  buildMongoSql,
  buildESQuerySql,
  type MongoQueryBody,
  type AIReviewResult,
} from "@/api/query";
import { explainQuery, type ExplainResult } from "@/api/explain";
import { useQueryStore, type MongoOperation } from "@/store/queryStore";
import {
  useSchemaCompletion,
  type SchemaData,
} from "@/hooks/useSchemaCompletion";
import SqlEditor from "./components/SqlEditor";
import MongoEditor from "./components/MongoEditor";
import ElasticEditor from "./components/ElasticEditor";
import ResultTable from "./components/ResultTable";
import QueryTabs from "./components/QueryTabs";
import HistoryPanel from "./components/HistoryPanel";
import StatusBar from "./components/StatusBar";
import ResizableSplit from "./components/ResizableSplit";
import AIReviewCard from "./components/AIReviewCard";
import TicketSubmitSheet from "./components/TicketSubmitSheet";
import ExplainPanel from "./components/ExplainPanel";
import ShareButton from "./components/ShareButton";
import ShareListPanel from "./components/ShareListPanel";
import TemplatePickerDialog from "./components/TemplatePickerDialog";


// --- Types ---

interface DataSourceOption {
  id: number;
  name: string;
  type: string;
  status: string;
}

interface DataSourceListResponse {
  code: number;
  data: DataSourceOption[];
}

// --- Main Page ---

export default function QueryPage() {
  const tabs = useQueryStore((s) => s.tabs);
  const activeTabId = useQueryStore((s) => s.activeTabId);
  const splitRatio = useQueryStore((s) => s.splitRatio);
  const historyOpen = useQueryStore((s) => s.historyOpen);
  const setSplitRatio = useQueryStore((s) => s.setSplitRatio);
  const updateTabSql = useQueryStore((s) => s.updateTabSql);
  const updateTabDatasource = useQueryStore((s) => s.updateTabDatasource);
  const setTabResult = useQueryStore((s) => s.setTabResult);
  const setTabExecuting = useQueryStore((s) => s.setTabExecuting);
  const setHistoryOpen = useQueryStore((s) => s.setHistoryOpen);
  const updateMongoField = useQueryStore((s) => s.updateMongoField);
  const updateESField = useQueryStore((s) => s.updateESField);
  const setAIReviewStatus = useQueryStore((s) => s.setAIReviewStatus);
  const setAIReviewResult = useQueryStore((s) => s.setAIReviewResult);
  const appendAIReviewContent = useQueryStore((s) => s.appendAIReviewContent);
  const setAIReviewError = useQueryStore((s) => s.setAIReviewError);
  const clearAIReview = useQueryStore((s) => s.clearAIReview);

  const [datasources, setDatasources] = useState<DataSourceOption[]>([]);
  const [datasourceLoadError, setDatasourceLoadError] = useState("");
  const [ticketSheetOpen, setTicketSheetOpen] = useState(false);
  const [schemaMetadata, setSchemaMetadata] = useState<{
    datasourceId: number;
    data: SchemaData | null;
  } | null>(null);
  const [esIndexMetadata, setESIndexMetadata] = useState<{
    datasourceId: number;
    patterns: string[];
  } | null>(null);
  const [explainSheetOpen, setExplainSheetOpen] = useState(false);
  const [explainResult, setExplainResult] = useState<ExplainResult | null>(null);
  const [explaining, setExplaining] = useState(false);
  const [explainError, setExplainError] = useState<string | null>(null);
  const [shareListOpen, setShareListOpen] = useState(false);
  const [templatePickerOpen, setTemplatePickerOpen] = useState(false);

  const { fetchTables, fetchColumns, clearDatasourceCache } =
    useSchemaCompletion();
  void clearDatasourceCache;

  const cancelReviewRef = useRef<(() => void) | null>(null);

  const activeTab = tabs.find((t) => t.id === activeTabId) ?? tabs[0];
  const isMongo = activeTab?.datasourceType === "mongodb";
  const isES = activeTab?.datasourceType === "elasticsearch";

  // Load datasources
  useEffect(() => {
    api
      .get<DataSourceListResponse>("/datasources/available")
      .then((res) => {
        const list = (res.data ?? []).filter((ds) => ds.status === "active");
        setDatasources(list);
        setDatasourceLoadError(list.length === 0 ? "当前没有可用数据源" : "");
      })
      .catch((err) => {
        setDatasourceLoadError(err instanceof Error ? err.message : "数据源加载失败");
      });
  }, []);

  // Select a compatible datasource for tabs created after the page has mounted,
  // including tabs generated from the in-workbench template picker.
  useEffect(() => {
    if (datasources.length === 0) return;
    const frame = requestAnimationFrame(() => {
      for (const tab of tabs) {
        if (tab.datasourceId) continue;
        const datasource = tab.sourceTemplateId
          ? datasources.find((item) => item.type === tab.datasourceType)
          : datasources[0];
        if (datasource) {
          updateTabDatasource(tab.id, datasource.id, "", datasource.type);
        } else if (tab.sourceTemplateId) {
          setDatasourceLoadError(
            `模板需要 ${tab.datasourceType} 数据源，但当前没有可用实例`,
          );
        }
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [datasources, tabs, updateTabDatasource]);

  // Cleanup review stream on unmount
  useEffect(() => {
    return () => {
      cancelReviewRef.current?.();
    };
  }, []);

  // Load the metadata endpoint appropriate for the selected datasource type.
  useEffect(() => {
    let cancelled = false;

    if (!activeTab?.datasourceId) {
      return;
    }
    const datasourceId = activeTab.datasourceId;

    if (activeTab.datasourceType === "elasticsearch") {
      void fetchESIndices(datasourceId)
        .then((indices) => {
          if (!cancelled) {
            setESIndexMetadata({
              datasourceId,
              patterns: indices.map((index) => index.name),
            });
          }
        })
        .catch(() => {
          if (!cancelled) {
            setESIndexMetadata({
              datasourceId,
              patterns: [],
            });
          }
        });
      return () => {
        cancelled = true;
      };
    }

    void fetchTables(datasourceId).then((data) => {
      if (!cancelled) {
        setSchemaMetadata({
          datasourceId,
          data: data ?? null,
        });
      }
    });
    return () => {
      cancelled = true;
    };
  }, [
    activeTab?.datasourceId,
    activeTab?.datasourceType,
    fetchTables,
  ]);

  // Derive: when datasource is cleared, schema is implicitly null
  const effectiveSchemaData =
    activeTab?.datasourceId === schemaMetadata?.datasourceId
      ? schemaMetadata.data
      : null;
  const esIndexPatterns =
    activeTab?.datasourceId === esIndexMetadata?.datasourceId
      ? esIndexMetadata.patterns
      : [];

  const buildMongoQuerySql = useCallback((): string | null => {
    if (!activeTab) return null;

    let filter: Record<string, unknown>;
    let options: Record<string, unknown>;
    try {
      filter = JSON.parse(activeTab.mongoFilter || "{}");
    } catch {
      toast.error("Filter JSON 格式错误");
      return null;
    }
    try {
      options = JSON.parse(activeTab.mongoOptions || "{}");
    } catch {
      toast.error("Options JSON 格式错误");
      return null;
    }

    if (!activeTab.mongoCollection.trim()) {
      toast.error("请输入集合名");
      return null;
    }

    const body: MongoQueryBody = {
      collection: activeTab.mongoCollection.trim(),
      operation:
        activeTab.mongoOperation === "update"
          ? "update"
          : activeTab.mongoOperation,
    };

    if (activeTab.mongoOperation === "aggregate") {
      body.pipeline = Array.isArray(filter) ? filter : [];
    } else {
      body.filter = filter;
    }

    if (options && Object.keys(options).length > 0) {
      body.options = options;
    }

    return buildMongoSql(body);
  }, [activeTab]);

  // Execute query directly (bypasses AI review — used for confirmed/low-risk)
  const doExecute = useCallback(
    async (sql: string) => {
      if (!activeTab?.datasourceId) return;

      setTabExecuting(activeTab.id, true);
      try {
        const result = await executeQuery({
          datasource_id: activeTab.datasourceId,
          database: activeTab.database,
          sql,
          params: activeTab.queryParams,
        });
        setTabResult(activeTab.id, result);
        clearAIReview(activeTab.id);
        if (!activeTab.result) {
          setSplitRatio(0.5);
        }
      } catch (err) {
        setTabResult(
          activeTab.id,
          null,
          err instanceof Error ? err.message : "查询执行失败",
        );
        toast.error(err instanceof Error ? err.message : "查询执行失败");
      }
    },
    [activeTab, setTabExecuting, setTabResult, setSplitRatio, clearAIReview],
  );

  // Start AI review + execute flow
  const handleExecute = useCallback(async () => {
    if (!activeTab?.datasourceId) return;

    let sql: string;
    if (isMongo) {
      if (activeTab.mongoOperation === "update") {
        toast.error("MongoDB update 操作需要通过变更工单提交");
        return;
      }
      const mongoSql = buildMongoQuerySql();
      if (!mongoSql) return;
      sql = mongoSql;
    } else if (isES) {
      if (!activeTab.esIndexPattern.trim()) {
        toast.error("请输入 Index Pattern");
        return;
      }
      sql = buildESQuerySql(
        activeTab.esIndexPattern.trim(),
        activeTab.esQueryBody,
      );
    } else {
      if (!activeTab.sql.trim()) return;
      sql = activeTab.sql.trim();
    }

    // Cancel any previous review
    cancelReviewRef.current?.();
    cancelReviewRef.current = null;

    // Clear previous review state and start reviewing
    clearAIReview(activeTab.id);
    setAIReviewStatus(activeTab.id, "reviewing");

    const cancel = streamAIReview(
      {
        datasource_id: activeTab.datasourceId,
        database: activeTab.database,
        sql,
      },
      (event) => {
        if (event.type === "content") {
          appendAIReviewContent(activeTab.id, String(event.data));
        } else if (event.type === "result") {
          const result = event.data as AIReviewResult;
          setAIReviewResult(activeTab.id, result);

          // Auto-execute for low risk
          if (result.decision === "execute") {
            setTimeout(() => {
              doExecute(sql);
            }, 1000);
          }
        } else if (event.type === "error") {
          setAIReviewError(activeTab.id, String(event.data));
        }
      },
      (err) => {
        setAIReviewError(activeTab.id, err.message);
      },
    );

    cancelReviewRef.current = cancel;
  }, [
    activeTab,
    isMongo,
    isES,
    buildMongoQuerySql,
    clearAIReview,
    setAIReviewStatus,
    appendAIReviewContent,
    setAIReviewResult,
    setAIReviewError,
    doExecute,
  ]);

  // Handle EXPLAIN
  const handleExplain = useCallback(async () => {
    if (!activeTab?.datasourceId || !activeTab.sql.trim()) return;

    setExplaining(true);
    setExplainError(null);
    setExplainResult(null);
    setExplainSheetOpen(true);

    try {
      const result = await explainQuery(
        activeTab.sql.trim(),
        activeTab.datasourceId,
        activeTab.database,
        activeTab.queryParams,
      );
      setExplainResult(result);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "获取执行计划失败";
      setExplainError(msg);
      toast.error(msg);
    } finally {
      setExplaining(false);
    }
  }, [activeTab]);

  // Handle confirmed execute (medium risk)
  const handleConfirmExecute = useCallback(() => {
    if (!activeTab) return;
    let sql: string;
    if (isMongo) {
      sql = buildMongoQuerySql() || "";
    } else if (isES) {
      sql = buildESQuerySql(activeTab.esIndexPattern.trim(), activeTab.esQueryBody);
    } else {
      sql = activeTab.sql.trim();
    }
    if (sql) {
      doExecute(sql);
    }
  }, [activeTab, isMongo, isES, buildMongoQuerySql, doExecute]);

  // Handle auto-execute trigger from low-risk
  const handleAutoExecute = useCallback(() => {
    if (!activeTab) return;
    let sql: string;
    if (isMongo) {
      sql = buildMongoQuerySql() || "";
    } else if (isES) {
      sql = buildESQuerySql(activeTab.esIndexPattern.trim(), activeTab.esQueryBody);
    } else {
      sql = activeTab.sql.trim();
    }
    if (sql) {
      doExecute(sql);
    }
  }, [activeTab, isMongo, isES, buildMongoQuerySql, doExecute]);

  // Handle submit ticket (high risk)
  const handleSubmitTicket = useCallback(() => {
    setTicketSheetOpen(true);
  }, []);

  // Handle dismiss review
  const handleDismissReview = useCallback(() => {
    cancelReviewRef.current?.();
    cancelReviewRef.current = null;
    clearAIReview(activeTab.id);
  }, [activeTab, clearAIReview]);

  // Handle ticket submit success
  const handleTicketSuccess = useCallback(
    (ticketId: number) => {
      toast.success(`工单 #${ticketId} 已提交，等待审批`);
      clearAIReview(activeTab.id);
    },
    [activeTab, clearAIReview],
  );

  const currentSql = isMongo || isES ? "" : (activeTab?.sql ?? "");

  return (
    <div className="flex h-full flex-col">
      {/* Toolbar: datasource selectors + history button */}
      <div className="flex items-center gap-2 border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2">
        <Select
          value={activeTab?.datasourceId ? String(activeTab.datasourceId) : ""}
          onValueChange={(v) => {
            const ds = datasources.find((d) => d.id === Number(v));
            if (
              activeTab.sourceTemplateId &&
              ds?.type !== activeTab.datasourceType
            ) {
              toast.error(`该模板需要 ${activeTab.datasourceType} 数据源`);
              return;
            }
            updateTabDatasource(
              activeTab.id,
              Number(v),
              ds?.type === "mysql" ? activeTab.database : activeTab.database,
              ds?.type ?? "",
            );
          }}
        >
          <SelectTrigger className="h-8 w-48 border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm">
            <SelectValue placeholder="选择数据源" />
          </SelectTrigger>
          <SelectContent>
            {datasources.map((ds) => (
              <SelectItem
                key={ds.id}
                value={String(ds.id)}
                disabled={
                  !!activeTab.sourceTemplateId &&
                  ds.type !== activeTab.datasourceType
                }
              >
                <span className="flex items-center gap-2">
                  <span
                    className={`inline-block h-1.5 w-1.5 rounded-full ${ds.type === "mysql" ? "bg-blue-400" : ds.type === "elasticsearch" ? "bg-orange-400" : "bg-green-400"}`}
                  />
                  {ds.name}
                  <span className="text-[var(--text-muted)]">({ds.type})</span>
                </span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {datasourceLoadError && (
          <span className="text-xs text-amber-400">{datasourceLoadError}</span>
        )}

        <input
          type="text"
          value={activeTab?.database ?? ""}
          onChange={(e) =>
            updateTabDatasource(
              activeTab.id,
              activeTab.datasourceId,
              e.target.value,
              activeTab.datasourceType,
            )
          }
          placeholder={isMongo ? "Database" : "数据库名"}
          className="h-8 w-36 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-2.5 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent-primary)]"
        />

        {/* DB type indicator */}
        {isMongo && (
          <span className="rounded bg-green-500/20 px-1.5 py-0.5 text-[10px] font-medium text-green-400">
            MongoDB
          </span>
        )}
        {isES && (
          <span className="rounded bg-orange-500/20 px-1.5 py-0.5 text-[10px] font-medium text-orange-400">
            Elasticsearch
          </span>
        )}

        <div className="flex-1" />

        <Button
          variant="ghost"
          size="sm"
          className="h-8 gap-1.5 px-2.5 text-sm text-[var(--text-secondary)]"
          onClick={() => setTemplatePickerOpen(true)}
        >
          <Braces size={14} />
          使用模板
        </Button>

        {/* History toggle */}
        <div className="relative" data-history-panel>
          <Button
            variant="ghost"
            size="sm"
            className={`h-8 gap-1 px-2.5 text-sm ${historyOpen ? "text-[var(--accent-primary)]" : "text-[var(--text-secondary)]"}`}
            onClick={() => setHistoryOpen(!historyOpen)}
          >
            <Clock size={14} />
            历史
          </Button>
          <HistoryPanel />
        </div>
      </div>

      {/* Tabs */}
      <QueryTabs />

      {activeTab?.sourceTemplateId && (
        <div className="flex items-center gap-2 border-b border-[var(--border-default)] bg-[var(--accent-primary)]/5 px-3 py-1.5 text-xs text-[var(--text-secondary)]">
          <Braces size={13} className="text-[var(--accent-primary)]" />
          <span>
            来源模板：
            <strong className="font-medium text-[var(--text-primary)]">
              {activeTab.sourceTemplateName}
            </strong>
          </span>
          <Badge
            variant="outline"
            className="h-5 border-[var(--accent-primary)]/30 px-1.5 text-[10px] text-[var(--accent-primary)]"
          >
            {activeTab.queryParams.length} 个绑定参数
          </Badge>
          <span className="text-[var(--text-muted)]">
            编辑 SQL 将自动解除参数绑定
          </span>
        </div>
      )}

      {/* AI Review Card (between editor and result) */}
      <AIReviewCard
        status={activeTab?.aiReviewStatus ?? "idle"}
        result={activeTab?.aiReviewResult ?? null}
        streamingContent={activeTab?.aiReviewContent ?? ""}
        error={activeTab?.aiReviewError ?? null}
        onConfirm={handleConfirmExecute}
        onAutoExecute={handleAutoExecute}
        onSubmitTicket={handleSubmitTicket}
        onDismiss={handleDismissReview}
      />

      {/* Main editor + result area */}
      <div className="flex-1 overflow-hidden">
        <ResizableSplit
          ratio={splitRatio}
          onRatioChange={setSplitRatio}
          top={
            isMongo ? (
              <MongoEditor
                key={`mongo-${activeTab?.id}`}
                collection={activeTab?.mongoCollection ?? ""}
                operation={activeTab?.mongoOperation ?? "find"}
                filter={activeTab?.mongoFilter ?? "{}"}
                options={activeTab?.mongoOptions ?? "{}"}
                onCollectionChange={(v) =>
                  updateMongoField(activeTab.id, { mongoCollection: v })
                }
                onOperationChange={(v: MongoOperation) =>
                  updateMongoField(activeTab.id, { mongoOperation: v })
                }
                onFilterChange={(v) =>
                  updateMongoField(activeTab.id, { mongoFilter: v })
                }
                onOptionsChange={(v) =>
                  updateMongoField(activeTab.id, { mongoOptions: v })
                }
                onExecute={handleExecute}
                collectionNames={
                  isMongo && effectiveSchemaData
                    ? effectiveSchemaData.tables
                    : []
                }
              />
            ) : isES ? (
              <ElasticEditor
                key={`es-${activeTab?.id}`}
                indexPattern={activeTab?.esIndexPattern ?? ""}
                queryBody={activeTab?.esQueryBody ?? ""}
                onIndexPatternChange={(v) =>
                  updateESField(activeTab.id, { esIndexPattern: v })
                }
                onQueryBodyChange={(v) =>
                  updateESField(activeTab.id, { esQueryBody: v })
                }
                onExecute={handleExecute}
                defaultIndexPatterns={esIndexPatterns}
              />
            ) : (
              <SqlEditor
                key={activeTab?.id}
                value={activeTab?.sql ?? ""}
                onChange={(sql) => updateTabSql(activeTab.id, sql)}
                onExecute={handleExecute}
                schemaData={effectiveSchemaData}
                onFetchColumns={async (tableName: string) => {
                  if (!activeTab?.datasourceId) return [];
                  return fetchColumns(activeTab.datasourceId, tableName);
                }}
              />
            )
          }
          bottom={
            <div className="flex h-full flex-col">
              <div className="flex items-center justify-between border-b border-[var(--border-default)] px-2 py-1">
                <div className="flex items-center gap-1">
                  {activeTab?.result && activeTab.result.rows.length > 0 && (
                    <ShareButton
                      columns={activeTab.result.columns}
                      rows={activeTab.result.rows}
                      sqlSummary={currentSql.trim().substring(0, 200)}
                    />
                  )}
                </div>
                <div className="flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 gap-1 text-[10px]"
                    onClick={() => setShareListOpen((v) => !v)}
                  >
                    <Link2 size={10} />
                    我的共享
                  </Button>
                </div>
              </div>
              <div className="flex-1 overflow-hidden">
                <ResultTable result={activeTab?.result ?? null} />
              </div>
              <StatusBar
                executing={activeTab?.executing ?? false}
                error={activeTab?.error ?? null}
                result={activeTab?.result ?? null}
                datasourceId={activeTab?.datasourceId ?? null}
                database={activeTab?.database ?? ""}
                sql={currentSql}
                params={activeTab?.queryParams ?? []}
                dbType={activeTab?.datasourceType ?? ""}
                onExecute={handleExecute}
                onExplain={handleExplain}
                explaining={explaining}
                isMongo={isMongo}
                mongoCollection={activeTab?.mongoCollection ?? ""}
              />
            </div>
          }
        />
      </div>

      {/* Ticket submission sheet for high-risk operations */}
      <TicketSubmitSheet
        open={ticketSheetOpen}
        onOpenChange={setTicketSheetOpen}
        sql={currentSql}
        datasourceId={activeTab?.datasourceId ?? null}
        database={activeTab?.database ?? ""}
        dbType={activeTab?.datasourceType ?? ""}
        reviewResult={activeTab?.aiReviewResult ?? null}
        onSubmitSuccess={handleTicketSuccess}
      />

      <TemplatePickerDialog
        open={templatePickerOpen}
        onOpenChange={setTemplatePickerOpen}
      />

      {/* Explain panel sheet */}
      <Sheet open={explainSheetOpen} onOpenChange={setExplainSheetOpen}>
        <SheetContent
          side="right"
          className="w-[700px] sm:max-w-[700px] border-[var(--border-default)] bg-[var(--bg-surface)]"
        >
          <SheetHeader>
            <SheetTitle className="text-[var(--text-primary)]">
              执行计划
            </SheetTitle>
          </SheetHeader>
          <div className="mt-4 overflow-auto">
            <ExplainPanel
              plan={explainResult?.plan ?? []}
              formatted={explainResult?.formatted ?? ""}
              loading={explaining}
              error={explainError}
            />
          </div>
        </SheetContent>
      </Sheet>

      {/* Share list panel */}
      <Sheet open={shareListOpen} onOpenChange={setShareListOpen}>
        <SheetContent
          side="right"
          className="w-[500px] sm:max-w-[500px] border-[var(--border-default)] bg-[var(--bg-surface)]"
        >
          <SheetHeader>
            <SheetTitle className="text-[var(--text-primary)] flex items-center gap-2">
              <Link2 size={16} />
              我的共享链接
            </SheetTitle>
          </SheetHeader>
          <div className="mt-4 overflow-auto">
            <ShareListPanel />
          </div>
        </SheetContent>
      </Sheet>

    </div>
  );
}
