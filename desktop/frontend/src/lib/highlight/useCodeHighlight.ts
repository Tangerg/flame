import type { Highlighter, ShikiTransformer } from "shiki";
import { useEffect, useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { measureShikiHighlight } from "@/lib/metrics";
import { useScheme } from "../appearance";
import { getHighlighter, reportHighlightFailure, resolveLang } from "./shiki";
import { getCachedHighlight, setCachedHighlight } from "./shikiCache";

function useShikiTheme(): string {
  return useScheme() === "light" ? "github-light-high-contrast" : "github-dark";
}

export function useCodeHighlighter(): { highlighter: Highlighter | null; theme: string } {
  const theme = useShikiTheme();
  const [highlighter, setHighlighter] = useState<Highlighter | null>(null);
  useEffect(() => {
    let cancelled = false;
    void getHighlighter().then((h) => {
      if (!cancelled) setHighlighter(h);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return { highlighter, theme };
}

export function stripCodeWrapper(html: string, fallback: string): string {
  return html.match(/<code[^>]*>([\s\S]*)<\/code>/)?.[1] ?? fallback;
}

const FOCUS_INSET: ShikiTransformer[] = [
  {
    pre(node) {
      node.properties["data-focus-inset"] = "";
    },
  },
];

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
    getHighlighter()
      .then((highlighter) => {
        if (cancelled) return;
        try {
          const resolvedLang = resolveLang(highlighter, lang);
          const start = performance.now();
          const html = highlighter.codeToHtml(settledCode, {
            lang: resolvedLang,
            theme,
            transformers: FOCUS_INSET,
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
