import * as stylex from "@stylexjs/stylex";
import { color, space } from "@/styles/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons";

export type StepState = "done" | "active" | "pending";

const styles = stylex.create({
  mark: { display: "grid", height: "1lh", width: space.s4, flexShrink: 0, placeItems: "center" },
  done: { color: color.success },
  active: { color: color.fg },
  pending: { color: color.fgFaint },
});

// One glyph at one size for every standing, so the column reads as a single
// scale and only its ink says how far the work got. The marks it replaces mixed
// a bare tick with two discs, and gave the current step the live-session glow —
// which borrowed a status light to mean "current" and left the column looking
// like a radio group.
const GLYPH: Record<StepState, IconName> = {
  done: "circle-check",
  active: "circle-dot",
  pending: "circle",
};

export function StepMark({ state }: { state: StepState }) {
  return (
    <div {...stylex.props(styles.mark)}>
      <Icon name={GLYPH[state]} size="sm" className={stylex.props(styles[state]).className} />
    </div>
  );
}
