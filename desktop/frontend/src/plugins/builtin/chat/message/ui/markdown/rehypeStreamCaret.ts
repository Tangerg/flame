import type { Element, Root } from "hast";

const ATOMIC = new Set(["img", "br", "hr", "input", "pre", "katex", "table"]);

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
      let lastEl: Element | undefined;
      for (let i = node.children.length - 1; i >= 0; i--) {
        const child = node.children[i]!;
        if (child.type === "element") {
          lastEl = child;
          break;
        }
      }
      if (!lastEl || ATOMIC.has(lastEl.tagName)) break;
      node = lastEl;
    }
    node.children.push(caret);
  };
}
