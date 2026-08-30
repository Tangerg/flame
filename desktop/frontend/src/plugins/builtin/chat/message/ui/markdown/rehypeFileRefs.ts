import type { Element, Root, Text } from "hast";
import { visit } from "unist-util-visit";
import { parseFileRefs } from "@/plugins/builtin/agent/public/fileRefs";

const SKIP_TAGS = new Set(["pre", "code", "a", "sup", "script", "style"]);

export function rehypeFileRefs() {
  return (tree: Root) => {
    interface Job {
      parent: Element | Root;
      index: number;
      replacement: Array<Element | Text>;
    }
    const jobs: Job[] = [];

    visit(tree, "text", (node: Text, index, parent) => {
      if (index === undefined || parent === undefined) return;
      if (parent.type === "element" && SKIP_TAGS.has(parent.tagName)) return;

      const segments = parseFileRefs(node.value);
      if (segments.length === 1 && typeof segments[0] === "string") return;

      const parts: Array<Element | Text> = [];
      for (const seg of segments) {
        if (typeof seg === "string") {
          parts.push({ type: "text", value: seg });
          continue;
        }
        parts.push({
          type: "element",
          tagName: "a",
          properties: { dataFileRef: seg.path, dataFileLine: seg.line },
          children: [{ type: "text", value: seg.line > 0 ? `${seg.path}:${seg.line}` : seg.path }],
        });
      }
      jobs.push({ parent: parent as Element | Root, index, replacement: parts });
    });

    for (let i = jobs.length - 1; i >= 0; i--) {
      const { parent, index, replacement } = jobs[i]!;
      parent.children.splice(index, 1, ...replacement);
    }
  };
}
