import { type RefObject, useCallback } from "react";

const containerLocks = new WeakMap<HTMLElement, { leases: number; restore: () => void }>();

function acquireScrollLock(container: HTMLElement): () => void {
  let lock = containerLocks.get(container);
  if (!lock) {
    const scrollPosition = container.scrollTop;
    const previousScrollbarWidth = container.style.scrollbarWidth;
    const computed = getComputedStyle(container);
    const paddingSide = computed.direction === "rtl" ? "paddingLeft" : "paddingRight";
    const previousPadding = container.style[paddingSide];
    const scrollbarSize =
      container.offsetWidth -
      container.clientWidth -
      parseFloat(computed.borderLeftWidth) -
      parseFloat(computed.borderRightWidth);
    const resetPosition = () => {
      container.scrollTop = scrollPosition;
    };
    container.style.scrollbarWidth = "none";
    if (scrollbarSize > 0) {
      container.style[paddingSide] = `${parseFloat(computed[paddingSide]) + scrollbarSize}px`;
    }
    container.addEventListener("scroll", resetPosition, { passive: true });
    lock = {
      leases: 0,
      restore: () => {
        container.removeEventListener("scroll", resetPosition);
        container.style.scrollbarWidth = previousScrollbarWidth;
        container.style[paddingSide] = previousPadding;
      },
    };
    containerLocks.set(container, lock);
  }
  const owned = lock;
  owned.leases += 1;
  let released = false;
  return () => {
    if (released) return;
    released = true;
    owned.leases -= 1;
    if (owned.leases === 0) {
      containerLocks.delete(container);
      owned.restore();
    }
  };
}

export function useScrollLock<T extends HTMLElement = HTMLElement>(
  animatedElementRef: RefObject<T | null>,
) {
  return useCallback(() => {
    const animatedElement = animatedElementRef.current;
    if (!animatedElement) return;
    let container: HTMLElement | null = animatedElement;
    while (container) {
      const { overflowY } = getComputedStyle(container);
      if (overflowY === "scroll" || overflowY === "auto") break;
      container = container.parentElement;
    }
    if (!container) return;
    const release = acquireScrollLock(container);
    let finished = false;
    const finish = () => {
      if (finished) return;
      finished = true;
      clearTimeout(timeoutId);
      animatedElement.removeEventListener("transitionend", onTransition);
      animatedElement.removeEventListener("transitioncancel", onTransition);
      release();
    };
    const onTransition = (event: Event) => {
      if (event.target === animatedElement) finish();
    };
    animatedElement.addEventListener("transitionend", onTransition);
    animatedElement.addEventListener("transitioncancel", onTransition);
    const timeoutId = setTimeout(finish, transitionWindowMs(getComputedStyle(animatedElement)));
    return finish;
  }, [animatedElementRef]);
}

function transitionWindowMs(style: CSSStyleDeclaration): number {
  const durations = style.transitionDuration.split(",").map(cssTimeMs);
  const delays = style.transitionDelay.split(",").map(cssTimeMs);
  return durations.reduce(
    (longest, duration, index) =>
      Math.max(longest, duration + (delays[index % delays.length] ?? 0)),
    0,
  );
}

function cssTimeMs(value: string): number {
  const time = Number.parseFloat(value);
  if (!Number.isFinite(time)) return 0;
  return value.trim().endsWith("ms") ? time : time * 1_000;
}
