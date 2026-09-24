import { describe, expect, it } from "vitest";
import { quoteText } from "./quoteText";

describe("quoteText", () => {
  it("names the file and the lines a selection came from", () => {
    expect(quoteText({ kind: "file", path: "src/a.ts", lines: [10, 20] }, "const a = 1;\n")).toBe(
      "`src/a.ts:10-20`\n```\nconst a = 1;\n```",
    );
  });

  it("says a single line once", () => {
    expect(quoteText({ kind: "file", path: "a.go", lines: [4, 4] }, "x")).toContain("`a.go:4`");
  });

  it("quotes prose line by line, keeping blank lines inside the quote", () => {
    expect(quoteText({ kind: "message" }, "first\n\nsecond")).toBe("> first\n>\n> second");
  });

  it("fences tool output so it stays verbatim", () => {
    expect(quoteText({ kind: "tool-output" }, "FAIL x")).toBe("```\nFAIL x\n```");
  });
});
