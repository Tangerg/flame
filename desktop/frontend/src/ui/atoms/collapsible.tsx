import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { motion } from "@/styles/tokens.stylex";
import { useScrollLock } from "./use-scroll-lock";

const styles = stylex.create({
  // The row opens by growing its own track from `0fr` to `1fr`, which is the one way to
  // animate to a height nobody measured: the child keeps its natural size throughout.
  row: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr)",
    transitionProperty: "grid-template-rows",
    transitionDuration: motion.med,
    transitionTimingFunction: "var(--ease-out)",
  },
  open: { gridTemplateRows: "1fr" },
  shut: { gridTemplateRows: "0fr" },
  well: { minHeight: 0, overflow: "clip" },
});

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
      {...stylex.props(styles.row, open ? styles.open : styles.shut)}
      onTransitionRun={() => {
        if (open) setRevealed(true);
      }}
    >
      <div inert={!open} {...stylex.props(styles.well)}>
        {(open || revealed) && children}
      </div>
    </div>
  );
}
