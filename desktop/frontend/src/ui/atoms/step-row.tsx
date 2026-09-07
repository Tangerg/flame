import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, corner, motion, space, surface, type, weight } from "@/styles/tokens.stylex";
import { Icon } from "@/ui/icons";

export type StepState = "done" | "active" | "pending";

const styles = stylex.create({
  mark: { display: "grid", height: space.s4, width: space.s4, flexShrink: 0, placeItems: "center" },
  ring: {
    position: "relative",
    height: space.s3,
    width: space.s3,
    borderWidth: "1.5px",
    borderStyle: "solid",
  },
  ringActive: { borderColor: color.accent },
  ringPending: { borderColor: surface.fieldStrong },
  // The dot inside the active ring is the only thing on the row that moves: it is what says
  // this step is the one happening, as opposed to the one that is merely next.
  pulse: {
    position: "absolute",
    inset: space.s0_5,
    backgroundColor: color.accent,
    animation: motion.pulseDot,
  },
  done: { color: color.success },
  row: { display: "flex", alignItems: "center", gap: space.s2, paddingBlock: space.s0_5 },
  inkDone: { color: color.fgFaint },
  inkActive: { fontWeight: weight.medium, color: color.fg },
  inkPending: { color: color.fgMuted },
  label: { minWidth: 0, flex: 1 },
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
