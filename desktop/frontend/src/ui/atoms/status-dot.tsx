import * as stylex from "@stylexjs/stylex";
import type { DotTone } from "@/lib/tone";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space } from "@/styles/tokens.stylex";

const styles = stylex.create({
  base: {
    display: "inline-block",
    height: space.s1_5,
    width: space.s1_5,
    flexShrink: 0,
    borderRadius: radius.pill,
  },
  idle: { backgroundColor: color.fgFaint },
  // The only dot that says something is happening right now, so the only one that moves.
  running: {
    backgroundColor: color.accent,
    boxShadow: "var(--shadow-live-glow)",
    animation: motion.pulseDot,
  },
  waiting: { backgroundColor: color.warning },
  ok: { backgroundColor: color.success },
  err: { backgroundColor: color.negative },
});

export function StatusDot({ tone = "idle", className }: { tone?: DotTone; className?: string }) {
  const styled = stylex.props(styles.base, styles[tone]);
  return <span aria-hidden="true" {...styled} className={cn(styled.className, className)} />;
}
