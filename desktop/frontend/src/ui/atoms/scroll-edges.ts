import * as stylex from "@stylexjs/stylex";
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";

const FADE = "24px";

const EPSILON = 1;

export const scrollEdges = stylex.create({
  fade: {
    maskImage: `linear-gradient(to bottom, transparent 0, #000 var(--fade-top, 0px), #000 calc(100% - var(--fade-bottom, 0px)), transparent 100%)`,
    WebkitMaskImage: `linear-gradient(to bottom, transparent 0, #000 var(--fade-top, 0px), #000 calc(100% - var(--fade-bottom, 0px)), transparent 100%)`,
  },
});

interface ScrollEdges {
  port: React.RefObject<HTMLDivElement | null>;
  content: React.RefObject<HTMLDivElement | null>;
  onScroll: () => void;
  style: CSSProperties;
  overflowing: boolean;
  distanceFromEnd: () => number;
  scrollToEnd: () => void;
}

export function useScrollEdges(active = true): ScrollEdges {
  const port = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({ top: false, bottom: false, overflowing: false });

  const measure = useCallback(() => {
    const el = port.current;
    if (!el) return;
    const slack = Math.max(0, el.scrollHeight - el.clientHeight);
    const next =
      slack <= EPSILON
        ? { top: false, bottom: false, overflowing: false }
        : {
            top: el.scrollTop > EPSILON,
            bottom: el.scrollTop < slack - EPSILON,
            overflowing: true,
          };
    setEdges((prev) =>
      prev.top === next.top && prev.bottom === next.bottom && prev.overflowing === next.overflowing
        ? prev
        : next,
    );
  }, []);

  useEffect(() => {
    if (!active) return;
    measure();
    const observer = new ResizeObserver(measure);
    if (port.current) observer.observe(port.current);
    if (content.current) observer.observe(content.current);
    return () => observer.disconnect();
  }, [active, measure]);

  const distanceFromEnd = useCallback(() => {
    const el = port.current;
    if (!el) return 0;
    return Math.max(0, el.scrollHeight - el.scrollTop - el.clientHeight);
  }, []);

  const scrollToEnd = useCallback(() => {
    const el = port.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, []);

  const top = active && edges.top;
  const bottom = active && edges.bottom;
  const style = useMemo(
    () =>
      ({
        "--fade-top": top ? FADE : "0px",
        "--fade-bottom": bottom ? FADE : "0px",
      }) as CSSProperties,
    [top, bottom],
  );

  return useMemo(
    () => ({
      port,
      content,
      onScroll: measure,
      style,
      overflowing: active && edges.overflowing,
      distanceFromEnd,
      scrollToEnd,
    }),
    [measure, style, active, edges.overflowing, distanceFromEnd, scrollToEnd],
  );
}
