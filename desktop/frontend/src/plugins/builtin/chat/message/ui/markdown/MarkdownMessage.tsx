import { memo, useDeferredValue, useEffect, useMemo, useRef } from "react";

import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import rehypeKatex from "rehype-katex";
import rehypeRaw from "rehype-raw";
import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";
import remarkGfm from "remark-gfm";
import remarkAlert from "remark-github-blockquote-alert";
import remarkMath from "remark-math";
import remend from "remend";
import { parseMarkdownIntoBlocks } from "streamdown";
import { createMarkdownComponents } from "./markdownComponents";
import { isInlineMarkdownImage } from "./MarkdownImage";
import { handleMarkdownCopy } from "./markdownSelectionCopy";
import { ensureKatexCss } from "./katexCss";
import { rehypeFadeIn } from "./rehypeFadeIn";
import { rehypeFileRefs } from "./rehypeFileRefs";
import { rehypeStreamCaret } from "./rehypeStreamCaret";
import { normalizeMarkdownMath } from "./preprocess";
import { remarkLiteralUnknownHtml } from "./remarkLiteralUnknownHtml";
import { useCommitThrottle, useStreamReveal } from "./streamReveal";
import { useVisibleTextMaterial } from "../messageVisibleMaterial";
import "remark-github-blockquote-alert/alert.css";

// ~30fps: imperceptible for a text reveal, but caps a run of tiny tokens at one markdown
// re-parse per window instead of one per animation frame.
const PARSE_COMMIT_MS = 33;

export type MarkdownReveal = "instant" | "smooth" | "typewriter";

type Props = {
  text: string;
} & (
  | {
      reveal: "instant";
      streaming?: false;
    }
  | {
      reveal: Exclude<MarkdownReveal, "instant">;
      streaming?: boolean;
    }
);

interface MarkdownBlockProps {
  text: string;
  streaming: boolean;
  reveal: MarkdownReveal;
}

// Module-level so react-markdown does not treat each render as a new plugin set. Order
// matters in the rehype chain.
const remarkPlugins = [
  remarkGfm,
  remarkBreaks,
  remarkCjkFriendly,
  remarkMath,
  remarkAlert,
  remarkLiteralUnknownHtml,
];

// Can execute or break the sandbox if the model emits them as raw HTML; this blocklist
// takes precedence over rehype-raw.
const DENIED_HTML_TAGS = new Set(["script", "iframe", "object", "embed", "form"]);
const allowElement = (el: { tagName: string }) => !DENIED_HTML_TAGS.has(el.tagName);

// react-markdown drops data URLs by default. MarkdownImage blocks every REMOTE image, but
// inlined data is already self-contained and must survive the parser to reach it.
const markdownUrlTransform: NonNullable<
  React.ComponentProps<typeof ReactMarkdown>["urlTransform"]
> = (value, _key, node) =>
  node.tagName === "img" && isInlineMarkdownImage(value) ? value : defaultUrlTransform(value);

/**
 * Uses `streamdown`'s `parseMarkdownIntoBlocks` — it balances unclosed fences, math and HTML
 * mid-stream — but NOT its `<Streamdown>` renderer, which ships a design system that
 * bypasses `.md` CSS. Each block is memoised, so only the tail re-parses per reveal tick.
 */
export function MarkdownMessage(props: Props) {
  const { text, reveal } = props;
  const streaming = reveal === "instant" ? false : !!props.streaming;
  const instant = reveal === "instant";
  const rootRef = useRef<HTMLDivElement>(null);
  const revealed = useStreamReveal(text, streaming, reveal === "typewriter");
  const display = instant ? text : revealed;

  const committed = useCommitThrottle(display, streaming ? PARSE_COMMIT_MS : 0);

  // Low-priority re-parse: scrolling and typing keep the previous parse on-screen instead
  // of blocking a frame. Instant text skips the defer to stay crisp on first paint — there
  // is no stream to keep responsive.
  const deferred = useDeferredValue(committed);
  const source = instant ? committed : deferred;
  useVisibleTextMaterial(source === text);

  // Normalize model-emitted math delimiters + guard currency BEFORE remark-math
  // parses. Must run on the whole body ahead of block-splitting so a display
  // math span (`$$...$$`) isn't torn across two blocks.
  const normalized = useMemo(() => normalizeMarkdownMath(source), [source]);

  // Auto-closes unterminated emphasis and fences on the FULL text before splitting, since
  // block boundaries read more reliably on well-formed markdown.
  const repaired = useMemo(() => {
    if (instant) return normalized;
    return remend(normalized);
  }, [instant, normalized]);

  const blocks = useMemo(() => parseMarkdownIntoBlocks(repaired), [repaired]);
  const lastIdx = blocks.length - 1;

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const ownerDocument = root.ownerDocument;
    const onCopy = (event: ClipboardEvent) => {
      handleMarkdownCopy(root, event);
    };
    ownerDocument.addEventListener("copy", onCopy, true);
    return () => ownerDocument.removeEventListener("copy", onCopy, true);
  }, []);

  return (
    <div ref={rootRef} className="md" dir="auto">
      {blocks.map((block, i) => (
        // Index keys are correct here: blocks are append-only while streaming, so position
        // IS identity. Keying by content changes the key on every tail edit and remounts
        // the fiber each tick, losing its state and paying the mount cost per token.
        <MarkdownBlock
          key={i}
          text={block}
          streaming={streaming && i === lastIdx}
          reveal={reveal}
        />
      ))}
    </div>
  );
}

// Memoised on content plus flags. `streaming` is true only for the TAIL block, gating the
// typewriter caret; smooth mode conveys the same thing through its per-word fade.
const MarkdownBlock = memo(function MarkdownBlock({ text, streaming, reveal }: MarkdownBlockProps) {
  // The probe is just `$`, so a USD price preloads the ~30KB stylesheet earlier than needed
  // — harmless, and remarkMath still ignores ambiguous single-`$` cases at render.
  const hasMath = text.includes("$");
  useEffect(() => {
    if (hasMath) ensureKatexCss();
  }, [hasMath]);

  // ORDER MATTERS: rehypeRaw first so later plugins see the expanded tree, and
  // rehypeFileRefs before rehypeFadeIn so it sees whole text nodes rather than per-word
  // spans. Typewriter mode drops rehypeFadeIn — the char reveal IS the animation.
  //
  // rehypeFileRefs runs only on a SETTLED block: a half-arrived path would flash as a link.
  const rehypePlugins = useMemo(() => {
    if (reveal === "instant") {
      return [rehypeRaw, rehypeFileRefs, rehypeKatex];
    }
    if (reveal === "typewriter") {
      return streaming
        ? [rehypeRaw, rehypeKatex, rehypeStreamCaret]
        : [rehypeRaw, rehypeFileRefs, rehypeKatex];
    }
    return streaming
      ? [rehypeRaw, rehypeFadeIn, rehypeKatex]
      : [rehypeRaw, rehypeFileRefs, rehypeFadeIn, rehypeKatex];
  }, [reveal, streaming]);
  const components = useMemo(() => createMarkdownComponents(text), [text]);

  return (
    <ReactMarkdown
      remarkPlugins={remarkPlugins}
      rehypePlugins={rehypePlugins}
      components={components}
      allowElement={allowElement}
      urlTransform={markdownUrlTransform}
    >
      {text}
    </ReactMarkdown>
  );
});
