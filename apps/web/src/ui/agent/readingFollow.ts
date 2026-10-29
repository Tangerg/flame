import { type FocusEvent, type MouseEvent, useLayoutEffect } from "react";
import { type StickToBottomInstance } from "use-stick-to-bottom";
import { readingUnitFor } from "@flame/ui";
type ReadingFollow = Pick<StickToBottomInstance, "scrollRef" | "state" | "stopScroll">;

// Native zoom reports fractional CSS pixels.
const EDGE_TOLERANCE = 0.5;
const CLIPPING_OVERFLOW = ["auto", "scroll", "hidden", "clip"];

function readingTargetBounds(target: Element) {
  const bounds = target.getBoundingClientRect();
  const style = getComputedStyle(target);
  const start = parseFloat(style.scrollMarginTop) || 0;
  const end = parseFloat(style.scrollMarginBottom) || 0;
  return {
    top: bounds.top - start,
    bottom: bounds.bottom + end,
    height: bounds.height + start + end,
    marginStart: start,
    marginEnd: end,
  };
}

function readingFocusContext(target: Element, viewport: HTMLElement) {
  const focused = readingTargetBounds(target);
  // A larger unit must retain the focused control's own reserved clearance.
  const contextBounds = (unit: Element) => {
    const bounds = readingTargetBounds(unit);
    const top = Math.min(bounds.top, focused.top);
    const bottom = Math.max(bounds.bottom, focused.bottom);
    return {
      top,
      bottom,
      height: bottom - top,
      marginStart: bounds.marginStart + bounds.top - top,
      marginEnd: bounds.marginEnd + bottom - bounds.bottom,
    };
  };
  const frame = safeReadingBounds(viewport);
  const unit = readingUnitFor(target, viewport, (candidate) => {
    const height = candidate.getBoundingClientRect().height;
    if (height <= 0 || contextBounds(candidate).height > frame.bottom - frame.top) return false;
    for (
      let parent = candidate.parentElement;
      parent && parent !== viewport;
      parent = parent.parentElement
    ) {
      if (
        CLIPPING_OVERFLOW.includes(getComputedStyle(parent).overflowY) &&
        height > parent.getBoundingClientRect().height
      )
        return false;
    }
    return true;
  });
  return { unit, bounds: unit === target ? focused : contextBounds(unit) };
}

function withinSafeReading(
  target: Element,
  viewport: HTMLElement,
  bounds = readingTargetBounds(target),
) {
  let { top, bottom } = safeReadingBounds(viewport);
  for (
    let parent = target.parentElement;
    parent && parent !== viewport;
    parent = parent.parentElement
  ) {
    if (CLIPPING_OVERFLOW.includes(getComputedStyle(parent).overflowY)) {
      const clip = parent.getBoundingClientRect();
      // Scroll margins reserve viewport space; ancestor clips still test the actual target.
      top = Math.max(top, clip.top - bounds.marginStart);
      bottom = Math.min(bottom, clip.bottom + bounds.marginEnd);
    }
  }
  if (bottom <= top || bounds.top < top - EDGE_TOLERANCE) return false;
  // Start alignment rounds toward visible content at integer native scroll offsets.
  return bounds.height > bottom - top
    ? bounds.top < top + 1
    : bounds.bottom <= bottom + EDGE_TOLERANCE;
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
    const measure = () => {
      const next = safeReadingBounds(viewport);
      viewport.style.setProperty(
        "--reading-frame-height",
        `${Math.max(0, next.bottom - next.top)}px`,
      );
      return next;
    };
    let frame = measure();
    let observedScrollTop = viewport.scrollTop;
    let witness: Element | null = null;
    let pressedTarget: Element | null = null;
    const remember = () => {
      frame = measure();
      observedScrollTop = viewport.scrollTop;
      const active = content.ownerDocument.activeElement;
      witness = null;
      if (active && active !== content && content.contains(active)) {
        const { unit, bounds } = readingFocusContext(active, viewport);
        if (withinSafeReading(active, viewport) && withinSafeReading(unit, viewport, bounds))
          witness = active;
      }
    };
    const retire = () => {
      witness = null;
    };
    const onScroll = (event: Event) => {
      const next = safeReadingBounds(viewport);
      // Native zoom can notify before its frame or content reflow is observed, without moving.
      if (event.target === viewport) {
        const moved = viewport.scrollTop !== observedScrollTop;
        observedScrollTop = viewport.scrollTop;
        if (
          !moved ||
          next.top !== frame.top ||
          next.bottom !== frame.bottom ||
          next.width !== frame.width
        )
          return;
      }
      remember();
    };
    const reveal = () => {
      frame = measure();
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
      const { unit, bounds } = readingFocusContext(target, viewport);
      if (withinSafeReading(target, viewport) && withinSafeReading(unit, viewport, bounds)) return;
      const { top, bottom } = frame;
      if (bottom <= top) return;
      const alignStart = bounds.height > bottom - top || bounds.top < top;
      const delta = alignStart ? bounds.top - top : Math.max(0, bounds.bottom - bottom);
      if (delta === 0) return;
      if (state.isAtBottom) stopScroll();
      // Absolute delivery keeps WebKit's async scroll position in sync after native page zoom.
      viewport.scrollTo({
        top: viewport.scrollTop + (alignStart ? Math.floor(delta) : Math.ceil(delta)),
        behavior: "instant",
      });
    };
    const onPointerDown = (event: PointerEvent) => {
      pressedTarget = event.target instanceof Element ? event.target : null;
    };
    const clearPress = () => {
      pressedTarget = null;
    };
    const onFocus = () => {
      const active = content.ownerDocument.activeElement;
      const pressed = pressedTarget;
      clearPress();
      if (!active || active === content || !content.contains(active)) {
        retire();
        return;
      }
      const pointerFocus =
        pressed &&
        (active.contains(pressed) ||
          pressed.closest("label, [data-focus-proxy]")?.contains(active));
      // Modal return focus suppresses native scrolling. The reader owns its reserved clearance,
      // but must not move a pointer target before the activating press completes.
      if (!pointerFocus) {
        witness = active;
        reveal();
      }
      remember();
    };
    remember();
    const resize = new ResizeObserver(reveal);
    resize.observe(viewport);
    resize.observe(content);
    content.ownerDocument.addEventListener("focusin", onFocus);
    content.addEventListener("pointerdown", onPointerDown, true);
    content.ownerDocument.addEventListener("pointerup", clearPress, true);
    content.ownerDocument.addEventListener("pointercancel", clearPress, true);
    viewport.addEventListener("scroll", onScroll, { passive: true, capture: true });
    viewport.addEventListener("wheel", retire, { passive: true, capture: true });
    return () => {
      resize.disconnect();
      viewport.style.removeProperty("--reading-frame-height");
      content.ownerDocument.removeEventListener("focusin", onFocus);
      content.removeEventListener("pointerdown", onPointerDown, true);
      content.ownerDocument.removeEventListener("pointerup", clearPress, true);
      content.ownerDocument.removeEventListener("pointercancel", clearPress, true);
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
  const { bounds } = readingFocusContext(target, viewport);
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
  const bounds = readingTargetBounds(trigger);
  const { top, bottom } = safeReadingBounds(viewport);
  if (bounds.top >= top && bounds.bottom <= bottom) return;
  if (state.isAtBottom) stopScroll();
  trigger.scrollIntoView({
    behavior: "instant",
    inline: "nearest",
    block: bounds.height > bottom - top ? "start" : "nearest",
  });
}
