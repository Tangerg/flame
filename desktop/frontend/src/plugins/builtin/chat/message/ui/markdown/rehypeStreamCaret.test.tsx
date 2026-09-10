import { act, render } from "@testing-library/react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { getHighlighter } from "@/lib/highlight/shiki";
import { MarkdownMessage } from "./MarkdownMessage";

// The caret marks where the next character will land, so the contract is one sentence: nothing
// the reader can see comes after it.
//
// Asserted that way rather than as "its parent is <p>", because the right parent depends on the
// content — a message ending in bold SHOULD put the caret inside the `<strong>`, and one ending
// in a code fence should put it after the block. Only "there is nothing to my right" is true of
// all of them, and it is also the thing that was wrong: the plugin used to search BACKWARDS for
// the last element and descend into it, so a paragraph ending in plain text sent the caret into
// whichever emphasis, link or code span came before that text. Measured in the rendered DOM,
// `Hello **world** and more text` put it inside `<strong>` with " and more text" to its right.
//
// Rendered through `MarkdownMessage` rather than run against a hand-built tree, because the
// whitespace nodes `remark-rehype` inserts between blocks are exactly what the descent has to
// see past, and a tree written by hand does not have them.
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

/** The block a caret sits in: what "after the caret" is measured against. */
const BLOCK = "p,li,h1,h2,h3,h4,h5,h6,blockquote,dd,dt,figcaption,td,th";

/** Frames enough for the reveal to hand over every case's text. */
const FRAMES = 300;

describe("rehypeStreamCaret", () => {
  beforeAll(async () => {
    // The code-fence case renders through Shiki, which is an application-lifetime singleton.
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
      // Floor: no caret means the assertion below is about nothing. The caret is only added in
      // `typewriter` mode while streaming, which is what these cases render.
      expect(caret, `${name}: the stream caret was not rendered`).not.toBeNull();

      // To the end of the BLOCK the caret is in, not of its immediate parent. That distinction
      // is the whole assertion: with the defect the caret's parent was the `<strong>` it had
      // descended into, which has nothing after it — the stranded text is a SIBLING of that
      // element. Measured against the parent, this test passed on the broken implementation.
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
