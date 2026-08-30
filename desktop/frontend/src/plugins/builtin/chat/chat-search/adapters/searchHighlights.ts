const HIGHLIGHTS_AVAILABLE = typeof CSS !== "undefined" && "highlights" in CSS;
const HIGHLIGHT_STYLE_ID = "flame-chat-search-highlight-styles";
const HIGHLIGHT_STYLES = `
::highlight(chat-search) {
  background-color: color-mix(in oklab, var(--color-warning) 32%, transparent);
  color: var(--color-text);
}
::highlight(chat-search-active) {
  background-color: var(--color-warning);
  color: var(--color-text-on-warning);
}
`;

/**
 * A runtime style rather than the application stylesheet: Lightning CSS cannot yet parse
 * Custom Highlight selectors and warns on these valid platform rules — and uninstalling the
 * search UI should take its paint rules with its Range registry entries.
 */
export function installChatSearchHighlightStyles(): () => void {
  const existing = document.getElementById(HIGHLIGHT_STYLE_ID);
  if (existing) return () => undefined;

  const style = document.createElement("style");
  style.id = HIGHLIGHT_STYLE_ID;
  style.textContent = HIGHLIGHT_STYLES;
  document.head.append(style);
  return () => style.remove();
}

export function paintChatSearchHighlights(ranges: Range[], activeIndex: number): void {
  // Older WebViews may lack CSS.highlights; navigation still scrolls ranges.
  if (!HIGHLIGHTS_AVAILABLE) return;

  CSS.highlights.delete("chat-search");
  CSS.highlights.delete("chat-search-active");
  if (ranges.length === 0) return;

  const inactive = ranges.filter((_, index) => index !== activeIndex);
  if (inactive.length > 0) {
    CSS.highlights.set("chat-search", new Highlight(...inactive));
  }
  if (ranges[activeIndex]) {
    CSS.highlights.set("chat-search-active", new Highlight(ranges[activeIndex]));
  }
}

export function clearChatSearchHighlights(): void {
  if (!HIGHLIGHTS_AVAILABLE) return;

  CSS.highlights.delete("chat-search");
  CSS.highlights.delete("chat-search-active");
}
