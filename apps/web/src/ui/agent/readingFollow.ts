import { type FocusEvent, type MouseEvent, useLayoutEffect } from "react";
import { type StickToBottomInstance } from "use-stick-to-bottom";
type ReadingFollow = Pick<StickToBottomInstance, "scrollRef" | "state" | "stopScroll">;

// Native zoom reports fractional CSS pixels.
const EDGE_TOLERANCE = 0.5;

function withinSafeReading(target: Element, viewport: HTMLElement) {
  const bounds = target.getBoundingClientRect();
  let { top, bottom } = safeReadingBounds(viewport);
  for (
    let parent = target.parentElement;
    parent && parent !== viewport;
    parent = parent.parentElement
  ) {
    if (["auto", "scroll", "hidden", "clip"].includes(getComputedStyle(parent).overflowY)) {
      const clip = parent.getBoundingClientRect();
      top = Math.max(top, clip.top);
      bottom = Math.min(bottom, clip.bottom);
    }
  }
  return (
    bottom > top && bounds.top >= top - EDGE_TOLERANCE && bounds.bottom <= bottom + EDGE_TOLERANCE
  );
}

export function useReadingFocusVisibility(
  {
    scrollRef,
    contentRef,
    state,
    stopScroll,
  }: ReadingFollow & Pick<StickToBottomInstance, "contentRef">,
  attached: boolean,
) {
  useLayoutEffect(() => {
    const viewport = scrollRef.current;
    const content = contentRef.current;
    if (!attached || !viewport || !content) return;
    let frame = safeReadingBounds(viewport);
    let witness: Element | null = null;
    const remember = () => {
      frame = safeReadingBounds(viewport);
      const active = content.ownerDocument.activeElement;
      witness =
        active &&
        active !== content &&
        content.contains(active) &&
        withinSafeReading(active, viewport)
          ? active
          : null;
    };
    const retire = () => {
      witness = null;
    };
    const onScroll = (event: Event) => {
      const next = safeReadingBounds(viewport);
      // WebKit can emit a geometry-induced scroll before ResizeObserver delivers the new frame.
      if (
        event.target === viewport &&
        (next.top !== frame.top || next.bottom !== frame.bottom || next.width !== frame.width)
      )
        return;
      remember();
    };
    const reveal = () => {
      frame = safeReadingBounds(viewport);
      const target = witness;
      if (!target) return;
      if (
        scrollRef.current !== viewport ||
        contentRef.current !== content ||
        target !== content.ownerDocument.activeElement ||
        !content.contains(target) ||
        target.getClientRects().length === 0
      ) {
        retire();
        return;
      }
      if (withinSafeReading(target, viewport)) return;
      const bounds = target.getBoundingClientRect();
      const { top, bottom } = frame;
      if (bottom <= top) return;
      const delta =
        bounds.height > bottom - top || bounds.top < top
          ? bounds.top - top
          : Math.max(0, bounds.bottom - bottom);
      if (delta === 0) return;
      if (state.isAtBottom) stopScroll();
      viewport.scrollBy({
        top: delta > 0 ? Math.ceil(delta) : Math.floor(delta),
        behavior: "instant",
      });
    };
    remember();
    const resize = new ResizeObserver(reveal);
    resize.observe(viewport);
    resize.observe(content);
    content.addEventListener("focusin", remember);
    viewport.addEventListener("scroll", onScroll, { passive: true, capture: true });
    viewport.addEventListener("wheel", retire, { passive: true, capture: true });
    return () => {
      resize.disconnect();
      content.removeEventListener("focusin", remember);
      viewport.removeEventListener("scroll", onScroll, true);
      viewport.removeEventListener("wheel", retire, true);
    };
  }, [attached, scrollRef, contentRef, state, stopScroll]);
}

function safeReadingBounds(viewport: HTMLElement) {
  const frame = viewport.getBoundingClientRect();
  const style = getComputedStyle(viewport);
  return {
    width: frame.width,
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
