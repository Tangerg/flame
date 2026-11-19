import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { type RefCallback } from "react";
function allowsScrolling(overflow: string): boolean {
  return overflow === "auto" || overflow === "scroll";
}

function trackHorizontalEnd(node: HTMLElement) {
  const direction = getComputedStyle(node).direction === "rtl" ? -1 : 1;
  let clientWidth = node.clientWidth;
  let scrollWidth = node.scrollWidth;
  let atEnd = false;

  const onScroll = () => {
    // Native clamping can dispatch scroll before the new layout is observed.
    if (
      clientWidth > 0 &&
      scrollWidth > clientWidth + 1 &&
      node.clientWidth === clientWidth &&
      node.scrollWidth === scrollWidth
    ) {
      atEnd = direction * node.scrollLeft >= scrollWidth - clientWidth - 1;
    }
  };
  onScroll();
  node.addEventListener("scroll", onScroll, { passive: true });

  return {
    measure() {
      const nextClientWidth = node.clientWidth;
      const nextScrollWidth = node.scrollWidth;
      if (
        nextClientWidth <= 0 ||
        (nextClientWidth === clientWidth && nextScrollWidth === scrollWidth)
      ) {
        return;
      }
      clientWidth = nextClientWidth;
      scrollWidth = nextScrollWidth;
      if (atEnd) {
        const left = direction * Math.max(0, scrollWidth - clientWidth);
        if (node.scrollLeft !== left) node.scrollTo({ left, behavior: "instant" });
      }
    },
    disconnect() {
      node.removeEventListener("scroll", onScroll);
    },
  };
}

export function useScrollReach({
  retainHorizontalEnd = false,
}: { retainHorizontalEnd?: boolean } = {}): {
  ref: RefCallback<HTMLElement>;
  tabIndex: 0 | undefined;
} {
  const nodeRef = useRef<HTMLElement | null>(null);
  const [reachable, setReachable] = useState(false);

  const measure = useCallback(() => {
    const node = nodeRef.current;
    if (!node) return;
    const style = getComputedStyle(node);
    const next =
      (allowsScrolling(style.overflowX) && node.scrollWidth > node.clientWidth + 1) ||
      (allowsScrolling(style.overflowY) && node.scrollHeight > node.clientHeight + 1);
    setReachable((prev) => (prev === next ? prev : next));
  }, []);

  useLayoutEffect(measure);

  const ref = useCallback(
    (node: HTMLElement | null) => {
      nodeRef.current = node;
      if (!node) return;
      const horizontalEnd = retainHorizontalEnd ? trackHorizontalEnd(node) : null;
      const update = () => {
        horizontalEnd?.measure();
        measure();
      };
      const observer = new ResizeObserver(update);
      const observe = () => {
        observer.disconnect();
        observer.observe(node);
        for (const child of node.children) observer.observe(child);
        update();
      };
      const content = new MutationObserver(observe);
      content.observe(node, { childList: true, characterData: true, subtree: true });
      observe();
      return () => {
        observer.disconnect();
        content.disconnect();
        horizontalEnd?.disconnect();
        if (nodeRef.current === node) nodeRef.current = null;
      };
    },
    [measure, retainHorizontalEnd],
  );

  return { ref, tabIndex: reachable ? 0 : undefined };
}
