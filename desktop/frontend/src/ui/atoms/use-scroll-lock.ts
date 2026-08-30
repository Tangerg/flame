import type { RefObject } from "react";
import { useCallback, useEffect, useRef } from "react";

/**
 * Call the returned function right BEFORE a height change begins: browser scroll anchoring
 * shifts the viewport when a block resizes mid-scroll, and the sticky-bottom chat scroller
 * reads that transient as a jump. The snapshot is re-asserted on every scroll event until
 * the animation window closes.
 */
export function useScrollLock<T extends HTMLElement = HTMLElement>(
  animatedElementRef: RefObject<T | null>,
) {
  const scrollContainerRef = useRef<HTMLElement | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);

  // A lock in flight at unmount would leave the scrollbar hidden and the padding shim in
  // place on an element that outlives this component.
  useEffect(() => () => cleanupRef.current?.(), []);

  return useCallback(() => {
    cleanupRef.current?.();

    if (!scrollContainerRef.current && animatedElementRef.current) {
      let el: HTMLElement | null = animatedElementRef.current;
      while (el) {
        const { overflowY } = getComputedStyle(el);
        if (overflowY === "scroll" || overflowY === "auto") {
          scrollContainerRef.current = el;
          break;
        }
        el = el.parentElement;
      }
    }

    const scrollContainer = scrollContainerRef.current;
    const animatedElement = animatedElementRef.current;
    if (!scrollContainer || !animatedElement) return;

    const scrollPosition = scrollContainer.scrollTop;
    const previousScrollbarWidth = scrollContainer.style.scrollbarWidth;

    // Hiding the scrollbar collapses its gutter on classic scrollbars, shifting centered
    // content horizontally; compensated with padding on the side it occupied.
    const computed = getComputedStyle(scrollContainer);
    const paddingSide = computed.direction === "rtl" ? "paddingLeft" : "paddingRight";
    const previousPadding = scrollContainer.style[paddingSide];
    const scrollbarSize =
      scrollContainer.offsetWidth -
      scrollContainer.clientWidth -
      parseFloat(computed.borderLeftWidth) -
      parseFloat(computed.borderRightWidth);

    scrollContainer.style.scrollbarWidth = "none";
    if (scrollbarSize > 0) {
      scrollContainer.style[paddingSide] = `${parseFloat(computed[paddingSide]) + scrollbarSize}px`;
    }

    const restoreStyles = () => {
      scrollContainer.style.scrollbarWidth = previousScrollbarWidth;
      scrollContainer.style[paddingSide] = previousPadding;
    };

    const resetPosition = () => {
      scrollContainer.scrollTop = scrollPosition;
    };
    scrollContainer.addEventListener("scroll", resetPosition);

    const timeoutId = setTimeout(
      () => {
        scrollContainer.removeEventListener("scroll", resetPosition);
        restoreStyles();
        cleanupRef.current = null;
      },
      transitionWindowMs(getComputedStyle(animatedElement)),
    );

    cleanupRef.current = () => {
      clearTimeout(timeoutId);
      scrollContainer.removeEventListener("scroll", resetPosition);
      restoreStyles();
    };
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
