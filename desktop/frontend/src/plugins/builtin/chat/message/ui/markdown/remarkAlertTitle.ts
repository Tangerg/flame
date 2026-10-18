import type { Paragraph, Root } from "mdast";
import { visit } from "unist-util-visit";

const ALERT_TITLE: Readonly<Record<string, string>> = {
  NOTE: "Note",
  TIP: "Tip",
  IMPORTANT: "Important",
  WARNING: "Warning",
  CAUTION: "Caution",
};

const isAlertTitle = (node: Paragraph) =>
  (node.data as { hProperties?: { className?: unknown } } | undefined)?.hProperties?.className ===
  "markdown-alert-title";

export function remarkAlertTitle() {
  return (tree: Root): void => {
    visit(tree, "paragraph", (node) => {
      if (!isAlertTitle(node)) return;
      for (const child of node.children) {
        if (child.type === "text") child.value = ALERT_TITLE[child.value] ?? child.value;
      }
    });
  };
}
