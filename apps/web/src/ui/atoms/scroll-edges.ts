import * as stylex from "@stylexjs/stylex";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
  type RefObject,
} from "react";
const FADE = "24px";

const EPSILON = 1;

export const scrollEdges = stylex.create({
  fade: {
    maskImage: `linear-gradient(to bottom, transparent 0, #000 var(--fade-top, 0px), #000 calc(100% - var(--fade-bottom, 0px)), transparent 100%)`,
    WebkitMaskImage: `linear-gradient(to bottom, transparent 0, #000 var(--fade-top, 0px), #000 calc(100% - var(--fade-bottom, 0px)), transparent 100%)`,
  },
});

interface ScrollEdges {
  onScroll: () => void;
  style: CSSProperties;
  overflowing: boolean;
}

export function useScrollEdges(
  port: RefObject<HTMLElement | null>,
  content: RefObject<HTMLElement | null>,
  active = true,
): ScrollEdges {
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
  }, [port]);

  useEffect(() => {
    if (!active) return;
    const observer = new ResizeObserver(measure);
    if (port.current) observer.observe(port.current);
    if (content.current) observer.observe(content.current);
    return () => observer.disconnect();
  }, [active, measure, port, content]);

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
      onScroll: measure,
      style,
      overflowing: active && edges.overflowing,
    }),
    [measure, style, active, edges.overflowing],
  );
}
