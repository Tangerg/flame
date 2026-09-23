import type { Element, Root, Text } from "hast";
import { segmentWords } from "@/lib/i18n/segmentWords";
import { rewriteTextNodes } from "./rewriteTextNodes";

const SKIP_TAGS = new Set(["pre", "code", "script", "style"]);

const WHITESPACE = /^\s+$/;

function optedOut(node: Element): boolean {
  const properties = node.properties ?? {};
  return Boolean(properties.dataNoFade ?? properties["data-no-fade"]);
}

export function rehypeFadeIn() {
  return (tree: Root) => {
    rewriteTextNodes(
      tree,
      (node) => SKIP_TAGS.has(node.tagName) || optedOut(node),
      (value) => {
        if (!value) return null;
        const segments = segmentWords(value);
        if (segments.every((segment) => WHITESPACE.test(segment))) return null;

        return segments.map((segment) =>
          WHITESPACE.test(segment)
            ? ({ type: "text", value: segment } satisfies Text)
            : ({
                type: "element",
                tagName: "span",
                properties: { className: ["fade-in"] },
                children: [{ type: "text", value: segment }],
              } satisfies Element),
        );
      },
    );
  };
}
