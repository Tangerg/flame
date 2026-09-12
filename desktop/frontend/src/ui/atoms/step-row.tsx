import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, corner, motion, space, surface, type, weight } from "@/styles/tokens.stylex";
import { Icon } from "@/ui/icons";

export type StepState = "done" | "active" | "pending";

const styles = stylex.create({
  // One LINE tall, not one mark tall. A step that wraps aligns to the start so its mark stays
  // with the sentence it belongs to, and a box of `1lh` then centres the glyph on that line
  // without a magic offset, following the reader's type size and leading for free.
  //
  // THE CONSTRAINT THIS PUTS ON CALLERS: `1lh` resolves against the mark's OWN inherited
  // leading, so a row must set its leading on the row and not on the text beside the mark —
  // otherwise the two disagree about how tall a line is and the mark sizes itself to the wrong
  // one. `ActivePlan` had it on the text, and this measured 3px too tall in its plan pill.
  mark: { display: "grid", height: "1lh", width: space.s4, flexShrink: 0, placeItems: "center" },
  ring: {
    position: "relative",
    height: space.s3,
    width: space.s3,
    borderWidth: "1.5px",
    borderStyle: "solid",
  },
  ringActive: { borderColor: color.accent },
  ringPending: { borderColor: surface.fieldStrong },
  pulse: {
    position: "absolute",
    inset: space.s0_5,
    backgroundColor: color.accent,
    animation: motion.pulseDot,
  },
  done: { color: color.success },
  // `flex-start`, because a step that needs two lines is still one step: centring put the mark
  // half a line below the sentence it marks — measured at 10.1px on a two-line step, which is
  // exactly half the leading.
  row: { display: "flex", alignItems: "flex-start", gap: space.s2, paddingBlock: space.s0_5 },
  inkDone: { color: color.fgFaint },
  inkActive: { fontWeight: weight.medium, color: color.fg },
  inkPending: { color: color.fgMuted },
  // The agent writes the steps, so one can name a path or an identifier with nowhere to
  // break. Measured spilling 1368px out of the plan pane before this.
  label: { minWidth: 0, flex: 1, overflowWrap: "break-word" },
  struck: { textDecorationLine: "line-through" },
});

const INK = { done: styles.inkDone, active: styles.inkActive, pending: styles.inkPending } as const;

export function StepMark({ state }: { state: StepState }) {
  return (
    <div {...stylex.props(styles.mark)}>
      {state === "done" && <Icon name="check" size="sm" {...stylex.props(styles.done)} />}
      {state === "active" && (
        <div {...stylex.props(styles.ring, corner.pill, styles.ringActive)}>
          <div {...stylex.props(styles.pulse, corner.pill)} />
        </div>
      )}
      {state === "pending" && (
        <div {...stylex.props(styles.ring, corner.pill, styles.ringPending)} />
      )}
    </div>
  );
}

export function StepRow({
  state,
  className,
  children,
}: {
  state: StepState;
  className?: string;
  children: ReactNode;
}) {
  const styled = stylex.props(styles.row, type.uiSm, INK[state]);
  return (
    <div {...styled} className={cn(styled.className, className)}>
      <StepMark state={state} />
      <span {...stylex.props(styles.label, state === "done" && styles.struck)}>{children}</span>
    </div>
  );
}
