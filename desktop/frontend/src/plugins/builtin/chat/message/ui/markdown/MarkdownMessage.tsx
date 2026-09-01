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
import { useCommitThrottle, useStreamReveal, type MarkdownReveal } from "./streamReveal";
import { useVisibleTextMaterial } from "../messageVisibleMaterial";
import "remark-github-blockquote-alert/alert.css";

const PARSE_COMMIT_MS = 33;

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

const remarkPlugins = [
  remarkGfm,
  remarkBreaks,
  remarkCjkFriendly,
  remarkMath,
  remarkAlert,
  remarkLiteralUnknownHtml,
];

const DENIED_HTML_TAGS = new Set(["script", "iframe", "object", "embed", "form"]);
const allowElement = (el: { tagName: string }) => !DENIED_HTML_TAGS.has(el.tagName);

const markdownUrlTransform: NonNullable<
  React.ComponentProps<typeof ReactMarkdown>["urlTransform"]
> = (value, _key, node) =>
  node.tagName === "img" && isInlineMarkdownImage(value) ? value : defaultUrlTransform(value);

export function MarkdownMessage(props: Props) {
  const { text, reveal } = props;
  // The props union already narrows `streaming` to false wherever `reveal` is "instant";
  // re-deriving it here only hid that the type had settled it.
  const streaming = props.streaming ?? false;
  const instant = reveal === "instant";
  const rootRef = useRef<HTMLDivElement>(null);
  const display = useStreamReveal(text, streaming, reveal);

  const committed = useCommitThrottle(display, streaming ? PARSE_COMMIT_MS : 0);

  const deferred = useDeferredValue(committed);
  const source = instant ? committed : deferred;
  useVisibleTextMaterial(source === text);

  const normalized = useMemo(() => normalizeMarkdownMath(source), [source]);

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

const MarkdownBlock = memo(function MarkdownBlock({ text, streaming, reveal }: MarkdownBlockProps) {
  const hasMath = text.includes("$");
  useEffect(() => {
    if (hasMath) ensureKatexCss();
  }, [hasMath]);

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
