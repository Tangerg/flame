import { fireEvent, render, renderHook, within } from "@testing-library/react";
import { beforeAll, describe, expect, it } from "vitest";
import { getHighlighter } from "@/lib/highlight/shiki";
import { useWorkspaceFileViewer } from "@/plugins/builtin/workspace/public/navigation";
import { MarkdownRenderer } from "./MarkdownRenderer";
import { markdownSelectionPayload } from "./markdownSelectionCopy";

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
      const { container, unmount } = render(<MarkdownRenderer text={markdown} reveal="instant" />);

      const own = within(container).queryAllByRole("button").length;
      controls += own;
      if (linked && own === 0) wrong.push(`${name}: the path was not made openable`);
      if (!linked && own > 0)
        wrong.push(`${name}: a path that was already something got a control`);

      const insideAnchor = container.querySelectorAll('a button, a [role="button"], a a').length;
      if (insideAnchor > 0) {
        wrong.push(`${name}: ${insideAnchor} control(s) nested inside a link`);
      }

      unmount();
    }

    expect(controls, "the sweep has to produce real file references").toBeGreaterThan(1);
    expect(wrong, "file references in the wrong place, or missing").toEqual([]);
  });

  it("keeps the visible and copied column while opening the existing line target", () => {
    const viewer = renderHook(useWorkspaceFileViewer);
    const text = "在 src/main.ts:12:3 中检查这一处边界。";
    const { container } = render(<MarkdownRenderer text={text} reveal="instant" />);
    const reference = within(container).getByRole("button", { name: "src/main.ts:12:3" });
    const paragraph = container.querySelector("p")!;
    expect(paragraph.textContent).toBe(text);

    const range = document.createRange();
    range.selectNodeContents(paragraph);
    const selection = document.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    try {
      expect(markdownSelectionPayload(container, selection)?.plainText).toBe(text);
    } finally {
      selection.removeAllRanges();
    }

    fireEvent.click(reference);
    expect(viewer.result.current).toEqual({ path: "src/main.ts", line: 12 });
  });

  it("opens sentence-ending paths without consuming their punctuation", () => {
    const viewer = renderHook(useWorkspaceFileViewer);
    const text = "Inspect desktop/frontend/src/main.ts. Then review src/main.ts.";
    const { container } = render(<MarkdownRenderer text={text} reveal="instant" />);
    const references = within(container).getAllByRole("button");
    expect(references.map((reference) => reference.textContent)).toEqual([
      "desktop/frontend/src/main.ts",
      "src/main.ts",
    ]);
    expect(container.querySelector("p")?.textContent).toBe(text);

    fireEvent.click(references[0]!);
    expect(viewer.result.current).toEqual({
      path: "desktop/frontend/src/main.ts",
      line: 0,
    });
    fireEvent.click(references[1]!);
    expect(viewer.result.current).toEqual({ path: "src/main.ts", line: 0 });
  });
});
