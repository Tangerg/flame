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
  // The track's height is the whole animation, and a track growing past text that is already
  // at full ink reads as a reveal from behind an edge rather than as a thing opening. Codex
  // animates `{ height, opacity }` as one change on its activity disclosure; this is the same
  // change said in the two properties a grid track can carry.
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
  /**
   * The disclosed content. It is rendered on every open state, shut included: what a row this
   * size can afford to defer is what a caller puts inside its own node, and the node itself
   * has to survive being shut — a trigger naming it through `aria-controls` is otherwise
   * pointing at an element that does not exist until the row is first opened. Fourteen rows
   * across the fixtures were, which is a control announcing that it operates something nothing
   * can find. `useDisclosedContent` below is the latch a caller defers WITH.
   */
  children: ReactNode;
}

/**
 * Whether the disclosed content has ever been asked for.
 *
 * The latch belongs to the caller rather than to `Collapsible`, because only the caller knows
 * which of its nodes carries the region's identity and must therefore outlive being shut.
 */
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
