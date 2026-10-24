import { type FocusEvent, type MouseEvent } from "react";
import { type StickToBottomInstance } from "use-stick-to-bottom";
type ReadingFollow = Pick<StickToBottomInstance, "scrollRef" | "state" | "stopScroll">;

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
  const frame = viewport.getBoundingClientRect();
  const style = getComputedStyle(viewport);
  const top = frame.top + (parseFloat(style.scrollPaddingTop) || 0);
  const bottom = frame.bottom - (parseFloat(style.scrollPaddingBottom) || 0);
  // Native focus can reveal its target before a queued resize-follow tick.
  if (bounds.top - movement < top || bounds.bottom - movement > bottom) stopScroll();
}

// Opening a disclosure mounts content whose resize-follow would carry the already-focused
// trigger away; the bubbled click still sees the closed state before that expansion renders.
export function stopFollowOnDisclosureActivation(
  event: MouseEvent<HTMLElement>,
  { scrollRef, state, stopScroll }: ReadingFollow,
) {
  const viewport = scrollRef.current;
  const host = event.currentTarget;
  if (event.defaultPrevented || !state.isAtBottom || !viewport?.contains(host)) return;
  if (!(event.target instanceof Element)) return;
  const trigger = event.target.closest<HTMLButtonElement>('button[aria-expanded="false"]');
  if (!trigger || trigger === host || !host.contains(trigger)) return;
  const popup = trigger.getAttribute("aria-haspopup");
  if (
    trigger.disabled ||
    trigger.getAttribute("aria-disabled") === "true" ||
    trigger.getAttribute("role") === "combobox" ||
    (popup !== null && popup !== "false")
  )
    return;
  stopScroll();
}
