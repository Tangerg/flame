import { describe, expect, it } from "vitest";
import { unified } from "unified";
import remarkParse from "remark-parse";
import { visit } from "unist-util-visit";
import { remarkMathSyntax } from "./remarkMathSyntax";
import { forEachSeed } from "@/test/arbitrary";

const parser = unified().use(remarkParse).use(remarkMathSyntax).freeze();

function formulas(text: string) {
  const values: { type: string; value: string }[] = [];
  visit(parser.parse(text), (node) => {
    if (node.type === "math" || node.type === "inlineMath")
      values.push({ type: node.type, value: node.value });
  });
  return values;
}

describe("Markdown math syntax", () => {
  it.each([String.raw`\( x^2 \)`, String.raw`\\( x^2 \\)`, "[/inline] x^2 [/inline]", "$x^2$"])(
    "parses inline syntax %s without rewriting source",
    (source) => {
      expect(formulas(`before ${source} after`)).toEqual([{ type: "inlineMath", value: "x^2" }]);
    },
  );

  it.each([String.raw`\[ x^2 \]`, String.raw`\\[ x^2 \\]`, "[/math] x^2 [/math]", "$$\nx^2\n$$"])(
    "parses display syntax %s",
    (source) => {
      expect(formulas(source)).toEqual([{ type: "math", value: "x^2" }]);
    },
  );

  it.each(["\\[\nx^2\n\n+ y^2\n\\]", "[/math]\nx^2\n\n+ y^2\n[/math]"])(
    "keeps blank lines inside display math",
    (source) => {
      expect(formulas(source)).toEqual([{ type: "math", value: "x^2\n\n+ y^2" }]);
    },
  );

  it("keeps currency literal without stealing a subsequent formula", () => {
    expect(formulas("Pay $5 for $x^2$, then $19.99 or $1,299.")).toEqual([
      { type: "inlineMath", value: "x^2" },
    ]);
    expect(formulas(String.raw`\$5 \\$5 $5 $$5x$$ \(5x\)`)).toEqual([
      { type: "inlineMath", value: "5x" },
      { type: "inlineMath", value: "5x" },
    ]);
  });

  it("retains container ownership of nested display math", () => {
    expect(formulas("> \\[\n> x^2\n> \\]")).toEqual([{ type: "math", value: "x^2" }]);
    expect(formulas("- Example\n\n  \\[\n  x^2\n  \\]")).toEqual([{ type: "math", value: "x^2" }]);
  });

  it("does not admit incomplete or multiline inline delimiters", () => {
    expect(formulas(String.raw`before \(x^2`)).toEqual([]);
    expect(formulas("\\(x\ny\\)")).toEqual([]);
    expect(formulas("[/inline]x\ny[/inline]")).toEqual([]);
  });

  it("does not extend a formula beyond its quote or list container", () => {
    expect(formulas("> \\[\n> x\n\noutside \\]")).toEqual([]);
    expect(formulas("- \\[\n  x\n\noutside \\]")).toEqual([]);
  });

  it("preserves arbitrary Unicode code material through the parser", () => {
    forEachSeed(600, (random) => {
      const source = random.text().replaceAll("\0", "\uFFFD");
      const node = parser.parse(`~~~text\n${source}\n~~~`).children[0];
      expect(node).toMatchObject({ type: "code", value: source });
    });
  });
});
