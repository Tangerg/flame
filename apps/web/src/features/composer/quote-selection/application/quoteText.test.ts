import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ReactMarkdown from "react-markdown";
import remarkBreaks from "remark-breaks";
import remarkGfm from "remark-gfm";
import { describe, expect, it } from "vitest";
import {
  quoteText,
  type QuoteSource,
} from "@/features/composer/quote-selection/application/quoteText";
describe("quoteText", () => {
  it("names the file and the lines a selection came from", () => {
    expect(quoteText({ kind: "file", path: "src/a.ts", lines: [10, 20] }, "const a = 1;\n")).toBe(
      "`src/a.ts:10-20`\n```\nconst a = 1;\n```\n\n",
    );
  });

  it("says a single line once", () => {
    expect(quoteText({ kind: "file", path: "a.go", lines: [4, 4] }, "x")).toContain("`a.go:4`");
  });

  it("quotes prose line by line, keeping blank lines inside the quote", () => {
    expect(quoteText({ kind: "message" }, "first\n\nsecond")).toBe("> first\n>\n> second\n\n");
  });

  it("fences tool output so it stays verbatim", () => {
    expect(quoteText({ kind: "tool-output" }, "FAIL x")).toBe("```\nFAIL x\n```\n\n");
  });

  it.each<QuoteSource>([
    { kind: "message" },
    { kind: "file", path: "src/a.ts", lines: [4, 5] },
    { kind: "tool-output" },
  ])("renders immediate continuation as a paragraph after the $kind quote", (source) => {
    const rendered = document.createElement("div");
    rendered.innerHTML = renderToStaticMarkup(
      createElement(
        ReactMarkdown,
        { remarkPlugins: [remarkGfm, remarkBreaks] },
        `${quoteText(source, "Selected evidence.")}Please explain this.`,
      ),
    );

    expect(rendered.lastElementChild?.tagName).toBe("P");
    expect(rendered.lastElementChild?.textContent).toBe("Please explain this.");
    expect(rendered.lastElementChild?.previousElementSibling?.tagName).toBe(
      source.kind === "message" ? "BLOCKQUOTE" : "PRE",
    );
    expect(rendered.querySelector("blockquote, pre")?.textContent).toContain("Selected evidence.");
    expect(rendered.querySelector("blockquote, pre")?.textContent).not.toContain(
      "Please explain this.",
    );
  });
});
