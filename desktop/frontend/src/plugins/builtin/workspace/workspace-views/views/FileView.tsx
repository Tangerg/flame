import type { Highlighter } from "shiki";
import { useEffect, useMemo, useRef } from "react";
import { stripCodeWrapper, useCodeHighlighter } from "@/lib/highlight/useCodeHighlight";
import { langFromPath, resolveLang } from "@/lib/highlight/shiki";
import { cn } from "@/lib/classNames";

function highlightLines(h: Highlighter, code: string, theme: string, path: string): string[] {
  const lang = resolveLang(h, langFromPath(path));
  return stripCodeWrapper(h.codeToHtml(code, { lang, theme }), "").split("\n");
}

export function FileView({
  path,
  content,
  startLine,
  targetLine,
}: {
  path: string;
  content: string;
  startLine: number;
  targetLine: number;
}) {
  const { highlighter, theme: shikiTheme } = useCodeHighlighter();

  const plain = useMemo(() => content.split("\n"), [content]);
  const highlighted = useMemo(
    () => (highlighter ? highlightLines(highlighter, content, shikiTheme, path) : null),
    [highlighter, content, shikiTheme, path],
  );

  const targetRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (targetLine > 0) targetRef.current?.scrollIntoView({ block: "center" });
  }, [content, path, targetLine]);

  return (
    <div className="py-2 font-mono text-code leading-relaxed">
      {plain.map((line, i) => {
        const n = startLine + i;
        const isTarget = n === targetLine;
        const html = highlighted?.[i];
        return (
          <div
            key={i}
            ref={isTarget ? targetRef : undefined}
            className={cn(
              "grid grid-cols-[44px_minmax(0,1fr)] items-start gap-2 px-3",
              isTarget && "bg-accent-wash",
            )}
          >
            <span className="text-right text-ui-sm text-fg-faint select-none">{n}</span>
            {html !== undefined ? (
              <span
                className="min-w-0 whitespace-pre-wrap wrap-anywhere"
                dangerouslySetInnerHTML={{ __html: html }}
              />
            ) : (
              <span className="min-w-0 whitespace-pre-wrap wrap-anywhere text-fg-soft">
                {line || " "}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}
