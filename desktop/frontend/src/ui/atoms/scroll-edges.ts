import * as stylex from "@stylexjs/stylex";
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";

const FADE = "24px";

/** Sub-pixel scroll offsets mean "at the end" is never exactly zero. */
const EPSILON = 1;

/**
 * A bounded scroller says where it is cut, and only there.
 *
 * A mask is painted against the element's own box and does not scroll, so it lands where the
 * clipping actually happens; two absolutely-positioned gradient overlays cannot — inside
 * `overflow-y: auto` they anchor to the scrolled content origin, so the top one leaves the
 * viewport at the moment it is needed and the bottom one sits below it from the start.
 *
 * It also stops needing to know what is behind it, which an overlay does: an overlay has to
 * name an opaque end colour and then paints the wrong one on any other surface.
 */
export const scrollEdges = stylex.create({
  fade: {
    maskImage: `linear-gradient(to bottom, transparent 0, #000 var(--fade-top, 0px), #000 calc(100% - var(--fade-bottom, 0px)), transparent 100%)`,
    WebkitMaskImage: `linear-gradient(to bottom, transparent 0, #000 var(--fade-top, 0px), #000 calc(100% - var(--fade-bottom, 0px)), transparent 100%)`,
  },
});

export interface ScrollEdges {
  /** The clipping box. Carries `scrollEdges.fade`, `onScroll` and `style`. */
  port: React.RefObject<HTMLDivElement | null>;
  /** What grows inside it, watched so a streamed line re-measures without a scroll event. */
  content: React.RefObject<HTMLDivElement | null>;
  onScroll: () => void;
  style: CSSProperties;
  overflowing: boolean;
  /** How far the port is from its own end, in pixels. */
  distanceFromEnd: () => number;
  scrollToEnd: () => void;
}

/**
 * `active` is the caller's own gate — a collapsed disclosure has a zero-height port, and every
 * answer measured through one is a lie about the content it is hiding.
 */
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

  // Measured once here and not left to the observer's own first delivery: whether the port
  // overflows decides whether it is in the tab order, and a keyboard reaching for it on the
  // frame it appeared must not find it missing.
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

  // Read through `active` rather than cleared on the way in, so reopening shows the mask the
  // content already earned instead of one frame without it.
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
