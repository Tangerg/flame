const HIGHLIGHTS_AVAILABLE = typeof CSS !== "undefined" && "highlights" in CSS;

export function paintChatSearchHighlights(ranges: Range[], activeIndex: number): void {
  if (!HIGHLIGHTS_AVAILABLE) return;

  CSS.highlights.delete("search-match");
  CSS.highlights.delete("search-match-active");
  if (ranges.length === 0) return;

  const inactive = ranges.filter((_, index) => index !== activeIndex);
  if (inactive.length > 0) {
    CSS.highlights.set("search-match", new Highlight(...inactive));
  }
  if (ranges[activeIndex]) {
    CSS.highlights.set("search-match-active", new Highlight(ranges[activeIndex]));
  }
}

export function clearChatSearchHighlights(): void {
  if (!HIGHLIGHTS_AVAILABLE) return;

  CSS.highlights.delete("search-match");
  CSS.highlights.delete("search-match-active");
}
