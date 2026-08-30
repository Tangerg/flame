// Completed code blocks rarely re-render, but they DO re-mount on scroll-away/back, theme
// toggle, and memo-key invalidation in long histories — each re-mount re-runs the tokenizer
// at ~3-10ms. Bounded so a long session cannot grow the map without limit.

import QuickLRU from "quick-lru";

const cache = new QuickLRU<string, string>({ maxSize: 128 });

// `:` delimits the key fields. It can't appear in a Shiki lang or theme id
// (lowercase / digits / hyphens), and `code` is last, so an arbitrary code
// body — even one containing `:` — can never collide with another entry.
function cacheKey(lang: string, theme: string, code: string): string {
  return `${lang}:${theme}:${code}`;
}

export function getCachedHighlight(lang: string, theme: string, code: string): string | undefined {
  return cache.get(cacheKey(lang, theme, code));
}

export function setCachedHighlight(lang: string, theme: string, code: string, html: string): void {
  cache.set(cacheKey(lang, theme, code), html);
}
