import type { Element, ElementContent, Root, RootContent, Text } from "hast";

/**
 * The skip is checked at EVERY level on the way down, and that is the whole reason this exists
 * rather than a `visit` with a check on `parent`. A plugin that asks "is my parent an `<a>`?"
 * says yes for `[src/foo.ts](url)` and no for `[**src/foo.ts**](url)`, because the second one's
 * parent is the `<strong>` in between — so a file reference inside a bold link became a
 * `<button>` nested in an `<a target="_blank">`, which is invalid HTML and two things to
 * activate where the reader sees one.
 *
 * `replace` returns `null` to leave a text node alone, so a caller that finds nothing to do
 * costs one array scan and no allocation.
 */
export function rewriteTextNodes(
  tree: Root,
  skip: (node: Element) => boolean,
  replace: (value: string) => Array<Element | Text> | null,
): void {
  const walk = (node: Root | Element): void => {
    let rewritten: Array<RootContent | ElementContent> | undefined;

    for (let index = 0; index < node.children.length; index += 1) {
      const child = node.children[index]!;

      if (child.type === "element") {
        if (!skip(child)) walk(child);
        rewritten?.push(child);
        continue;
      }

      const parts = child.type === "text" ? replace(child.value) : null;
      if (parts === null) {
        rewritten?.push(child);
        continue;
      }

      // First rewrite in this node: everything before it was left alone, so copy it once and
      // carry on appending. A node with nothing to rewrite never builds an array at all.
      rewritten ??= node.children.slice(0, index);
      rewritten.push(...parts);
    }

    if (rewritten) node.children = rewritten as Element["children"];
  };

  walk(tree);
}
