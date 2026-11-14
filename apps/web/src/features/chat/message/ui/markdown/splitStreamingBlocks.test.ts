import { describe, expect, it, vi } from "vitest";
import { unified } from "unified";
import remarkParse from "remark-parse";
import { remarkSyntax } from "./remarkSyntax";
import { createStreamingBlockSplitter, splitStreamingBlocks } from "./splitStreamingBlocks";
const CASES: readonly [name: string, markdown: string, blocks: readonly string[]][] = [
  ["two paragraphs", "alpha\n\nbeta", ["alpha", "\n\n", "beta"]],
  [
    "bracket display math owns its blank lines",
    "before\n\n\\[\nx^2\n\n+ y^2\n\\]\n\nafter",
    ["before", "\n\n", "\\[\nx^2\n\n+ y^2\n\\]", "\n\n", "after"],
  ],
  ["an unclosed fence keeps what follows", "text\n\n```js\ncode", ["text", "\n\n", "```js\ncode"]],
  [
    "a closed fence ends its block",
    "text\n\n```js\ncode\n```\n\nafter",
    ["text", "\n\n", "```js\ncode\n```", "\n\n", "after"],
  ],
  [
    "a block element spans every token until it closes",
    "<div>\n\npara\n\n</div>\n\nafter",
    ["<div>\n\npara\n\n</div>", "\n\n", "after"],
  ],
  [
    "an odd display fence keeps what follows",
    "text\n\n$$\nx=1\n\nmore",
    ["text", "\n\n", "$$\nx=1\n\nmore"],
  ],
  [
    "an even display fence ends its block",
    "$$\nx=1\n$$\n\nafter",
    ["$$\nx=1\n$$", "\n\n", "after"],
  ],
  ["a footnote reference keeps the document whole", "see[^a]\n\nbeta", ["see[^a]\n\nbeta"]],
  ["a footnote definition keeps the document whole", "[^a]: note\n\nbeta", ["[^a]: note\n\nbeta"]],
  [
    "a non-Latin footnote label keeps its definition with the reference",
    "see[^来源]\n\n[^来源]: supporting evidence",
    ["see[^来源]\n\n[^来源]: supporting evidence"],
  ],
  [
    "a link definition keeps the reference and document together",
    '[guide][id]\n\nA later paragraph.\n\n[id]: https://example.test/protocol "Protocol guide"',
    ['[guide][id]\n\nA later paragraph.\n\n[id]: https://example.test/protocol "Protocol guide"'],
  ],
  [
    "direct links and fenced definitions retain ordinary block splitting",
    "[guide](https://example.test)\n\n```md\n[guide][id]\n[id]: https://example.test/literal\n```",
    [
      "[guide](https://example.test)",
      "\n\n",
      "```md\n[guide][id]\n[id]: https://example.test/literal\n```",
    ],
  ],
  ["a void element opens nothing", "<img src=x>\n\npara", ["<img src=x>", "\n\n", "para"]],
  ["a self-closing element opens nothing", "<div />\n\npara", ["<div />", "\n\n", "para"]],
  [
    "nesting pops one level at a time",
    "<div>\n<div>\nnested\n</div>\n</div>\n\nend",
    ["<div>\n<div>\nnested\n</div>\n</div>", "\n\n", "end"],
  ],
  [
    "a fence is literal text, not a math fence",
    "```\n$$\n```\n\n$$\ny\n$$\n\nz",
    ["```\n$$\n```", "\n\n", "$$\ny\n$$", "\n\n", "z"],
  ],
];

describe("splitStreamingBlocks", () => {
  for (const [name, markdown, blocks] of CASES)
    it(`${name}`, () => {
      const result = splitStreamingBlocks(markdown);
      expect(result).toEqual([...blocks]);
      expect(result.join("")).toBe(markdown);
    });

  it("leaves settled blocks byte-identical as the tail grows", () => {
    const document = "# Title\n\nfirst para\n\n```js\nconst a = 1;\n```\n\nlast para";
    const rewritten: string[] = [];
    let settled: string[] = [];
    for (let end = 1; end <= document.length; end += 1) {
      const prefix = splitStreamingBlocks(document.slice(0, end)).slice(0, -1);
      const kept = prefix.slice(0, settled.length);
      if (prefix.length >= settled.length && kept.join("\u0000") !== settled.join("\u0000"))
        rewritten.push(`at ${end}: ${JSON.stringify(settled)} became ${JSON.stringify(kept)}`);
      settled = prefix;
    }
    expect(rewritten).toEqual([]);
    expect(splitStreamingBlocks(document).at(-1)).toBe("last para");
  });
});

describe("incremental streaming block ownership", () => {
  const parser = unified().use(remarkParse).use(remarkSyntax);
  const semantics = (blocks: string[]) =>
    JSON.stringify(
      {
        type: "root",
        children: blocks.flatMap((block) => parser.parse(block).children),
      },
      (key, value) => (key === "position" ? undefined : value),
    );

  it.each([
    "# Heading\n\nA paragraph.\n\nA second paragraph.\n\nlast tail",
    "A title\n---\n\n- one\n\n  next paragraph\n- two\n\nlast tail",
    "before\n\n| Left | Right |\n| --- | --- |\n| one | two |\n\nafter",
    "before\n\n```md\n[^literal]: ignored\n\ninside\n```\n\nafter",
    "before\n\n<div>\n\n<p>inside</p>\n\n</div>\n\nafter",
    "[late][guide]\n\nA later paragraph.\n\n[guide]: https://example.test",
    "A supported claim[^来源].\n\nother paragraph\n\n[^来源]: supporting note",
    "before\n\n\\[\nx^2\n\n+ y^2\n\\]\n\nafter",
    "before\n\n$$\nx^2\n\n+ y^2\n$$\n\nafter",
  ])("keeps complete-document semantics through appends: %s", (source) => {
    const split = createStreamingBlockSplitter();
    for (let end = 1; end <= source.length; end += 3) {
      const prefix = source.slice(0, end);
      const blocks = split(prefix);
      expect(blocks.join("")).toBe(prefix);
      expect(semantics(blocks)).toBe(semantics([prefix]));
    }
    expect(semantics(split(source))).toBe(semantics([source]));
  });

  it("parses only the mutable tail after committing a long prefix", () => {
    const split = createStreamingBlockSplitter();
    const prefix = Array.from(
      { length: 30 },
      (_, index) => `# Section ${index}\n\nParagraph ${index}.\n\n`,
    ).join("");
    split(`${prefix}tail`);
    const parse = vi.spyOn(Object.getPrototypeOf(unified()), "parse");
    try {
      const result = split(`${prefix}tail grows`);
      expect(result.join("")).toBe(`${prefix}tail grows`);
      expect(parse).toHaveBeenCalledOnce();
      expect(String(parse.mock.calls[0]![0])).toBe("\n\ntail grows");
    } finally {
      parse.mockRestore();
    }
  });

  it("retires cached prefixes after a rollback or replacement and repairs only the current tail", () => {
    const split = createStreamingBlockSplitter();
    const repair = vi.fn((tail: string) => tail);
    split("# Original\n\nFirst.\n\nTail", repair);
    const next = split("# Original\n\nFirst.\n\nTail grows", repair);
    expect(next.join("")).toBe("# Original\n\nFirst.\n\nTail grows");
    expect(repair.mock.lastCall![0]).not.toContain("# Original");
    expect(split("# Original", repair).join("")).toBe("# Original");
    expect(split("A replacement\n\n[link][id]\n\n[id]: https://example.test", repair)).toEqual([
      "A replacement\n\n[link][id]\n\n[id]: https://example.test",
    ]);
    expect(split("New plain reply")).toEqual(["New plain reply"]);
  });
});
