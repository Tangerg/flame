import { useCallback, useLayoutEffect, useRef, useState } from "react";
import type { RefCallback } from "react";

function allowsScrolling(overflow: string): boolean {
  return overflow === "auto" || overflow === "scroll";
}

export function useScrollReach(): {
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
      const observer = new ResizeObserver(measure);
      const observe = () => {
        observer.disconnect();
        observer.observe(node);
        for (const child of node.children) observer.observe(child);
        measure();
      };
      const content = new MutationObserver(observe);
      content.observe(node, { childList: true, characterData: true, subtree: true });
      observe();
      return () => {
        observer.disconnect();
        content.disconnect();
        if (nodeRef.current === node) nodeRef.current = null;
      };
    },
    [measure],
  );

  return { ref, tabIndex: reachable ? 0 : undefined };
}
