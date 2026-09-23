import { Lexer } from "marked";

const FOOTNOTE_REFERENCE = /\[\^[\w-]{1,200}\](?!:)/;
const FOOTNOTE_DEFINITION = /\[\^[\w-]{1,200}\]:/;
const HTML_BLOCK_TAG = /<([A-Za-z][\w:-]*)[\s>/]/;

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

function openingTags(raw: string, tag: string): number {
  if (VOID_ELEMENTS.has(tag.toLowerCase())) return 0;
  const found = raw.match(new RegExp(`<${tag}(?=[\\s>/])[^>]*>`, "gi"));
  return found ? found.filter((match) => !match.trimEnd().endsWith("/>")).length : 0;
}

function closingTags(raw: string, tag: string): number {
  return raw.match(new RegExp(`</${tag}(?=[\\s>])[^>]*>`, "gi"))?.length ?? 0;
}

const mathFences = (raw: string) => raw.match(/\$\$/g)?.length ?? 0;

export function splitStreamingBlocks(markdown: string): string[] {
  if (FOOTNOTE_REFERENCE.test(markdown) || FOOTNOTE_DEFINITION.test(markdown)) return [markdown];

  const blocks: string[] = [];
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

    if (blocks.length > 0 && !afterCode && mathFences(blocks[blocks.length - 1]!) % 2 === 1) {
      blocks[blocks.length - 1] += raw;
      continue;
    }

    blocks.push(raw);
    if (token.type !== "space") afterCode = token.type === "code";
  }
  return blocks;
}
