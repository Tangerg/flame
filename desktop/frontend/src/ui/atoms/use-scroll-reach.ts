import { useCallback, useLayoutEffect, useRef, useState } from "react";
import type { RefCallback } from "react";

export function useScrollReach(): {
  ref: RefCallback<HTMLElement>;
  tabIndex: 0 | undefined;
} {
  const nodeRef = useRef<HTMLElement | null>(null);
  const [reachable, setReachable] = useState(false);

  const measure = useCallback(() => {
    const node = nodeRef.current;
    if (!node) return;
    const next = node.scrollHeight > node.clientHeight + 1;
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
