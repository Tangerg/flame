// Blocks re-mount on scroll-away/back and theme toggle, each re-running the tokenizer at
// ~3-10ms. Bounded so a long session cannot grow the map without limit.

import QuickLRU from "quick-lru";

const cache = new QuickLRU<string, string>({ maxSize: 128 });

// `:` delimits the fields: it cannot appear in a lang or theme id, and `code` is last, so a
// body containing `:` cannot collide.
function cacheKey(lang: string, theme: string, code: string): string {
  return `${lang}:${theme}:${code}`;
}

export function getCachedHighlight(lang: string, theme: string, code: string): string | undefined {
  return cache.get(cacheKey(lang, theme, code));
}

export function setCachedHighlight(lang: string, theme: string, code: string, html: string): void {
  cache.set(cacheKey(lang, theme, code), html);
}
