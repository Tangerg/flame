import * as stylex from "@stylexjs/stylex";
import { type HighlighterCore } from "shiki/core";
import { useLayoutEffect, useMemo, useRef, type Ref } from "react";
import { stripCodeWrapper, useCodeHighlighter } from "@/lib/highlight/useCodeHighlight";
import { langFromPath, resolveLang } from "@/lib/highlight/shiki";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { codeStyles as cs } from "@/features/workspace/ui/viewStyles";
import { vocab } from "@flame/ui";
function highlightLines(h: HighlighterCore, code: string, theme: string, path: string): string[] {
  const lang = resolveLang(h, langFromPath(path));
  return stripCodeWrapper(h.codeToHtml(code, { lang, theme }), "").split("\n");
}

export function FileView({
  path,
  content,
  startLine,
  targetLine,
  intent,
  ref,
}: {
  path: string;
  content: string;
  startLine: number;
  targetLine: number;
  // Null while the content is a placeholder for another request, so it cannot consume the intent.
  intent: object | null;
  // The presented source, without framing outside the line rows.
  ref?: Ref<HTMLDivElement>;
}) {
  const { highlighter, theme: shikiTheme } = useCodeHighlighter(langFromPath(path));

  const plain = useMemo(() => content.split("\n"), [content]);
  const numberWidth = `${String(startLine + plain.length - 1).length}ch`;
  const highlighted = useMemo(
    () => (highlighter ? highlightLines(highlighter, content, shikiTheme, path) : null),
    [highlighter, content, shikiTheme, path],
  );

  const targetRef = useRef<HTMLDivElement>(null);
  const placedIntent = useRef<object | null>(null);
  useLayoutEffect(() => {
    if (!intent || placedIntent.current === intent || targetLine <= 0 || !targetRef.current) return;
    placedIntent.current = intent;
    targetRef.current.scrollIntoView({ block: "center" });
  }, [intent, content, targetLine]);

  const last = plain.length - 1;
  return (
    <div
      ref={ref}
      data-quote-source="file"
      data-quote-path={path}
      {...stylex.props(cs.sheet, typeStep.code)}
    >
      {/* One stable child keeps a whole-source range intact while rows refresh or grow. */}
      <div>
        {plain.map((line, i) => {
          const n = startLine + i;
          const isTarget = n === targetLine;
          const html = highlighted?.[i];
          // The cell carries the source's own line break; row layout alone drops blank lines.
          const separator = i < last ? "\n" : "";
          return (
            <div
              key={i}
              ref={isTarget ? targetRef : undefined}
              data-quote-line={n}
              style={{ gridTemplateColumns: `${numberWidth} minmax(0, 1fr)` }}
              {...stylex.props(cs.lineRow, isTarget && cs.targetLine)}
            >
              <span {...stylex.props(cs.gutter)}>{n}</span>
              {html !== undefined ? (
                <span
                  {...stylex.props(cs.wrap)}
                  dangerouslySetInnerHTML={{ __html: html + separator }}
                />
              ) : (
                <span {...stylex.props(cs.wrap, vocab.soft)}>{line + separator}</span>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
