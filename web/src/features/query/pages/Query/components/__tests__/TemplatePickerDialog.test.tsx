import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TemplatePickerDialog from "../TemplatePickerDialog";
import { useQueryStore } from "@/store/queryStore";

const mocks = vi.hoisted(() => ({
  listTemplates: vi.fn(),
  renderTemplate: vi.fn(),
  toastSuccess: vi.fn(),
  fetchDatasourceTypes: vi.fn(),
}));

// Whether a template can be opened is now the driver's answer — query_form ===
// "sql" — rather than a list of type names kept in the component.
vi.mock("@/shared/datasource/types", () => ({
  fetchDatasourceTypes: mocks.fetchDatasourceTypes,
}));

vi.mock("@/features/query/api/sql-template", () => ({
  listTemplates: mocks.listTemplates,
  renderTemplate: mocks.renderTemplate,
  parseParamsJSON: (value: string) => JSON.parse(value),
}));

vi.mock("sonner", () => ({
  toast: {
    success: mocks.toastSuccess,
    error: vi.fn(),
  },
}));

const template = {
  id: 7,
  user_id: 1,
  name: "按用户查询",
  description: "根据用户 ID 查询",
  sql_content: "SELECT * FROM users WHERE id = {{user_id}}",
  db_type: "mysql",
  category: "query",
  params_json: `[{"name":"user_id","default":""}]`,
  is_public: false,
  created_at: "2026-07-29T00:00:00Z",
  updated_at: "2026-07-29T00:00:00Z",
};

describe("TemplatePickerDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.fetchDatasourceTypes.mockResolvedValue([
      { type: "mysql", query_form: "sql", fields: [], placeholder_style: "positional" },
      { type: "mongodb", query_form: "document", fields: [], placeholder_style: "none" },
    ]);
    const firstTab = useQueryStore.getState().tabs[0];
    useQueryStore.setState({
      tabs: [firstTab],
      activeTabId: firstTab.id,
    });
    mocks.listTemplates.mockResolvedValue({
      items: [template],
      total: 1,
      page: 1,
      page_size: 100,
    });
    mocks.renderTemplate.mockResolvedValue({
      rendered_sql: "SELECT * FROM users WHERE id = ?",
      param_values: ["42"],
      sql: template.sql_content,
    });
  });

  it("renders a template and opens it as a parameterized query tab", async () => {
    const onOpenChange = vi.fn();
    render(
      <TemplatePickerDialog open onOpenChange={onOpenChange} />,
    );

    const templateButton = await screen.findByRole("button", {
      name: /按用户查询/,
    });
    fireEvent.click(templateButton);
    fireEvent.change(screen.getByLabelText("模板参数 user_id"), {
      target: { value: "42" },
    });
    fireEvent.click(screen.getByRole("button", { name: "生成查询" }));

    await waitFor(() => {
      expect(mocks.renderTemplate).toHaveBeenCalledWith(7, {
        user_id: "42",
      });
    });

    const state = useQueryStore.getState();
    const createdTab = state.tabs[state.tabs.length - 1];
    expect(createdTab.sql).toBe("SELECT * FROM users WHERE id = ?");
    expect(createdTab.queryParams).toEqual(["42"]);
    expect(createdTab.sourceTemplateId).toBe(7);
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      "模板已生成新的查询标签",
    );
  });

  it("shows the backend validation error when a required parameter is missing", async () => {
    mocks.renderTemplate.mockRejectedValueOnce(
      new Error("缺少必填参数：user_id"),
    );
    render(<TemplatePickerDialog open onOpenChange={vi.fn()} />);

    fireEvent.click(
      await screen.findByRole("button", { name: /按用户查询/ }),
    );
    fireEvent.click(screen.getByRole("button", { name: "生成查询" }));

    expect(
      await screen.findByText("缺少必填参数：user_id"),
    ).toBeInTheDocument();
  });
});
