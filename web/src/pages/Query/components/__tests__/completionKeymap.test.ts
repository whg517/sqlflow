import { afterEach, describe, expect, it, vi } from "vitest";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import {
  autocompletion,
  completeFromList,
  completionStatus,
  startCompletion,
} from "@codemirror/autocomplete";
import {
  acceptCompletionOrIndent,
  tabCompletionKeymap,
} from "@/pages/Query/components/completionKeymap";

let view: EditorView | null = null;

afterEach(() => {
  view?.destroy();
  view = null;
});

describe("SQL editor Tab completion", () => {
  it("binds the Tab key", () => {
    expect(tabCompletionKeymap.key).toBe("Tab");
    expect(tabCompletionKeymap.run).toBe(acceptCompletionOrIndent);
  });

  it("accepts the selected completion instead of moving focus", async () => {
    const parent = document.createElement("div");
    document.body.appendChild(parent);
    view = new EditorView({
      state: EditorState.create({
        doc: "SEL",
        selection: { anchor: 3 },
        extensions: [
          autocompletion({
            override: [
              completeFromList([
                { label: "SELECT", type: "keyword" },
                { label: "SET", type: "keyword" },
              ]),
            ],
            interactionDelay: 0,
          }),
        ],
      }),
      parent,
    });

    startCompletion(view);
    await vi.waitFor(() => {
      expect(completionStatus(view!.state)).toBe("active");
    });

    expect(acceptCompletionOrIndent(view)).toBe(true);
    expect(view.state.doc.toString()).toBe("SELECT");
  });

  it("falls back to indentation when no completion is active", () => {
    const parent = document.createElement("div");
    document.body.appendChild(parent);
    view = new EditorView({
      state: EditorState.create({
        doc: "value",
        selection: { anchor: 5 },
      }),
      parent,
    });

    expect(acceptCompletionOrIndent(view)).toBe(true);
    expect(view.state.doc.toString()).toBe("  value");
  });
});
