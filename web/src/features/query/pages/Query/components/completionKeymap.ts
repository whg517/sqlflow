import type { EditorView } from "@codemirror/view";
import { indentMore } from "@codemirror/commands";
import { acceptCompletion } from "@codemirror/autocomplete";

/**
 * Tab accepts the selected completion when the popup is active. When there is
 * no completion to accept, it falls back to normal editor indentation.
 */
export function acceptCompletionOrIndent(view: EditorView): boolean {
  return acceptCompletion(view) || indentMore(view);
}

export const tabCompletionKeymap = {
  key: "Tab",
  run: acceptCompletionOrIndent,
};
