import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/classNames";
import { useScrollLock } from "./use-scroll-lock";

interface Props {
  open: boolean;
  children: ReactNode;
}

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
        "grid grid-cols-[minmax(0,1fr)]",
        "transition-[grid-template-rows] duration-[var(--dur-disclosure)] ease-out",
        open ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}
      onTransitionRun={() => {
        if (open) setRevealed(true);
      }}
    >
      <div inert={!open} className="min-h-0 overflow-clip">
        {(open || revealed) && children}
      </div>
    </div>
  );
}
