import type { Element, ElementContent, Root, RootContent } from "hast";

const ATOMIC = new Set(["img", "br", "hr", "input", "pre", "katex", "table"]);

/**
 * Whitespace between blocks, which `remark-rehype` inserts and nobody typed. Skipped when
 * looking for the last thing in a node, so a trailing newline does not read as "the paragraph
 * ended with text" and leave the caret outside it.
 */
function isBlank(node: RootContent | ElementContent): boolean {
  return node.type === "text" && node.value.trim() === "";
}

/** The last thing in `node` that a reader would see, or nothing if it holds only whitespace. */
function lastMeaningful(node: Root | Element): RootContent | ElementContent | undefined {
  for (let i = node.children.length - 1; i >= 0; i -= 1) {
    const child = node.children[i]!;
    if (!isBlank(child)) return child;
  }
  return undefined;
}

/**
 * Puts the typing caret after the last thing the reader can see.
 *
 * It descends only while the last thing in a node IS an element, which is the whole of the
 * correction here. The first version searched BACKWARDS for the last element and descended
 * into whatever it found, so a paragraph ending in plain text — the common case in a stream —
 * sent the caret into whichever emphasis, link or code span happened to come before that text.
 * Measured in the rendered DOM: `Hello **world** and more text` put it inside `<strong>` with
 * " and more text" still to its right, and the same for a link and an inline code span.
 *
 * `ATOMIC` stops the descent at elements that own their internals — a caret inside `<pre>` or
 * `<table>` is laid out by rules that are not the transcript's.
 */
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
