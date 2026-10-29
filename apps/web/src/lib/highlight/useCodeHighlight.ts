import { type HighlighterCore } from "shiki/core";
import { useEffect, useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { measureShikiHighlight } from "@/lib/metrics";
import { useScheme } from "@/lib/appearance";
import { getHighlighter, reportHighlightFailure, resolveLang } from "@/lib/highlight/shiki";
import { getCachedHighlight, setCachedHighlight } from "@/lib/highlight/shikiCache";
function useShikiTheme(): string {
  return useScheme() === "light" ? "github-light-high-contrast" : "github-dark";
}

export function useCodeHighlighter(lang: string): {
  highlighter: HighlighterCore | null;
  theme: string;
} {
  const theme = useShikiTheme();
  const [ready, setReady] = useState<{ lang: string; highlighter: HighlighterCore } | null>(null);
  useEffect(() => {
    let cancelled = false;
    void getHighlighter(lang)
      .then((h) => {
        if (!cancelled) setReady({ lang, highlighter: h });
      })
      .catch((error: unknown) => reportHighlightFailure("highlighter unavailable", error));
    return () => {
      cancelled = true;
    };
  }, [lang]);
  return { highlighter: ready?.lang === lang ? ready.highlighter : null, theme };
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
  const [settledCode] = useDebouncedValue(code, { wait: 120 });
  const cached = getCachedHighlight(lang, theme, settledCode);
  const [highlighted, setHighlighted] = useState<HighlightedCode | null>(() =>
    cached === undefined ? null : { lang, theme, code: settledCode, html: cached },
  );

  useEffect(() => {
    if (cached !== undefined) return;
    let cancelled = false;
    getHighlighter(lang)
      .then((highlighter) => {
        if (cancelled) return;
        try {
          const resolvedLang = resolveLang(highlighter, lang);
          const start = performance.now();
          const html = highlighter.codeToHtml(settledCode, {
            lang: resolvedLang,
            theme,
          });
          measureShikiHighlight(performance.now() - start, resolvedLang);
          setCachedHighlight(lang, theme, settledCode, html);
          setHighlighted({ lang, theme, code: settledCode, html });
        } catch (error) {
          reportHighlightFailure(`grammar ${lang}`, error);
        }
      })
      .catch((error: unknown) => reportHighlightFailure("highlighter unavailable", error));
    return () => {
      cancelled = true;
    };
  }, [cached, lang, settledCode, theme]);

  if (code !== settledCode) return null;
  if (cached !== undefined) return cached;
  return highlighted?.lang === lang && highlighted.theme === theme && highlighted.code === code
    ? highlighted.html
    : null;
}
