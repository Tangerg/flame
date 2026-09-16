import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { motion as motionToken } from "@/styles/tokens.stylex";
import { useScrollLock } from "./use-scroll-lock";

const styles = stylex.create({
  // The row opens by growing its own track from `0fr` to `1fr`, which is the one way to
  // animate to a height nobody measured: the child keeps its natural size throughout.
  row: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr)",
    transitionProperty: "grid-template-rows",
    transitionDuration: motionToken.med,
    transitionTimingFunction: "var(--ease-out)",
  },
  open: { gridTemplateRows: "1fr" },
  shut: { gridTemplateRows: "0fr" },
  // Height and opacity are one change, the way Codex animates its activity disclosure.
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
  /** Rendered shut as well as open: the node a trigger names through `aria-controls` has to
   *  exist. Defer its CONTENT with `useDisclosedContent`, never its identity. */
  children: ReactNode;
}

/** Whether the disclosed content has ever been asked for. */
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
