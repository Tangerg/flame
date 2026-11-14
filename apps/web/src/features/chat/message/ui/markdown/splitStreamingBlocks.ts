import { unified } from "unified";
import remarkParse from "remark-parse";
import { visit } from "unist-util-visit";
import { remarkSyntax } from "@/features/chat/message/ui/markdown/remarkSyntax";
import { hasStreamingMathContext } from "./remarkMathSyntax";

const parser = unified().use(remarkParse).use(remarkSyntax);
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

function parsedBlocks(markdown: string): { blocks: string[]; global: boolean } {
  const tree = parser.parse(markdown);
  let hasDefinition = false;
  visit(tree, "definition", () => {
    hasDefinition = true;
  });
  if (hasDefinition) return { blocks: [markdown], global: true };

  const tokens: { type: string; raw: string }[] = [];
  let offset = 0;
  for (const node of tree.children) {
    const start = node.position!.start.offset!;
    const end = node.position!.end.offset!;
    if (start > offset) tokens.push({ type: "space", raw: markdown.slice(offset, start) });
    tokens.push({ type: node.type, raw: markdown.slice(start, end) });
    offset = end;
  }
  if (offset < markdown.length) tokens.push({ type: "space", raw: markdown.slice(offset) });

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
  if (blocks.length > 1 && /^\r?\n$/.test(blocks.at(-1)!)) {
    const tail = blocks.pop()!;
    blocks[blocks.length - 1] += tail;
  }
  return { blocks, global: false };
}

export function splitStreamingBlocks(markdown: string): string[] {
  return FOOTNOTE.test(markdown) ? [markdown] : parsedBlocks(markdown).blocks;
}

export function createStreamingBlockSplitter() {
  let previous = "";
  let previousRepair: ((tail: string) => string) | undefined;
  let stable: string[] = [];
  let offset = 0;
  let global = false;
  let result: string[] = [];

  return (markdown: string, repair?: (tail: string) => string): string[] => {
    if (markdown === previous && repair === previousRepair) return result;
    if (!markdown.startsWith(previous)) {
      stable = [];
      offset = 0;
      global = false;
    }
    previous = markdown;
    previousRepair = repair;
    let tail = markdown.slice(offset);
    global ||= FOOTNOTE.test(tail) || hasStreamingMathContext(tail);
    if (!global) {
      const parsed = parsedBlocks(tail);
      global = parsed.global;
      if (!global) {
        // The last block and its separator can still grow into a list, table or fence.
        const settled = Math.max(0, parsed.blocks.length - 2);
        const committed = parsed.blocks.slice(0, settled);
        stable = stable.concat(committed);
        offset += committed.reduce((length, block) => length + block.length, 0);
        tail = markdown.slice(offset);
        const repaired = repair?.(tail) ?? tail;
        result = stable.concat(
          repaired === tail ? parsed.blocks.slice(settled) : splitStreamingBlocks(repaired),
        );
        return result;
      }
    }
    // Later definitions and open display delimiters can change already rendered context.
    stable = [];
    offset = 0;
    result = [repair?.(markdown) ?? markdown];
    return result;
  };
}
