import type { Highlighter } from "shiki";
import { useEffect, useState } from "react";
import { useScheme } from "../appearance";
import { getHighlighter } from "./shiki";

export function useShikiTheme(): string {
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
