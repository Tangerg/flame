import { type Element, type Root, type Text } from "hast";
import {
  formatFileRef,
  parseFileRefs,
  parseInlineFileRef,
  type FileRef,
} from "@/features/agent/public/fileRefs";
import { rewriteTextNodes } from "@/features/chat/message/ui/markdown/rewriteTextNodes";
const SKIP_TAGS = new Set(["pre", "a", "sup", "script", "style"]);

function referenceElement(ref: FileRef, label: string): Element {
  return {
    type: "element",
    tagName: "a",
    properties: {
      dataFileRef: ref.path,
      dataFileLine: ref.line,
    },
    children: [{ type: "text", value: label }],
  };
}

export function rehypeFileRefs() {
  return (tree: Root) => {
    rewriteTextNodes(
      tree,
      (node) =>
        SKIP_TAGS.has(node.tagName) ||
        (node.tagName === "code" &&
          (Object.keys(node.properties).length > 0 ||
            node.children.length !== 1 ||
            node.children[0]?.type !== "text")),
      (value, parent) => {
        if (parent.type === "element" && parent.tagName === "code") {
          const ref = parseInlineFileRef(value);
          return ref ? [referenceElement(ref, value)] : null;
        }

        const segments = parseFileRefs(value);
        if (segments.length === 1 && typeof segments[0] === "string") return null;

        const parts: Array<Element | Text> = [];
        for (const segment of segments) {
          if (typeof segment === "string") {
            parts.push({ type: "text", value: segment });
            continue;
          }
          parts.push(referenceElement(segment, formatFileRef(segment)));
        }
        return parts;
      },
    );
  };
}
