import { useEffect, useId, useMemo, useRef } from "react";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import rehypeKatex from "rehype-katex";
import /* @flame-lazy:math-styles */ "katex/dist/katex.min.css";
import { remarkSyntax } from "./remarkSyntax";
import { MarkdownBlockSource, markdownComponents } from "./markdownComponents";
import { handleMarkdownCopy } from "./markdownSelectionCopy";
import { rehypeFileRefs } from "./rehypeFileRefs";
import { isInlineImage } from "@/foundation/imageExport";
import { isMarkdownFragment, rehypeFootnoteLabels, remarkCallouts } from "@flame/ui";
import { useVisibleTextMaterial } from "../messageVisibleMaterial";
import { useT } from "@/lib/i18n/index";

export interface MarkdownMessageProps {
  text: string;
  streaming?: boolean;
}

const remarkPlugins = [...remarkSyntax, remarkCallouts];
const markdownUrlTransform: NonNullable<
  React.ComponentProps<typeof ReactMarkdown>["urlTransform"]
> = (value, _key, node) =>
  (node.tagName === "img" && isInlineImage(value)) ||
  (node.tagName === "a" && isMarkdownFragment(value))
    ? value
    : defaultUrlTransform(value);

export function MarkdownRenderer({ text }: MarkdownMessageProps) {
  const t = useT();
  const rootRef = useRef<HTMLDivElement>(null);
  const footnotePrefix = `user-content-${useId()}-`;
  useVisibleTextMaterial(true);
  const rehypePlugins = useMemo(() => {
    const labels: [typeof rehypeFootnoteLabels, string] = [rehypeFootnoteLabels, footnotePrefix];
    return [labels, rehypeFileRefs, rehypeKatex];
  }, [footnotePrefix]);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const ownerDocument = root.ownerDocument;
    const onCopy = (event: ClipboardEvent) => handleMarkdownCopy(root, event);
    ownerDocument.addEventListener("copy", onCopy, true);
    return () => ownerDocument.removeEventListener("copy", onCopy, true);
  }, []);

  return (
    <div ref={rootRef} className="md" data-prose="" dir="auto">
      <MarkdownBlockSource value={text}>
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
          components={markdownComponents}
          urlTransform={markdownUrlTransform}
        >
          {text}
        </ReactMarkdown>
      </MarkdownBlockSource>
    </div>
  );
}
