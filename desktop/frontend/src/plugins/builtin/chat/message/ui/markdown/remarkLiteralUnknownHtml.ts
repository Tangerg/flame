import type { Root, Text } from "mdast";
import { visit } from "unist-util-visit";

const BASIC_RAW_HTML = /^<\s*(?:br\s*\/?|\/?\s*(?:b|del|em|i|kbd|s|strong|sub|sup|u))\s*>$/i;

export function remarkLiteralUnknownHtml() {
  return (tree: Root): void => {
    visit(tree, "html", (node, index, parent) => {
      if (index === undefined || parent === undefined) return;
      if (BASIC_RAW_HTML.test(node.value.trim())) return;
      const literal: Text = { type: "text", value: node.value };
      parent.children[index] = literal;
    });
  };
}
