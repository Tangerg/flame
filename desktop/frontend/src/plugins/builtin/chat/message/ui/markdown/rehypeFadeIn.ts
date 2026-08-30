import type { Element, Root, Text } from "hast";
import { visit } from "unist-util-visit";
import { segmentWords } from "@/lib/i18n/segmentWords";

const SKIP_TAGS = new Set(["pre", "code", "script", "style"]);

export function rehypeFadeIn() {
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
      if (parent.type === "element") {
        const props = parent.properties ?? {};
        if (props.dataNoFade || props["data-no-fade"]) return;
      }

      const value = node.value;
      if (!value) return;

      const segments = segmentWords(value);
      if (segments.every((s) => /^\s+$/.test(s))) return;

      const replacement: Array<Element | Text> = segments.map((seg) => {
        if (/^\s+$/.test(seg)) {
          return { type: "text", value: seg } satisfies Text;
        }
        return {
          type: "element",
          tagName: "span",
          properties: { className: ["fade-in"] },
          children: [{ type: "text", value: seg }],
        } satisfies Element;
      });

      jobs.push({ parent: parent as Element | Root, index, replacement });
    });

    for (let i = jobs.length - 1; i >= 0; i--) {
      const { parent, index, replacement } = jobs[i]!;
      parent.children.splice(index, 1, ...replacement);
    }
  };
}
