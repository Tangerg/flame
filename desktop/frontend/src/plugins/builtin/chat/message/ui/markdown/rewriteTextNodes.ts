import type { Element, ElementContent, Root, RootContent, Text } from "hast";

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

      rewritten ??= node.children.slice(0, index);
      rewritten.push(...parts);
    }

    if (rewritten) node.children = rewritten as Element["children"];
  };

  walk(tree);
}
