import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, corner, motion, space, surface, type, weight } from "@/styles/tokens.stylex";
import { Icon } from "@/ui/icons";

export type StepState = "done" | "active" | "pending";

const styles = stylex.create({
  mark: { display: "grid", height: "1lh", width: space.s4, flexShrink: 0, placeItems: "center" },
  dot: { height: space.s3, width: space.s3 },
  pending: { borderWidth: "1.5px", borderStyle: "solid", borderColor: surface.fieldStrong },
  active: {
    backgroundColor: color.accent,
    boxShadow: "var(--shadow-live-glow)",
    animation: motion.pulseDot,
  },
  done: { color: color.success },
  row: { display: "flex", alignItems: "flex-start", gap: space.s2, paddingBlock: space.s0_5 },
  inkDone: { color: color.fgFaint },
  inkActive: { fontWeight: weight.medium, color: color.fg },
  inkPending: { color: color.fgMuted },
  label: { minWidth: 0, flex: 1, overflowWrap: "break-word" },
  struck: { textDecorationLine: "line-through" },
});

const INK = { done: styles.inkDone, active: styles.inkActive, pending: styles.inkPending } as const;

export function StepMark({ state }: { state: StepState }) {
  return (
    <div {...stylex.props(styles.mark)}>
      {state === "done" && <Icon name="check" size="sm" {...stylex.props(styles.done)} />}
      {state === "active" && <div {...stylex.props(styles.dot, corner.pill, styles.active)} />}
      {state === "pending" && <div {...stylex.props(styles.dot, corner.pill, styles.pending)} />}
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
  const styled = stylex.props(styles.row, type.uiMd, INK[state]);
  return (
    <div
      {...styled}
      aria-current={state === "active" ? "step" : undefined}
      className={cn(styled.className, className)}
    >
      <StepMark state={state} />
      <span {...stylex.props(styles.label, state === "done" && styles.struck)}>{children}</span>
    </div>
  );
}
