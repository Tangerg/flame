import { type Element, type Root, type Text } from "hast";
import { formatFileRef, parseFileRefs } from "@/features/agent/public/fileRefs";
import { rewriteTextNodes } from "@/features/chat/message/ui/markdown/rewriteTextNodes";
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
            properties: {
              dataFileRef: segment.path,
              dataFileLine: segment.line,
              dataFileColumn: segment.column,
            },
            children: [{ type: "text", value: formatFileRef(segment) }],
          });
        }
        return parts;
      },
    );
  };
}
