import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { motion as motionToken } from "@flame/ui/tokens.stylex";
import { useScrollLock } from "@/ui/atoms/use-scroll-lock";
const styles = stylex.create({
  row: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr)",
    transitionProperty: "grid-template-rows",
    transitionDuration: motionToken.med,
    transitionTimingFunction: "var(--ease-out)",
  },
  // The zero minimum keeps WebKit's intrinsic track in sync when open content wraps.
  open: { gridTemplateRows: "minmax(0, 1fr)" },
  shut: { gridTemplateRows: "minmax(0, 0fr)" },
  well: {
    minHeight: 0,
    overflow: "clip",
    transitionProperty: "opacity",
    transitionDuration: motionToken.med,
    transitionTimingFunction: motionToken.easeState,
  },
  lit: { opacity: 1 },
  dim: { opacity: 0 },
});

interface Props {
  open: boolean;
  children: ReactNode;
}

export function useDisclosedContent(open: boolean): boolean {
  const [revealed, setRevealed] = useState(open);
  if (open && !revealed) setRevealed(true);
  return open || revealed;
}

export function Collapsible({ open, children }: Props) {
  const rowRef = useRef<HTMLDivElement>(null);
  const wasOpen = useRef(open);
  const lockScroll = useScrollLock(rowRef);

  useEffect(() => {
    if (wasOpen.current && !open) lockScroll();
    wasOpen.current = open;
  }, [open, lockScroll]);

  return (
    <div ref={rowRef} {...stylex.props(styles.row, open ? styles.open : styles.shut)}>
      <div
        inert={!open}
        data-focus-inset=""
        {...stylex.props(styles.well, open ? styles.lit : styles.dim)}
      >
        {children}
      </div>
    </div>
  );
}
