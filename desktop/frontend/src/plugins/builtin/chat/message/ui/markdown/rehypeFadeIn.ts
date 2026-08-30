// Stable hast positions across renders are what keep already-rendered spans from replaying,
// so only the freshly-appended tail animates. Skips <pre>/<code>, which Shiki handles as one
// chunk, and anything under `data-no-fade`.

import type { Element, Root, Text } from "hast";
import { visit } from "unist-util-visit";
import { segmentWords } from "@/lib/i18n/segmentWords";

const SKIP_TAGS = new Set(["pre", "code", "script", "style"]);

export function rehypeFadeIn() {
  return (tree: Root) => {
    // Collect work in a first pass so we don't mutate while visiting.
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
        // `visit()` gives no ancestor chain, so the opt-out marker is only honoured on the
        // IMMEDIATE parent.
        const props = parent.properties ?? {};
        if (props.dataNoFade || props["data-no-fade"]) return;
      }

      const value = node.value;
      if (!value) return;

      const segments = segmentWords(value);
      // All-whitespace has no visible effect to animate; wrapping it only bloats the DOM.
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

    // Apply in reverse so indices stay valid as we splice.
    for (let i = jobs.length - 1; i >= 0; i--) {
      const { parent, index, replacement } = jobs[i]!;
      parent.children.splice(index, 1, ...replacement);
    }
  };
}
