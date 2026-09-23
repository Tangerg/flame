import type { Element, ElementContent, Root, RootContent } from "hast";

const ATOMIC = new Set(["img", "br", "hr", "input", "pre", "katex", "table"]);

function isBlank(node: RootContent | ElementContent): boolean {
  return node.type === "text" && node.value.trim() === "";
}

function lastMeaningful(node: Root | Element): RootContent | ElementContent | undefined {
  for (let i = node.children.length - 1; i >= 0; i -= 1) {
    const child = node.children[i]!;
    if (!isBlank(child)) return child;
  }
  return undefined;
}

export function rehypeStreamCaret() {
  return (tree: Root) => {
    const caret: Element = {
      type: "element",
      tagName: "span",
      properties: { className: ["type-caret"], ariaHidden: "true" },
      children: [],
    };

    let node: Element | Root = tree;
    for (;;) {
      const last = lastMeaningful(node);
      if (!last || last.type !== "element" || ATOMIC.has(last.tagName)) break;
      node = last;
    }
    node.children.push(caret);
  };
}
