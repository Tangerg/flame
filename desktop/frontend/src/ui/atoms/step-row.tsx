import * as stylex from "@stylexjs/stylex";
import { color, corner, motion, space, surface } from "@/styles/tokens.stylex";
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
});

export function StepMark({ state }: { state: StepState }) {
  return (
    <div {...stylex.props(styles.mark)}>
      {state === "done" && <Icon name="check" size="sm" {...stylex.props(styles.done)} />}
      {state === "active" && <div {...stylex.props(styles.dot, corner.pill, styles.active)} />}
      {state === "pending" && <div {...stylex.props(styles.dot, corner.pill, styles.pending)} />}
    </div>
  );
}
