import { render } from "@testing-library/react";
import { beforeAll, describe, expect, it } from "vitest";
import { getHighlighter } from "@/lib/highlight/shiki";
import { MarkdownMessage } from "./MarkdownMessage";

// A path the agent mentions becomes something to open. Where it is ALREADY something — a link,
// a code span, a footnote marker — it stays what it is.
//
// The failure this pins is the second half of that, and it only showed up through an
// intermediate element: `[src/foo.ts](url)` was skipped because the text's parent is the `<a>`,
// while `[**src/foo.ts**](url)` was not, because its parent is the `<strong>` in between. What
// came out was a control nested inside an `<a target="_blank">` — invalid HTML, two tab stops
// where the reader sees one word, and a click that both opens the file and follows the link.
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

      // The reference renders as a control, not as an anchor — the viewer it opens is this
      // app's, not the browser's.
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

    // Floor, not a target: a run that produced no references agrees with any skip list at all.
    expect(controls, "the sweep has to produce real file references").toBeGreaterThan(1);
    expect(wrong, "file references in the wrong place, or missing").toEqual([]);
  });
});
