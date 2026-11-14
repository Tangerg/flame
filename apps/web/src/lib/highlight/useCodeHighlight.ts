import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { measureShikiHighlight } from "@/lib/metrics";
import { useScheme } from "@/lib/appearance";
import { requestHighlight, reportHighlightFailure } from "@/lib/highlight/shiki";
import { shikiThemeName } from "@/lib/highlight/shikiTheme";
import { getCachedHighlight, setCachedHighlight } from "@/lib/highlight/shikiCache";
function useShikiTheme(): string {
  return shikiThemeName(useScheme());
}

export function stripCodeWrapper(html: string, fallback: string): string {
  return html.match(/<code[^>]*>([\s\S]*)<\/code>/)?.[1] ?? fallback;
}

interface HighlightedCode {
  lang: string;
  theme: string;
  code: string;
  html: string;
}

export function useHighlightedCode(code: string, lang: string): string | null {
  const theme = useShikiTheme();
  const owner = useRef(Symbol("highlight"));
  const current = useRef<{ code: string; lang: string; theme: string } | null>(null);
  useLayoutEffect(() => {
    current.current = { code, lang, theme };
    return () => {
      current.current = null;
    };
  }, [code, lang, theme]);
  const [settledCode] = useDebouncedValue(code, { wait: 120 });
  const cached = getCachedHighlight(lang, theme, settledCode);
  const [highlighted, setHighlighted] = useState<HighlightedCode | null>(() =>
    cached === undefined ? null : { lang, theme, code: settledCode, html: cached },
  );

  useEffect(() => {
    if (cached !== undefined || code !== settledCode) return;
    const controller = new AbortController();
    void requestHighlight(owner.current, { code: settledCode, lang, theme }, controller.signal)
      .then((result) => {
        const captured = current.current;
        if (
          controller.signal.aborted ||
          !captured ||
          captured.code !== settledCode ||
          captured.lang !== lang ||
          captured.theme !== theme
        )
          return;
        measureShikiHighlight(result.duration, result.lang);
        setCachedHighlight(lang, theme, settledCode, result.html);
        setHighlighted({ lang, theme, code: settledCode, html: result.html });
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) reportHighlightFailure(`grammar ${lang}`, error);
      });
    return () => controller.abort();
  }, [cached, code, lang, settledCode, theme]);

  if (code !== settledCode) return null;
  if (cached !== undefined) return cached;
  return highlighted?.lang === lang && highlighted.theme === theme && highlighted.code === code
    ? highlighted.html
    : null;
}
