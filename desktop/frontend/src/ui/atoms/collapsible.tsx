import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/classNames";
import { useScrollLock } from "./use-scroll-lock";

interface Props {
  open: boolean;
  children: ReactNode;
}

/**
 * Vertical expand/collapse via `grid-template-rows: 0fr ↔ 1fr`. Use THIS, not Framer
 * Motion `height: "auto"`, for anything that expands inside the message stream: FM
 * measures "auto" by briefly inflating the element, and the chat scroller's
 * `use-stick-to-bottom` ResizeObserver reads that transient as a shrink and clamps to top.
 *
 * Children stay mounted after first open so the close animates, hence `inert` on the
 * collapsed row — clipped content is still focusable and still read aloud.
 */
export function Collapsible({ open, children }: Props) {
  const [revealed, setRevealed] = useState(open);
  const rowRef = useRef<HTMLDivElement>(null);
  const wasOpen = useRef(open);
  const lockScroll = useScrollLock(rowRef);

  useEffect(() => {
    if (wasOpen.current && !open) lockScroll();
    wasOpen.current = open;
  }, [open, lockScroll]);

  return (
    <div
      ref={rowRef}
      className={cn(
        // Column stated explicitly: left to `auto` an implicit grid sizes to the widest
        // child, so a long path in a nested row pushes the card past the reading column.
        "grid grid-cols-[minmax(0,1fr)]",
        "transition-[grid-template-rows] duration-[var(--dur-disclosure)] ease-out",
        open ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}
      onTransitionRun={() => {
        if (open) setRevealed(true);
      }}
    >
      {/* `clip`, not `hidden`: `hidden` makes this a scroll container, which becomes the
          scrollport a `sticky` descendant measures against — a port that never scrolls. */}
      <div inert={!open} className="min-h-0 overflow-clip">
        {(open || revealed) && children}
      </div>
    </div>
  );
}
