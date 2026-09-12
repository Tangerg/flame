import { Lexer } from "marked";

/**
 * Only the LAST block changes while a message streams, so splitting is what lets React leave
 * the settled ones alone — and what makes the split correct is not the cutting but the three
 * places a block legitimately spans what looks like a boundary.
 *
 * This replaces `streamdown`'s `parseMarkdownIntoBlocks`, which was the only thing this
 * product imported from that package. It ships no `sideEffects` flag, so the bundler has to
 * assume its module graph is live: a second markdown pipeline — `unified`, `remark-rehype`,
 * `rehype-sanitize`, `rehype-harden` — plus `tailwind-merge`, whose entire subject is
 * resolving Tailwind class conflicts, in a product with no Tailwind classes. Measured against
 * a stubbed import, that one function cost **139.3 KB of the entry chunk**, the one parsed
 * before first paint, at 91.4% of its budget. `marked` is what streamdown lexes with too, and
 * it is MIT with zero dependencies.
 *
 * `splitStreamingBlocks.test.ts` pins every case against the output the old function gave.
 */
const FOOTNOTE_REFERENCE = /\[\^[\w-]{1,200}\](?!:)/;
const FOOTNOTE_DEFINITION = /\[\^[\w-]{1,200}\]:/;
const HTML_BLOCK_TAG = /<([A-Za-z][\w:-]*)[\s>/]/;

/** An element with no closing tag: it can never be what a block is waiting for. */
const VOID_ELEMENTS = new Set([
  "area",
  "base",
  "br",
  "col",
  "embed",
  "hr",
  "img",
  "input",
  "link",
  "meta",
  "param",
  "source",
  "track",
  "wbr",
]);

/**
 * Three things are not an opening and each was a measured difference against the function this
 * replaces: a void element (`<img src=x>` closes nothing and waits for nothing), a self-closing
 * one (`<div />`), and a tag still being typed (`<div` with no `>` yet, which is every
 * half-arrived tag in a stream — it counts once the `>` lands).
 */
function openingTags(raw: string, tag: string): number {
  if (VOID_ELEMENTS.has(tag.toLowerCase())) return 0;
  const found = raw.match(new RegExp(`<${tag}(?=[\\s>/])[^>]*>`, "gi"));
  return found ? found.filter((match) => !match.trimEnd().endsWith("/>")).length : 0;
}

function closingTags(raw: string, tag: string): number {
  return raw.match(new RegExp(`</${tag}(?=[\\s>])[^>]*>`, "gi"))?.length ?? 0;
}

/** Non-overlapping `$$`, so `$$$$` is two fences and not three. */
const mathFences = (raw: string) => raw.match(/\$\$/g)?.length ?? 0;

export function splitStreamingBlocks(markdown: string): string[] {
  // A footnote's reference and its definition are two blocks that mean nothing apart, and the
  // renderer resolves them within one parse. Anything carrying either stays whole.
  if (FOOTNOTE_REFERENCE.test(markdown) || FOOTNOTE_DEFINITION.test(markdown)) return [markdown];

  const blocks: string[] = [];
  // Tag names still awaiting their close, innermost last. A block-level HTML element spans as
  // many tokens as it likes, and until it closes every one of them belongs to the same block.
  const openTags: string[] = [];
  let afterCode = false;

  for (const token of Lexer.lex(markdown, { gfm: true })) {
    const raw = token.raw;

    if (openTags.length > 0) {
      blocks[blocks.length - 1] += raw;
      const tag = openTags[openTags.length - 1]!;
      for (let i = openingTags(raw, tag); i > 0; i -= 1) openTags.push(tag);
      for (let i = closingTags(raw, tag); i > 0; i -= 1)
        if (openTags[openTags.length - 1] === tag) openTags.pop();
      continue;
    }

    if (token.type === "html" && (token as { block?: boolean }).block) {
      const tag = HTML_BLOCK_TAG.exec(raw)?.[1];
      if (tag && openingTags(raw, tag) > closingTags(raw, tag)) openTags.push(tag);
    }

    // An odd number of `$$` means the display-math fence is still open, so the next token is
    // inside it. Not checked straight after a code block, whose `$$` are literal text.
    if (blocks.length > 0 && !afterCode && mathFences(blocks[blocks.length - 1]!) % 2 === 1) {
      blocks[blocks.length - 1] += raw;
      continue;
    }

    blocks.push(raw);
    if (token.type !== "space") afterCode = token.type === "code";
  }
  return blocks;
}
