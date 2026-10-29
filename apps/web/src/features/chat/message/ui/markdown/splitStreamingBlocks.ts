import { unified } from "unified";
import remarkParse from "remark-parse";
import remarkGfm from "remark-gfm";
import { remarkMathSyntax } from "@/features/chat/message/ui/markdown/remarkMathSyntax";
const FOOTNOTE = /\[\^[^\]\r\n]+\]/;

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

const parser = unified().use(remarkParse).use(remarkGfm).use(remarkMathSyntax).freeze();

export function splitStreamingBlocks(markdown: string): string[] {
  if (FOOTNOTE.test(markdown)) return [markdown];

  const tree = parser.parse(markdown);
  if (tree.children.some((node) => node.type === "definition")) return [markdown];
  const tokens: { type: string; raw: string }[] = [];
  let end = 0;
  // Parser-created nodes retain source offsets; slicing keeps original material intact.
  for (const node of tree.children) {
    const start = node.position!.start.offset!;
    if (start > end) tokens.push({ type: "space", raw: markdown.slice(end, start) });
    end = node.position!.end.offset!;
    tokens.push({ type: node.type, raw: markdown.slice(start, end) });
  }
  if (end < markdown.length) tokens.push({ type: "space", raw: markdown.slice(end) });

  const blocks: string[] = [];
  const openTags: string[] = [];

  for (const token of tokens) {
    const raw = token.raw;

    if (openTags.length > 0) {
      blocks[blocks.length - 1] += raw;
      const tag = openTags[openTags.length - 1]!;
      for (let i = openingTags(raw, tag); i > 0; i -= 1) openTags.push(tag);
      for (let i = closingTags(raw, tag); i > 0; i -= 1)
        if (openTags[openTags.length - 1] === tag) openTags.pop();
      continue;
    }

    if (token.type === "html") {
      const tag = HTML_BLOCK_TAG.exec(raw)?.[1];
      if (tag && openingTags(raw, tag) > closingTags(raw, tag)) openTags.push(tag);
    }

    blocks.push(raw);
  }
  return blocks;
}
