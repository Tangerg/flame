import { act, render } from "@testing-library/react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { getHighlighter } from "@/lib/highlight/shiki";
import { MarkdownMessage } from "./MarkdownMessage";

const CASES: readonly [name: string, markdown: string][] = [
  ["plain text", "Hello world here"],
  ["bold, then more text", "Hello **world** and more text"],
  ["link, then more text", "See [docs](https://example.test) for more text"],
  ["code span, then more text", "Run `npm test` before pushing now"],
  ["ends with bold", "Hello and **world**"],
  ["ends with a code fence", "look:\n\n```\nx = 1\n```"],
  ["list whose last item is text", "- one\n- two and more text"],
  ["heading, then a paragraph", "# Title\n\nBody text here"],
];

const BLOCK = "p,li,h1,h2,h3,h4,h5,h6,blockquote,dd,dt,figcaption,td,th";

const FRAMES = 300;

describe("rehypeStreamCaret", () => {
  beforeAll(async () => {
    await getHighlighter();
  });

  it("leaves nothing visible to the right of the caret", () => {
    vi.useFakeTimers({
      toFake: [
        "requestAnimationFrame",
        "cancelAnimationFrame",
        "performance",
        "setTimeout",
        "clearTimeout",
      ],
    });

    const trailing: string[] = [];
    for (const [name, markdown] of CASES) {
      const { container, unmount } = render(
        <MarkdownMessage text={markdown} streaming reveal="typewriter" />,
      );
      for (let frame = 0; frame < FRAMES; frame += 1) {
        act(() => void vi.advanceTimersByTime(16));
      }

      const caret = container.querySelector(".type-caret");
      expect(caret, `${name}: the stream caret was not rendered`).not.toBeNull();

      const block = caret!.closest(BLOCK) ?? container;
      const after = document.createRange();
      after.setStartAfter(caret!);
      after.setEnd(block, block.childNodes.length);
      if (after.toString() !== "") trailing.push(`${name}: ${JSON.stringify(after.toString())}`);

      unmount();
    }
    vi.useRealTimers();

    expect(trailing, "text stranded to the right of the caret").toEqual([]);
  });
});
