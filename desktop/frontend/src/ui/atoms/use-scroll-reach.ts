import { useCallback, useLayoutEffect, useRef, useState } from "react";

export function useScrollReach(): {
  ref: (node: HTMLElement | null) => void;
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

  useLayoutEffect(() => {
    const node = nodeRef.current;
    if (!node) return;
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [measure]);

  const ref = useCallback((node: HTMLElement | null) => {
    nodeRef.current = node;
  }, []);

  return { ref, tabIndex: reachable ? 0 : undefined };
}
