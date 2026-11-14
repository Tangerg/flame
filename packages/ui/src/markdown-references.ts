import { visit } from "unist-util-visit";

export function isMarkdownFragment(value: string): boolean {
  return value.startsWith("#");
}

type ReferenceElement = {
  type: "element";
  tagName: string;
  properties: Record<string, unknown>;
  children: Array<{ type: string; properties?: Record<string, unknown> }>;
};

export function rehypeFootnoteLabels(prefix: string) {
  const labelId = `${prefix}footnote-label`;
  return (tree: Parameters<typeof visit>[0]): void => {
    visit(
      tree,
      (node): node is ReferenceElement => node.type === "element",
      (node) => {
        // mdast-util-to-hast scopes fn/fnref but leaves the supporting label global.
        if (
          node.tagName === "section" &&
          Object.hasOwn(node.properties, "dataFootnotes")
        ) {
          for (const child of node.children) {
            if (
              child.type === "element" &&
              child.properties?.id === "footnote-label"
            )
              child.properties.id = labelId;
          }
        }
        if (
          node.tagName === "a" &&
          Object.hasOwn(node.properties, "dataFootnoteRef")
        )
          node.properties.ariaDescribedBy = [labelId];
      },
    );
  };
}
