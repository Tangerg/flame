import type { Element, Root, Text } from "hast";
import { parseFileRefs } from "@/plugins/builtin/agent/public/fileRefs";
import { rewriteTextNodes } from "./rewriteTextNodes";

/**
 * Where a path is already something, or already literal. `a` is the load-bearing one: a
 * reference inside a link would be a control inside a link, and the reader sees one thing.
 */
const SKIP_TAGS = new Set(["pre", "code", "a", "sup", "script", "style"]);

export function rehypeFileRefs() {
  return (tree: Root) => {
    rewriteTextNodes(
      tree,
      (node) => SKIP_TAGS.has(node.tagName),
      (value) => {
        const segments = parseFileRefs(value);
        if (segments.length === 1 && typeof segments[0] === "string") return null;

        const parts: Array<Element | Text> = [];
        for (const segment of segments) {
          if (typeof segment === "string") {
            parts.push({ type: "text", value: segment });
            continue;
          }
          parts.push({
            type: "element",
            tagName: "a",
            properties: { dataFileRef: segment.path, dataFileLine: segment.line },
            children: [
              {
                type: "text",
                value: segment.line > 0 ? `${segment.path}:${segment.line}` : segment.path,
              },
            ],
          });
        }
        return parts;
      },
    );
  };
}
