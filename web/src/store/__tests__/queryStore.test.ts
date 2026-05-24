import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useQueryStore } from "@/store/queryStore";

// --- Tests ---

describe("queryStore", () => {
  beforeEach(() => {
    // Reset store to initial state
    vi.resetModules();
    // Re-import to get fresh store
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // --- Initial state ---

  describe("initial state", () => {
    it("starts with one default tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const state = useQueryStore.getState();
      expect(state.tabs).toHaveLength(1);
      expect(state.tabs[0].id).toBe("tab-initial");
    });

    it("default tab has correct initial values", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const tab = useQueryStore.getState().tabs[0];
      expect(tab.title).toBe("新查询");
      expect(tab.sql).toBe("");
      expect(tab.datasourceId).toBeNull();
      expect(tab.database).toBe("");
      expect(tab.result).toBeNull();
      expect(tab.executing).toBe(false);
      expect(tab.error).toBeNull();
      expect(tab.dirty).toBe(false);
    });

    it("default tab has MongoDB fields", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const tab = useQueryStore.getState().tabs[0];
      expect(tab.mongoOperation).toBe("find");
      expect(tab.mongoFilter).toBe("{}");
      expect(tab.mongoOptions).toBe("{}");
      expect(tab.mongoCollection).toBe("");
    });

    it("default tab has AI review idle state", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const tab = useQueryStore.getState().tabs[0];
      expect(tab.aiReviewStatus).toBe("idle");
      expect(tab.aiReviewResult).toBeNull();
      expect(tab.aiReviewContent).toBe("");
      expect(tab.aiReviewError).toBeNull();
    });

    it("activeTabId matches initial tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const state = useQueryStore.getState();
      expect(state.activeTabId).toBe(state.tabs[0].id);
    });

    it("historyOpen is false by default", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      expect(useQueryStore.getState().historyOpen).toBe(false);
    });

    it("splitRatio defaults to 0.5", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      expect(useQueryStore.getState().splitRatio).toBe(0.5);
    });
  });

  // --- Tab management ---

  describe("addTab", () => {
    it("adds a new tab and sets it as active", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();

      store.addTab();

      const state = useQueryStore.getState();
      expect(state.tabs).toHaveLength(2);
      expect(state.activeTabId).toBe(state.tabs[1].id);
    });

    it("new tab has correct default values", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      useQueryStore.getState().addTab();

      const tabs = useQueryStore.getState().tabs;
      const newTab = tabs[tabs.length - 1];
      expect(newTab.title).toBe("新查询");
      expect(newTab.sql).toBe("");
      expect(newTab.datasourceId).toBeNull();
    });
  });

  describe("removeTab", () => {
    it("removes a tab from the list", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      store.addTab();

      const tabs = useQueryStore.getState().tabs;
      const secondTabId = tabs[1].id;
      store.removeTab(secondTabId);

      expect(useQueryStore.getState().tabs).toHaveLength(1);
    });

    it("does not remove the last tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const firstTabId = store.tabs[0].id;

      store.removeTab(firstTabId);

      expect(useQueryStore.getState().tabs).toHaveLength(1);
    });

    it("activates the adjacent tab when removing active tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      store.addTab(); // adds tab at index 1
      store.addTab(); // adds tab at index 2

      // Active is now tab[2]
      const tabs = useQueryStore.getState().tabs;
      const lastTabId = tabs[2].id;
      store.removeTab(lastTabId);

      // Should activate tab[1]
      expect(useQueryStore.getState().activeTabId).toBe(tabs[1].id);
    });

    it("activates previous tab when removing last tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      store.addTab();

      const tabs = useQueryStore.getState().tabs;
      const lastTabId = tabs[1].id;
      store.removeTab(lastTabId);

      // Should go back to tab[0]
      expect(useQueryStore.getState().activeTabId).toBe(tabs[0].id);
    });
  });

  describe("setActiveTab", () => {
    it("switches the active tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      store.addTab();

      const tabs = useQueryStore.getState().tabs;
      store.setActiveTab(tabs[0].id);

      expect(useQueryStore.getState().activeTabId).toBe(tabs[0].id);
    });
  });

  // --- Tab SQL ---

  describe("updateTabSql", () => {
    it("updates SQL content for a tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.updateTabSql(tabId, "SELECT * FROM users");

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.sql).toBe("SELECT * FROM users");
    });

    it("sets dirty flag to true when SQL is updated", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.updateTabSql(tabId, "SELECT 1");

      expect(
        useQueryStore.getState().tabs.find((t) => t.id === tabId)?.dirty,
      ).toBe(true);
    });

    it("updates title with SQL preview (truncated to 20 chars)", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.updateTabSql(tabId, "SELECT * FROM users WHERE id = 1");

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.title).toBe("SELECT * FROM users ...");
    });

    it('resets title to "新查询" when SQL is cleared', async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.updateTabSql(tabId, "SELECT 1");
      store.updateTabSql(tabId, "");

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.title).toBe("新查询");
    });
  });

  // --- Tab datasource ---

  describe("updateTabDatasource", () => {
    it("updates datasource fields for a tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.updateTabDatasource(tabId, 10, "testdb", "mysql");

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.datasourceId).toBe(10);
      expect(tab?.database).toBe("testdb");
      expect(tab?.datasourceType).toBe("mysql");
    });
  });

  // --- Tab result ---

  describe("setTabResult", () => {
    it("sets result and clears executing/error flags", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.setTabExecuting(tabId, true);
      store.setTabResult(tabId, { columns: ["id"], rows: [[1]], total: 1 });

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.result).toEqual({ columns: ["id"], rows: [[1]], total: 1 });
      expect(tab?.executing).toBe(false);
      expect(tab?.error).toBeNull();
    });

    it("sets error when result is null and error is provided", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.setTabResult(tabId, null, "Connection failed");

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.result).toBeNull();
      expect(tab?.error).toBe("Connection failed");
      expect(tab?.executing).toBe(false);
    });
  });

  describe("setTabExecuting", () => {
    it("sets executing flag for a tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.setTabExecuting(tabId, true);
      expect(
        useQueryStore.getState().tabs.find((t) => t.id === tabId)?.executing,
      ).toBe(true);

      store.setTabExecuting(tabId, false);
      expect(
        useQueryStore.getState().tabs.find((t) => t.id === tabId)?.executing,
      ).toBe(false);
    });

    it("clears error when starting execution", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.setTabResult(tabId, null, "previous error");
      store.setTabExecuting(tabId, true);

      expect(
        useQueryStore.getState().tabs.find((t) => t.id === tabId)?.error,
      ).toBeNull();
    });
  });

  // --- MongoDB fields ---

  describe("updateMongoField", () => {
    it("updates MongoDB fields for a tab", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.updateMongoField(tabId, {
        mongoCollection: "users",
        mongoOperation: "aggregate",
        mongoFilter: '{ "age": { "$gt": 18 } }',
        mongoOptions: '{ "limit": 10 }',
      });

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.mongoCollection).toBe("users");
      expect(tab?.mongoOperation).toBe("aggregate");
      expect(tab?.mongoFilter).toBe('{ "age": { "$gt": 18 } }');
      expect(tab?.mongoOptions).toBe('{ "limit": 10 }');
    });

    it("sets dirty flag when updating MongoDB fields", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.updateMongoField(tabId, { mongoCollection: "test" });

      expect(
        useQueryStore.getState().tabs.find((t) => t.id === tabId)?.dirty,
      ).toBe(true);
    });
  });

  // --- AI Review ---

  describe("AI review actions", () => {
    it("setAIReviewStatus updates review status", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.setAIReviewStatus(tabId, "reviewing");
      expect(
        useQueryStore.getState().tabs.find((t) => t.id === tabId)
          ?.aiReviewStatus,
      ).toBe("reviewing");
    });

    it("setAIReviewResult sets result and updates status to done", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      const result = { risk_level: "low", suggestions: ["OK"] };
      store.setAIReviewResult(tabId, result as never);

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.aiReviewResult).toEqual(result);
      expect(tab?.aiReviewStatus).toBe("done");
    });

    it("setAIReviewResult resets status to idle when result is null", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.setAIReviewResult(tabId, null);

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.aiReviewStatus).toBe("idle");
    });

    it("appendAIReviewContent appends text", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.appendAIReviewContent(tabId, "Hello ");
      store.appendAIReviewContent(tabId, "World");

      expect(
        useQueryStore.getState().tabs.find((t) => t.id === tabId)
          ?.aiReviewContent,
      ).toBe("Hello World");
    });

    it("setAIReviewError sets error and status", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.setAIReviewError(tabId, "API rate limited");

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.aiReviewError).toBe("API rate limited");
      expect(tab?.aiReviewStatus).toBe("error");
    });

    it("setAIReviewError resets status when error is null", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.setAIReviewError(tabId, "some error");
      store.setAIReviewError(tabId, null);

      expect(
        useQueryStore.getState().tabs.find((t) => t.id === tabId)
          ?.aiReviewStatus,
      ).toBe("idle");
    });

    it("clearAIReview resets all AI review fields", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();
      const tabId = store.tabs[0].id;

      store.setAIReviewStatus(tabId, "reviewing");
      store.appendAIReviewContent(tabId, "some content");

      store.clearAIReview(tabId);

      const tab = useQueryStore.getState().tabs.find((t) => t.id === tabId);
      expect(tab?.aiReviewStatus).toBe("idle");
      expect(tab?.aiReviewResult).toBeNull();
      expect(tab?.aiReviewContent).toBe("");
      expect(tab?.aiReviewError).toBeNull();
    });
  });

  // --- Layout ---

  describe("layout actions", () => {
    it("setSplitRatio updates the split ratio", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      useQueryStore.getState().setSplitRatio(0.3);

      expect(useQueryStore.getState().splitRatio).toBe(0.3);
    });

    it("setSplitRatio persists to localStorage", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      useQueryStore.getState().setSplitRatio(0.7);

      expect(localStorage.getItem("query:splitRatio")).toBe("0.7");
    });

    it("setHistoryOpen toggles history panel", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      expect(useQueryStore.getState().historyOpen).toBe(false);

      useQueryStore.getState().setHistoryOpen(true);
      expect(useQueryStore.getState().historyOpen).toBe(true);

      useQueryStore.getState().setHistoryOpen(false);
      expect(useQueryStore.getState().historyOpen).toBe(false);
    });
  });

  // --- restoreHistoryAsTab ---

  describe("restoreHistoryAsTab", () => {
    it("creates a new tab with history data", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const store = useQueryStore.getState();

      store.restoreHistoryAsTab("SELECT * FROM orders", 10, "production");

      const tabs = useQueryStore.getState().tabs;
      expect(tabs).toHaveLength(2);

      const newTab = tabs[1];
      expect(newTab.sql).toBe("SELECT * FROM orders");
      expect(newTab.datasourceId).toBe(10);
      expect(newTab.database).toBe("production");
      expect(newTab.dirty).toBe(false);
    });

    it("sets the new tab as active", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      useQueryStore.getState().restoreHistoryAsTab("SELECT 1", 5, "db");

      const tabs = useQueryStore.getState().tabs;
      expect(useQueryStore.getState().activeTabId).toBe(tabs[1].id);
    });

    it("sets title from SQL content", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      useQueryStore
        .getState()
        .restoreHistoryAsTab("SELECT * FROM users", 10, "db");

      const tabs = useQueryStore.getState().tabs;
      expect(tabs[1].title).toBe("SELECT * FROM users");
    });

    it("truncates long SQL title", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      const longSql =
        "SELECT * FROM very_long_table_name WHERE some_column = some_value AND another_column > 0";
      useQueryStore.getState().restoreHistoryAsTab(longSql, 10, "db");

      const tabs = useQueryStore.getState().tabs;
      expect(tabs[1].title).toBe("SELECT * FROM very_l...");
    });

    it("closes history panel after restoring", async () => {
      const { useQueryStore } = await import("@/store/queryStore");
      useQueryStore.getState().setHistoryOpen(true);
      useQueryStore.getState().restoreHistoryAsTab("SELECT 1", 10, "db");

      expect(useQueryStore.getState().historyOpen).toBe(false);
    });
  });
});
