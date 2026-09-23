import QuickLRU from "quick-lru";

const cache = new QuickLRU<string, string>({ maxSize: 128 });

function cacheKey(lang: string, theme: string, code: string): string {
  return `${lang}:${theme}:${code}`;
}

export function getCachedHighlight(lang: string, theme: string, code: string): string | undefined {
  return cache.get(cacheKey(lang, theme, code));
}

export function setCachedHighlight(lang: string, theme: string, code: string, html: string): void {
  cache.set(cacheKey(lang, theme, code), html);
}
