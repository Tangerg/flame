import * as stylex from "@stylexjs/stylex";
import { type DotTone } from "@/lib/tone";
import { cn } from "@/lib/classNames";
import { color, corner, motion, space } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  base: {
    display: "inline-block",
    height: space.s1_5,
    width: space.s1_5,
    flexShrink: 0,
  },
  idle: { backgroundColor: color.fgFaint },
  running: {
    backgroundColor: color.accentText,
    boxShadow: "var(--shadow-live-glow)",
    animation: motion.pulseDot,
  },
  waiting: { backgroundColor: color.warning },
  ok: { backgroundColor: color.success },
  err: { backgroundColor: color.negative },
  ring: {
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: color.accentText,
    animation: motion.pulseDot,
  },
});

export function StatusDot({
  tone = "idle",
  ring = false,
  label,
  className,
}: {
  tone?: DotTone;
  ring?: boolean;
  label?: string;
  className?: string;
}) {
  const styled = stylex.props(styles.base, corner.pill, ring ? styles.ring : styles[tone]);
  const a11y = label
    ? { role: "img" as const, "aria-label": label, title: label }
    : { "aria-hidden": "true" as const };
  return <span {...a11y} {...styled} className={cn(styled.className, className)} />;
}
