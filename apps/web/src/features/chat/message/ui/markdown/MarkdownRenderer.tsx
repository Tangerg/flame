import { memo, useDeferredValue, useEffect, useId, useMemo, useRef } from "react";
import ReactMarkdown from "react-markdown";
import { defaultUrlTransform } from "react-markdown";
import rehypeKatex from "rehype-katex";
import rehypeRaw from "rehype-raw";
import { remarkSyntax } from "@/features/chat/message/ui/markdown/remarkSyntax";
import remend from "remend";
import { splitStreamingBlocks } from "@/features/chat/message/ui/markdown/splitStreamingBlocks";
import { createMarkdownComponents } from "@/features/chat/message/ui/markdown/markdownComponents";
import { isInlineImage } from "@/foundation/imageExport";
import { handleMarkdownCopy } from "@/features/chat/message/ui/markdown/markdownSelectionCopy";
import { ensureKatexCss } from "@/features/chat/message/ui/markdown/katexCss";
import { rehypeFadeIn } from "@/features/chat/message/ui/markdown/rehypeFadeIn";
import { rehypeFileRefs } from "@/features/chat/message/ui/markdown/rehypeFileRefs";
import { rehypeFootnoteLabels } from "@/features/chat/message/ui/markdown/rehypeFootnoteLabels";
import { rehypeStreamCaret } from "@/features/chat/message/ui/markdown/rehypeStreamCaret";
import { remarkCallouts } from "@flame/ui";
import { remarkLiteralUnknownHtml } from "@/features/chat/message/ui/markdown/remarkLiteralUnknownHtml";
import {
  useCommitThrottle,
  useStreamReveal,
  type MarkdownReveal,
} from "@/features/chat/message/ui/markdown/streamReveal";
import { useVisibleTextMaterial } from "@/features/chat/message/ui/messageVisibleMaterial";
import { useT } from "@/lib/i18n/index";
const PARSE_COMMIT_MS = 33;

export type MarkdownMessageProps = {
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
  footnotePrefix: string;
}

const remarkPlugins = [...remarkSyntax, remarkCallouts, remarkLiteralUnknownHtml];

const DENIED_HTML_TAGS = new Set(["script", "iframe", "object", "embed", "form"]);

const allowElement = (el: { tagName: string }) => !DENIED_HTML_TAGS.has(el.tagName);

const markdownUrlTransform: NonNullable<
  React.ComponentProps<typeof ReactMarkdown>["urlTransform"]
> = (value, _key, node) =>
  node.tagName === "img" && isInlineImage(value) ? value : defaultUrlTransform(value);

export function MarkdownRenderer(props: MarkdownMessageProps) {
  const { text, reveal } = props;
  const streaming = props.streaming ?? false;
  const instant = reveal === "instant";
  const rootRef = useRef<HTMLDivElement>(null);
  const footnotePrefix = `user-content-${useId()}-`;
  const display = useStreamReveal(text, streaming, reveal);

  const committed = useCommitThrottle(display, streaming ? PARSE_COMMIT_MS : 0);

  const deferred = useDeferredValue(committed);
  const source = instant ? committed : deferred;
  useVisibleTextMaterial(source === text);

  const repaired = useMemo(() => {
    if (instant) return source;
    return remend(source);
  }, [instant, source]);

  const blocks = useMemo(() => splitStreamingBlocks(repaired), [repaired]);
  const lastIdx = blocks.length - 1;

  useEffect(() => {
    ensureKatexCss();
  }, []);

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
    <div ref={rootRef} className="md" data-prose="" dir="auto">
      {blocks.map((block, i) => (
        <MarkdownBlock
          key={i}
          text={block}
          streaming={streaming && i === lastIdx}
          reveal={reveal}
          footnotePrefix={footnotePrefix}
        />
      ))}
    </div>
  );
}

const MarkdownBlock = memo(function MarkdownBlock({
  text,
  streaming,
  reveal,
  footnotePrefix,
}: MarkdownBlockProps) {
  const t = useT();

  const rehypePlugins = useMemo(() => {
    const labels: [typeof rehypeFootnoteLabels, string] = [rehypeFootnoteLabels, footnotePrefix];
    if (!streaming) return [rehypeRaw, labels, rehypeFileRefs, rehypeKatex];
    if (reveal === "typewriter") return [rehypeRaw, labels, rehypeKatex, rehypeStreamCaret];
    return [rehypeRaw, labels, rehypeFadeIn, rehypeKatex];
  }, [footnotePrefix, reveal, streaming]);
  const components = useMemo(() => createMarkdownComponents(text), [text]);

  return (
    <ReactMarkdown
      remarkPlugins={remarkPlugins}
      remarkRehypeOptions={{
        clobberPrefix: footnotePrefix,
        footnoteLabel: t("message.markdown.footnotes"),
        footnoteBackLabel: (index, reuse) =>
          t("message.markdown.backReference", {
            reference: `${index + 1}${reuse > 1 ? `-${reuse}` : ""}`,
          }),
      }}
      rehypePlugins={rehypePlugins}
      components={components}
      allowElement={allowElement}
      urlTransform={markdownUrlTransform}
    >
      {text}
    </ReactMarkdown>
  );
});
