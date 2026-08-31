import type { Highlighter } from "shiki";
import { useEffect, useState } from "react";
import { useScheme } from "../appearance";
import { getHighlighter } from "./shiki";

/** Keyed on the SCHEME, never the theme id, so a third-party light theme still resolves. */
export function useShikiTheme(): string {
  return useScheme() === "light" ? "github-light-high-contrast" : "github-dark";
}

/** Highlighter loaded into state (null until ready) plus the active theme. */
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

/** Inner HTML of Shiki's <pre><code>…</code></pre>, so token spans can go in a custom row.
 *  Returns `fallback` on no match. */
export function stripCodeWrapper(html: string, fallback: string): string {
  return html.match(/<code[^>]*>([\s\S]*)<\/code>/)?.[1] ?? fallback;
}
