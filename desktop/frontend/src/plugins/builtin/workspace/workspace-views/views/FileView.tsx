import * as stylex from "@stylexjs/stylex";
import type { Highlighter } from "shiki";
import { useLayoutEffect, useMemo, useRef } from "react";
import { stripCodeWrapper, useCodeHighlighter } from "@/lib/highlight/useCodeHighlight";
import { langFromPath, resolveLang } from "@/lib/highlight/shiki";
import { type as typeStep } from "@/styles/tokens.stylex";
import { codeStyles as cs } from "./viewStyles";
import { vocab } from "@/ui";

function highlightLines(h: Highlighter, code: string, theme: string, path: string): string[] {
  const lang = resolveLang(h, langFromPath(path));
  return stripCodeWrapper(h.codeToHtml(code, { lang, theme }), "").split("\n");
}

export function FileView({
  path,
  content,
  startLine,
  targetLine,
  intent,
}: {
  path: string;
  content: string;
  startLine: number;
  targetLine: number;
  intent: object;
}) {
  const { highlighter, theme: shikiTheme } = useCodeHighlighter();

  const plain = useMemo(() => content.split("\n"), [content]);
  const highlighted = useMemo(
    () => (highlighter ? highlightLines(highlighter, content, shikiTheme, path) : null),
    [highlighter, content, shikiTheme, path],
  );

  const targetRef = useRef<HTMLDivElement>(null);
  const placedIntent = useRef<object | null>(null);
  useLayoutEffect(() => {
    if (placedIntent.current === intent || targetLine <= 0 || !targetRef.current) return;
    placedIntent.current = intent;
    targetRef.current.scrollIntoView({ block: "center" });
  }, [intent, content, targetLine]);

  return (
    <div data-quote-source="file" data-quote-path={path} {...stylex.props(cs.sheet, typeStep.code)}>
      {plain.map((line, i) => {
        const n = startLine + i;
        const isTarget = n === targetLine;
        const html = highlighted?.[i];
        return (
          <div
            key={i}
            ref={isTarget ? targetRef : undefined}
            data-quote-line={n}
            {...stylex.props(cs.lineRow, cs.gutterOne, isTarget && cs.targetLine)}
          >
            <span {...stylex.props(cs.gutter, typeStep.uiSm)}>{n}</span>
            {html !== undefined ? (
              <span {...stylex.props(cs.wrap)} dangerouslySetInnerHTML={{ __html: html }} />
            ) : (
              <span {...stylex.props(cs.wrap, vocab.soft)}>{line || " "}</span>
            )}
          </div>
        );
      })}
    </div>
  );
}
