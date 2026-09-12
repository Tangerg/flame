import { render } from "@testing-library/react";
import { beforeAll, describe, expect, it } from "vitest";
import { getHighlighter } from "@/lib/highlight/shiki";
import { MarkdownMessage } from "./MarkdownMessage";

const CASES: readonly [name: string, markdown: string, linked: boolean][] = [
  ["a bare path", "Edited src/foo.ts:12 today", true],
  ["a bare path in bold", "Edited **src/foo.ts** today", true],
  ["a path that is the link text", "See [src/foo.ts](https://example.test) now", false],
  ["a path bold inside a link", "See [**src/foo.ts**](https://example.test) now", false],
  ["a path italic inside a link", "See [*src/foo.ts*](https://example.test) now", false],
  ["a path in a code span inside a link", "See [`src/foo.ts`](https://example.test) now", false],
  ["a path in a code span", "Edited `src/foo.ts` today", false],
];

describe("rehypeFileRefs", () => {
  beforeAll(async () => {
    await getHighlighter();
  });

  it("turns a mentioned path into one control, and never one inside a link", () => {
    const wrong: string[] = [];
    let controls = 0;

    for (const [name, markdown, linked] of CASES) {
      const { container, unmount } = render(<MarkdownMessage text={markdown} reveal="instant" />);

      const own = container.querySelectorAll("button").length;
      controls += own;
      if (linked && own === 0) wrong.push(`${name}: the path was not made openable`);
      if (!linked && own > 0)
        wrong.push(`${name}: a path that was already something got a control`);

      const insideAnchor = container.querySelectorAll("a button, a a").length;
      if (insideAnchor > 0) {
        wrong.push(`${name}: ${insideAnchor} control(s) nested inside a link`);
      }

      unmount();
    }

    expect(controls, "the sweep has to produce real file references").toBeGreaterThan(1);
    expect(wrong, "file references in the wrong place, or missing").toEqual([]);
  });
});
