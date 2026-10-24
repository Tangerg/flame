import { type FocusEvent, type MouseEvent } from "react";
import { type StickToBottomInstance } from "use-stick-to-bottom";
type ReadingFollow = Pick<StickToBottomInstance, "scrollRef" | "state" | "stopScroll">;

function safeReadingBounds(viewport: HTMLElement) {
  const frame = viewport.getBoundingClientRect();
  const style = getComputedStyle(viewport);
  return {
    top: frame.top + (parseFloat(style.scrollPaddingTop) || 0),
    bottom: frame.bottom - (parseFloat(style.scrollPaddingBottom) || 0),
  };
}

export function stopFollowOnReadingFocus(
  event: FocusEvent<HTMLElement>,
  { scrollRef, state, stopScroll }: ReadingFollow,
) {
  const viewport = scrollRef.current;
  const target = event.target;
  if (target === event.currentTarget || !viewport?.contains(target) || !state.isAtBottom) return;
  const movement = state.targetScrollTop - viewport.scrollTop;
  if (movement <= 0) return;
  const bounds = target.getBoundingClientRect();
  const { top, bottom } = safeReadingBounds(viewport);
  // Native focus can reveal its target before a queued resize-follow tick.
  if (bounds.top - movement < top || bounds.bottom - movement > bottom) stopScroll();
}

// The bubbled click still sees the pre-toggle state before the disclosure renders. Opening mounts
// content whose resize-follow would carry the already-focused trigger away. Closing retires content
// the reader may have scrolled through, which can leave the still-focused trigger outside the safe
// frame; revealing it here keeps the activated control readable before that content disappears.
export function prepareDisclosureReading(
  event: MouseEvent<HTMLElement>,
  { scrollRef, state, stopScroll }: ReadingFollow,
) {
  const viewport = scrollRef.current;
  const host = event.currentTarget;
  if (event.defaultPrevented || !viewport?.contains(host)) return;
  if (!(event.target instanceof Element)) return;
  const trigger = event.target.closest<HTMLButtonElement>(
    'button[aria-expanded="true"], button[aria-expanded="false"]',
  );
  if (!trigger || trigger === host || !host.contains(trigger)) return;
  const popup = trigger.getAttribute("aria-haspopup");
  if (
    trigger.disabled ||
    trigger.getAttribute("aria-disabled") === "true" ||
    trigger.getAttribute("role") === "combobox" ||
    (popup !== null && popup !== "false")
  )
    return;
  if (trigger.getAttribute("aria-expanded") === "false") {
    if (state.isAtBottom) stopScroll();
    return;
  }
  if (trigger !== trigger.ownerDocument.activeElement) return;
  const panelId = trigger.getAttribute("aria-controls");
  const panel = panelId ? trigger.ownerDocument.getElementById(panelId) : null;
  // A footer toggle can move when content above it folds; that position belongs to its owner.
  if (
    !panel ||
    !host.contains(panel) ||
    !(trigger.compareDocumentPosition(panel) & Node.DOCUMENT_POSITION_FOLLOWING)
  )
    return;
  const bounds = trigger.getBoundingClientRect();
  const { top, bottom } = safeReadingBounds(viewport);
  if (bounds.top >= top && bounds.bottom <= bottom) return;
  if (state.isAtBottom) stopScroll();
  trigger.scrollIntoView({
    behavior: "instant",
    inline: "nearest",
    block: bounds.height > bottom - top ? "start" : "nearest",
  });
}
