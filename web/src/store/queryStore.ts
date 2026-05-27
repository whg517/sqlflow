import { create } from "zustand";
import type { QueryResult, AIReviewResult } from "@/api/query";

// --- Types ---

export type MongoOperation = "find" | "aggregate" | "update";

export type AIReviewStatus = "idle" | "reviewing" | "done" | "error";

export interface QueryTab {
  id: string;
  title: string;
  sql: string;
  datasourceId: number | null;
  datasourceType: string;
  database: string;
  result: QueryResult | null;
  executing: boolean;
  error: string | null;
  dirty: boolean;
  // MongoDB-specific fields
  mongoCollection: string;
  mongoOperation: MongoOperation;
  mongoFilter: string;
  mongoOptions: string;
  // AI review fields
  aiReviewStatus: AIReviewStatus;
  aiReviewResult: AIReviewResult | null;
  aiReviewContent: string;
  aiReviewError: string | null;
  // Elasticsearch-specific fields
  esIndexPattern: string;
  esQueryBody: string;
}

interface QueryStore {
  tabs: QueryTab[];
  activeTabId: string;
  splitRatio: number;
  historyOpen: boolean;

  // Tab actions
  addTab: () => void;
  removeTab: (id: string) => void;
  setActiveTab: (id: string) => void;
  updateTabSql: (id: string, sql: string) => void;
  updateTabDatasource: (
    id: string,
    datasourceId: number | null,
    database: string,
    datasourceType?: string,
  ) => void;
  setTabResult: (
    id: string,
    result: QueryResult | null,
    error?: string | null,
  ) => void;
  setTabExecuting: (id: string, executing: boolean) => void;
  restoreHistoryAsTab: (
    sql: string,
    datasourceId: number,
    database: string,
  ) => void;
  // MongoDB actions
  updateMongoField: (
    id: string,
    field: Partial<
      Pick<
        QueryTab,
        "mongoCollection" | "mongoOperation" | "mongoFilter" | "mongoOptions"
      >
    >,
  ) => void;
  // AI review actions
  setAIReviewStatus: (id: string, status: AIReviewStatus) => void;
  setAIReviewResult: (id: string, result: AIReviewResult | null) => void;
  appendAIReviewContent: (id: string, chunk: string) => void;
  setAIReviewError: (id: string, error: string | null) => void;
  clearAIReview: (id: string) => void;
  // Elasticsearch actions
  updateESField: (
    id: string,
    field: Partial<Pick<QueryTab, "esIndexPattern" | "esQueryBody">>,
  ) => void;

  // Layout
  setSplitRatio: (ratio: number) => void;
  setHistoryOpen: (open: boolean) => void;
}

let tabCounter = 0;

function createTab(): QueryTab {
  tabCounter++;
  return {
    id: `tab-${Date.now()}-${tabCounter}`,
    title: "新查询",
    sql: "",
    datasourceId: null,
    datasourceType: "",
    database: "",
    result: null,
    executing: false,
    error: null,
    dirty: false,
    mongoCollection: "",
    mongoOperation: "find",
    mongoFilter: "{}",
    mongoOptions: "{}",
    aiReviewStatus: "idle",
    aiReviewResult: null,
    aiReviewContent: "",
    aiReviewError: null,
    esIndexPattern: "",
    esQueryBody: "",
  };
}

function getStoredSplitRatio(): number {
  try {
    const stored = localStorage.getItem("query:splitRatio");
    if (stored) {
      const ratio = parseFloat(stored);
      if (ratio >= 0.1 && ratio <= 0.9) return ratio;
    }
  } catch {
    /* ignore */
  }
  return 0.5;
}

const INITIAL_TAB_ID = "tab-initial";

export const useQueryStore = create<QueryStore>((set) => ({
  tabs: [
    {
      id: INITIAL_TAB_ID,
      title: "新查询",
      sql: "",
      datasourceId: null,
      datasourceType: "",
      database: "",
      result: null,
      executing: false,
      error: null,
      dirty: false,
      mongoCollection: "",
      mongoOperation: "find",
      mongoFilter: "{}",
      mongoOptions: "{}",
      aiReviewStatus: "idle",
      aiReviewResult: null,
      aiReviewContent: "",
      aiReviewError: null,
      esIndexPattern: "",
      esQueryBody: "",
    },
  ],
  activeTabId: INITIAL_TAB_ID,
  splitRatio: getStoredSplitRatio(),
  historyOpen: false,

  addTab: () => {
    const tab = createTab();
    set((state) => ({
      tabs: [...state.tabs, tab],
      activeTabId: tab.id,
    }));
  },

  removeTab: (id) => {
    set((state) => {
      if (state.tabs.length <= 1) return state;
      const idx = state.tabs.findIndex((t) => t.id === id);
      const newTabs = state.tabs.filter((t) => t.id !== id);
      let newActiveId = state.activeTabId;
      if (state.activeTabId === id) {
        newActiveId = newTabs[Math.min(idx, newTabs.length - 1)].id;
      }
      return { tabs: newTabs, activeTabId: newActiveId };
    });
  },

  setActiveTab: (id) => set({ activeTabId: id }),

  updateTabSql: (id, sql) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id
          ? {
              ...t,
              sql,
              dirty: true,
              title: sql.trim()
                ? sql.trim().substring(0, 20) +
                  (sql.trim().length > 20 ? "..." : "")
                : "新查询",
            }
          : t,
      ),
    })),

  updateTabDatasource: (id, datasourceId, database, datasourceType) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id
          ? {
              ...t,
              datasourceId,
              database,
              datasourceType: datasourceType ?? t.datasourceType,
            }
          : t,
      ),
    })),

  setTabResult: (id, result, error = null) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id
          ? { ...t, result, error, executing: false, dirty: false }
          : t,
      ),
    })),

  setTabExecuting: (id, executing) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id ? { ...t, executing, error: null } : t,
      ),
    })),

  restoreHistoryAsTab: (sql, datasourceId, database) => {
    const tab = createTab();
    tab.sql = sql;
    tab.datasourceId = datasourceId;
    tab.database = database;
    tab.title =
      sql.trim().substring(0, 20) + (sql.trim().length > 20 ? "..." : "");
    tab.dirty = false;
    set((state) => ({
      tabs: [...state.tabs, tab],
      activeTabId: tab.id,
      historyOpen: false,
    }));
  },

  updateMongoField: (id, fields) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id ? { ...t, ...fields, dirty: true } : t,
      ),
    })),

  setAIReviewStatus: (id, status) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id ? { ...t, aiReviewStatus: status } : t,
      ),
    })),

  setAIReviewResult: (id, result) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id
          ? {
              ...t,
              aiReviewResult: result,
              aiReviewStatus: result ? "done" : "idle",
            }
          : t,
      ),
    })),

  appendAIReviewContent: (id, chunk) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id ? { ...t, aiReviewContent: t.aiReviewContent + chunk } : t,
      ),
    })),

  setAIReviewError: (id, error) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id
          ? {
              ...t,
              aiReviewError: error,
              aiReviewStatus: error ? "error" : "idle",
            }
          : t,
      ),
    })),

  clearAIReview: (id) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id
          ? {
              ...t,
              aiReviewStatus: "idle",
              aiReviewResult: null,
              aiReviewContent: "",
              aiReviewError: null,
            }
          : t,
      ),
    })),

  setSplitRatio: (ratio) => {
    try {
      localStorage.setItem("query:splitRatio", String(ratio));
    } catch {
      /* ignore */
    }
    set({ splitRatio: ratio });
  },
  setHistoryOpen: (open) => set({ historyOpen: open }),

  updateESField: (id, fields) =>
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === id ? { ...t, ...fields, dirty: true } : t,
      ),
    })),
}));
