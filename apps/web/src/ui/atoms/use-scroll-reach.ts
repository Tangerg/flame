import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { type RefCallback } from "react";
function allowsScrolling(overflow: string): boolean {
  return overflow === "auto" || overflow === "scroll";
}

export function useScrollReach(
  trackViewport?: (node: HTMLElement) => { measure(): void; disconnect(): void },
): {
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
      const tracking = trackViewport?.(node);
      const update = () => {
        tracking?.measure();
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
        tracking?.disconnect();
        if (nodeRef.current === node) nodeRef.current = null;
      };
    },
    [measure, trackViewport],
  );

  return { ref, tabIndex: reachable ? 0 : undefined };
}
