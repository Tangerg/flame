import { type FocusEvent } from "react";
import { type StickToBottomInstance } from "use-stick-to-bottom";
export function stopFollowOnReadingFocus(
  event: FocusEvent<HTMLElement>,
  {
    scrollRef,
    state,
    stopScroll,
  }: Pick<StickToBottomInstance, "scrollRef" | "state" | "stopScroll">,
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
