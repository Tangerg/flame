import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});

import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FileView } from "@/features/workspace/ui/files/FileView";
const projection = vi.hoisted(() => {
  const highlighter = {
    codeToHtml: vi.fn<(code: string) => string>(
      () => "<pre><code><span>highlighted</span></code></pre>",
    ),
    getLoadedLanguages: vi.fn(() => ["go", "text"]),
  };
  return { highlighter, currentHighlighter: highlighter as typeof highlighter | null };
});

vi.mock("@/lib/highlight/useCodeHighlight", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/highlight/useCodeHighlight")>()),
  useCodeHighlighter: () => ({ highlighter: projection.currentHighlighter, theme: "test-theme" }),
}));

const INTENT = {};

let nativeScrollIntoView: typeof HTMLElement.prototype.scrollIntoView | undefined;

beforeEach(() => {
  projection.currentHighlighter = projection.highlighter;
  projection.highlighter.codeToHtml.mockClear();
  projection.highlighter.getLoadedLanguages.mockClear();
  nativeScrollIntoView = HTMLElement.prototype.scrollIntoView;
});

afterEach(() => {
  if (nativeScrollIntoView) HTMLElement.prototype.scrollIntoView = nativeScrollIntoView;
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
});

describe("FileView syntax ownership", () => {
  it("highlights a file in the language determined by its path", () => {
    const content = "package main\nfunc main() {}";

    testClient.render(
      <FileView
        path="cmd/main.go"
        content={content}
        startLine={1}
        targetLine={0}
        intent={INTENT}
      />,
    );

    expect(projection.highlighter.codeToHtml).toHaveBeenCalledWith(content, {
      lang: "go",
      theme: "test-theme",
    });
  });

  it("does not replay the target-line navigation when Shiki materializes", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    projection.currentHighlighter = null;
    const content = "package main\nfunc main() {}";
    const view = testClient.render(
      <FileView
        path="cmd/main.go"
        content={content}
        startLine={1}
        targetLine={2}
        intent={INTENT}
      />,
    );
    expect(scrollIntoView).toHaveBeenCalledTimes(1);

    projection.currentHighlighter = projection.highlighter;
    view.rerender(
      <FileView
        path="cmd/main.go"
        content={content}
        startLine={1}
        targetLine={2}
        intent={INTENT}
      />,
    );

    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("keeps gutter and target identity for a window from the middle of a file", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    const view = testClient.render(
      <FileView
        path="cmd/main.go"
        content={"first\nsecond"}
        startLine={40}
        targetLine={41}
        intent={INTENT}
      />,
    );

    expect(view.getByText("40")).toBeTruthy();
    expect(view.getByText("41")).toBeTruthy();
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("keeps the reader's place when the same file refreshes", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    const intent = {};
    const view = testClient.render(
      <FileView path="a.go" content={"one\ntwo"} startLine={1} targetLine={2} intent={intent} />,
    );
    view.rerender(
      <FileView
        path="a.go"
        content={"one\ntwo\nthree"}
        startLine={1}
        targetLine={2}
        intent={intent}
      />,
    );
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("leaves placeholder content unplaced and places the line once its own content arrives", () => {
    const placed: (string | null)[] = [];
    HTMLElement.prototype.scrollIntoView = function (this: HTMLElement) {
      placed.push(this.getAttribute("data-quote-line"));
    };
    const view = testClient.render(
      <FileView path="a.go" content={"a\nb\nc"} startLine={1} targetLine={3} intent={{}} />,
    );
    expect(placed).toEqual(["3"]);

    view.rerender(
      <FileView path="a.go" content={"a\nb\nc"} startLine={1} targetLine={2} intent={null} />,
    );
    expect(placed).toEqual(["3"]);

    view.rerender(
      <FileView path="a.go" content={"x\ny\nz"} startLine={2} targetLine={2} intent={{}} />,
    );
    expect(placed).toEqual(["3", "2"]);
  });

  describe("source line text", () => {
    const SOURCE =
      'package main\n\n// 核对空白与缩进\nfunc main() {\n\tmessage := "保留  double spaces"\n\tprintln(message)\n\n}\n\n';

    function codeCells(root: HTMLElement) {
      return Array.from(
        root.querySelectorAll("[data-quote-line]"),
        (row) => row.lastElementChild!.textContent,
      );
    }

    it.each([
      { label: "plain", highlighted: false },
      { label: "highlighted", highlighted: true },
    ])(
      "keeps blank lines, the trailing newline and indentation in $label code cells",
      ({ highlighted }) => {
        if (highlighted) {
          projection.highlighter.codeToHtml.mockImplementationOnce(
            (code: string) =>
              `<pre><code>${code
                .split("\n")
                .map((line) => `<span class="line"><span>${line}</span></span>`)
                .join("\n")}</code></pre>`,
          );
        } else {
          projection.currentHighlighter = null;
        }
        const source = createRef<HTMLDivElement>();
        const view = testClient.render(
          <FileView
            ref={source}
            path="src/review.go"
            content={SOURCE}
            startLine={1}
            targetLine={0}
            intent={INTENT}
          />,
        );

        const cells = codeCells(view.container);
        expect(cells).toHaveLength(SOURCE.split("\n").length);
        expect(cells.join("")).toBe(SOURCE);
        expect(cells[1]).toBe("\n");
        expect(cells[4]).toBe('\tmessage := "保留  double spaces"\n');
        expect(cells.at(-1)).toBe("");
        expect(source.current?.getAttribute("data-quote-source")).toBe("file");
        expect(source.current?.querySelectorAll("[data-quote-line]")).toHaveLength(cells.length);
        expect(Boolean(view.container.querySelector('[data-quote-line="5"] .line'))).toBe(
          highlighted,
        );
      },
    );
  });

  it("places the line again when the same line is asked for again", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    const view = testClient.render(
      <FileView path="a.go" content={"one\ntwo"} startLine={1} targetLine={2} intent={{}} />,
    );
    view.rerender(
      <FileView path="a.go" content={"one\ntwo"} startLine={1} targetLine={2} intent={{}} />,
    );
    expect(scrollIntoView).toHaveBeenCalledTimes(2);
  });
});
