import * as stylex from "@stylexjs/stylex";
import { color, space } from "@flame/ui/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons/index";
type StepState = "done" | "active" | "pending";

const styles = stylex.create({
  mark: { display: "grid", height: "1lh", width: space.s4, flexShrink: 0, placeItems: "center" },
  done: { color: color.success },
  active: { color: color.fg },
  pending: { color: color.fgFaint },
});

const GLYPH: Record<StepState, IconName> = {
  done: "circle-check",
  active: "circle-dot",
  pending: "circle",
};

export function StepMark({ state, label }: { state: StepState; label: string }) {
  return (
    <div role="img" aria-label={label} {...stylex.props(styles.mark)}>
      <Icon name={GLYPH[state]} size="sm" className={stylex.props(styles[state]).className} />
    </div>
  );
}
