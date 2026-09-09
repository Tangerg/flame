import { describe, expect, it } from "vitest";
import { splitStreamingBlocks } from "./splitStreamingBlocks";

/**
 * Pinned against the function this replaces, not written from a reading of it.
 *
 * Every expectation below is the output `streamdown`'s `parseMarkdownIntoBlocks` gave for the
 * same input, captured before the swap. A differential fuzz over 4000 generated documents
 * found no other disagreement — three of these cases are ones it DID find, and each named a
 * rule a reading would have missed: a void element opens nothing, a self-closing element opens
 * nothing, and a tag with no `>` yet is not a tag at all.
 *
 * Written as literals rather than as a comparison, because the package they came from is gone.
 */
const CASES: readonly [name: string, markdown: string, blocks: readonly string[]][] = [
  ["two paragraphs", "alpha\n\nbeta", ["alpha", "\n\n", "beta"]],
  ["an unclosed fence keeps what follows", "text\n\n```js\ncode", ["text", "\n\n", "```js\ncode"]],
  [
    "a closed fence ends its block",
    "text\n\n```js\ncode\n```\n\nafter",
    ["text", "\n\n", "```js\ncode\n```", "\n\n", "after"],
  ],
  [
    "a block element spans every token until it closes",
    "<div>\n\npara\n\n</div>\n\nafter",
    ["<div>\n\npara\n\n</div>\n\n", "after"],
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
  ["a void element opens nothing", "<img src=x>\n\npara", ["<img src=x>\n\n", "para"]],
  ["a self-closing element opens nothing", "<div />\n\npara", ["<div />\n\n", "para"]],
  [
    "nesting pops one level at a time",
    "<div>\n<div>\nnested\n</div>\n</div>\n\nend",
    ["<div>\n<div>\nnested\n</div>\n</div>\n\n", "end"],
  ],
  [
    "a fence is literal text, not a math fence",
    "```\n$$\n```\n\n$$\ny\n$$\n\nz",
    ["```\n$$\n```", "\n\n", "$$\ny\n$$", "\n\n", "z"],
  ],
];

describe("splitStreamingBlocks", () => {
  for (const [name, markdown, blocks] of CASES)
    it(`${name}`, () => expect(splitStreamingBlocks(markdown)).toEqual([...blocks]));

  // The reason the function exists at all: only the last block may still be growing, so every
  // block before it has to be the same string it was on the previous keystroke. Split a
  // document one character at a time and the settled prefix must never be rewritten.
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
