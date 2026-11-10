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
    const frameState = { value: measure() };
    const observedScrollTopState = { value: viewport.scrollTop };
    const witnessState: { value: Element | null } = { value: null };
    const pressedTargetState: { value: Element | null } = { value: null };
    const queuedRevealState = { value: 0 };
    const remember = () => {
      frameState.value = measure();
      observedScrollTopState.value = viewport.scrollTop;
      const active = content.ownerDocument.activeElement;
      witnessState.value = null;
      if (active && active !== content && content.contains(active)) {
        const { unit, bounds } = readingFocusContext(active, viewport);
        if (withinSafeReading(active, viewport) && withinSafeReading(unit, viewport, bounds))
          witnessState.value = active;
      }
    };
    const retire = () => {
      witnessState.value = null;
    };
    const onScroll = (event: Event) => {
      // A focused reader's internal scroll does not move its frame during reflow.
      if (event.target !== viewport && event.target === content.ownerDocument.activeElement) return;
      const next = safeReadingBounds(viewport);
      // Native zoom can notify before its frame or content reflow is observed, without moving.
      if (event.target === viewport) {
        const moved = viewport.scrollTop !== observedScrollTopState.value;
        observedScrollTopState.value = viewport.scrollTop;
        if (!moved) return;
      }
      // Layout-driven scrolling before the queued reveal must not re-witness a displaced target.
      if (queuedRevealState.value) return;
      // Defer re-witnessing until resize has observed the changed frame.
      if (
        next.top !== frameState.value.top ||
        next.bottom !== frameState.value.bottom ||
        next.width !== frameState.value.width
      )
        return;
      remember();
    };
    const reveal = () => {
      frameState.value = measure();
      const target = witnessState.value;
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
      if (withinSafeReading(target, viewport) && withinSafeReading(unit, viewport, bounds)) {
        // Visible focus can still be carried away by the resize-follow tick this reflow queued.
        stopFollowHidingReading(bounds, viewport, { state, stopScroll });
        return;
      }
      const { top, bottom } = frameState.value;
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
      // Its delayed native scroll event still belongs to this reveal, even after another reflow.
      observedScrollTopState.value = viewport.scrollTop;
    };
    const onPointerDown = (event: PointerEvent) => {
      pressedTargetState.value = event.target instanceof Element ? event.target : null;
    };
    const clearPress = () => {
      pressedTargetState.value = null;
    };
    const onWheel = (event: WheelEvent) => {
      const reader = content.ownerDocument.activeElement;
      if (
        reader instanceof HTMLElement &&
        reader !== viewport &&
        event.target instanceof Node &&
        reader.contains(event.target) &&
        ["auto", "scroll"].includes(getComputedStyle(reader).overflowY) &&
        withinSafeReading(reader, viewport)
      ) {
        // A consumed wheel can report its scroll only after the next native reflow.
        const remaining =
          event.deltaY < 0
            ? reader.scrollTop
            : reader.scrollHeight - reader.clientHeight - reader.scrollTop;
        if (event.deltaY !== 0 && remaining > 0) return;
      }
      retire();
    };
    const onFocus = () => {
      const active = content.ownerDocument.activeElement;
      const pressed = pressedTargetState.value;
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
        witnessState.value = active;
        reveal();
      }
      remember();
    };
    const window = content.ownerDocument.defaultView!;
    // Measuring during delivery resizes the reading frame's descendants in the same rendering
    // update, and WebKit can deliver layout-driven scrolling after ResizeObserver.
    const revealAfterLayout = () => {
      if (queuedRevealState.value) return;
      queuedRevealState.value = window.requestAnimationFrame(() => {
        queuedRevealState.value = 0;
        reveal();
      });
    };
    remember();
    const resize = new ResizeObserver(revealAfterLayout);
    resize.observe(viewport);
    resize.observe(content);
    content.ownerDocument.addEventListener("focusin", onFocus);
    content.addEventListener("pointerdown", onPointerDown, true);
    content.ownerDocument.addEventListener("pointerup", clearPress, true);
    content.ownerDocument.addEventListener("pointercancel", clearPress, true);
    viewport.addEventListener("scroll", onScroll, { passive: true, capture: true });
    viewport.addEventListener("wheel", onWheel, { passive: true, capture: true });
    // The retained DOM keeps its last measured height; clearing it would let a revealed reader
    // expand and clamp native scroll positions before this effect remeasures.
    return () => {
      resize.disconnect();
      if (queuedRevealState.value) window.cancelAnimationFrame(queuedRevealState.value);
      content.ownerDocument.removeEventListener("focusin", onFocus);
      content.removeEventListener("pointerdown", onPointerDown, true);
      content.ownerDocument.removeEventListener("pointerup", clearPress, true);
      content.ownerDocument.removeEventListener("pointercancel", clearPress, true);
      viewport.removeEventListener("scroll", onScroll, true);
      viewport.removeEventListener("wheel", onWheel, true);
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
  // Native focus can reveal its target before a queued resize-follow tick.
  stopFollowHidingReading(readingFocusContext(target, viewport).bounds, viewport, {
    state,
    stopScroll,
  });
}

function stopFollowHidingReading(
  bounds: { top: number; bottom: number },
  viewport: HTMLElement,
  { state, stopScroll }: Pick<ReadingFollow, "state" | "stopScroll">,
) {
  if (!state.isAtBottom) return;
  const movement = state.targetScrollTop - viewport.scrollTop;
  if (movement <= 0) return;
  const { top, bottom } = safeReadingBounds(viewport);
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
