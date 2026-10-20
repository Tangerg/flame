import { type Root } from "hast";
import { visit } from "unist-util-visit";
export function rehypeFootnoteLabels(prefix: string) {
  // mdast-util-to-hast leaves the label global even when clobberPrefix scopes fn/fnref.
  const labelId = `${prefix}footnote-label`;
  return (tree: Root): void => {
    visit(tree, "element", (node) => {
      if (node.tagName === "section" && Object.hasOwn(node.properties, "dataFootnotes")) {
        for (const child of node.children) {
          if (child.type === "element" && child.properties.id === "footnote-label") {
            child.properties.id = labelId;
          }
        }
      }
      if (node.tagName === "a" && Object.hasOwn(node.properties, "dataFootnoteRef")) {
        node.properties.ariaDescribedBy = [labelId];
      }
    });
  };
}
