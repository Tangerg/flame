import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface } from "@/styles/tokens.stylex";
import { ProgressPrimitive } from "@/ui/primitives";

/**
 * A meter, in the three weights the product shows one.
 *
 * `bar` stands on its own. `row` sits inside a line of statistics beside its label. `seam` is
 * the hairline a disclosure carries along its own edge — square, because it is continuous
 * with that edge rather than an object lying on it.
 *
 * These were three call sites each passing a height and, for the seam, a second class for the
 * indicator's corners. Height and shape ARE the meter's decision; where it sits is not, so
 * `className` still takes the margins and the flex that place it.
 */
type ProgressWeight = "bar" | "row" | "seam";

interface ProgressBarProps {
  value: number;
  label: string;
  weight?: ProgressWeight;
  className?: string;
}

const styles = stylex.create({
  track: { overflow: "hidden", backgroundColor: surface.sunken },
  fill: {
    height: "100%",
    backgroundColor: color.accent,
    transitionProperty: "width",
    transitionDuration: motion.fast,
  },
  bar: { height: space.s1_5, borderRadius: radius.pill },
  row: { height: space.s1, borderRadius: radius.pill },
  seam: { height: space.s0_5, borderRadius: 0 },
  barFill: { borderRadius: radius.pill },
  rowFill: { borderRadius: radius.pill },
  seamFill: { borderRadius: 0 },
});

const FILL: Record<ProgressWeight, keyof typeof styles> = {
  bar: "barFill",
  row: "rowFill",
  seam: "seamFill",
};

export function ProgressBar({ value, label, weight = "bar", className }: ProgressBarProps) {
  const bounded = Math.max(0, Math.min(100, value));
  const track = stylex.props(styles.track, styles[weight]);
  return (
    <ProgressPrimitive.Root
      value={bounded}
      aria-label={label}
      {...track}
      className={cn(track.className, className)}
    >
      <ProgressPrimitive.Indicator
        {...stylex.props(styles.fill, styles[FILL[weight]])}
        style={{ width: `${bounded}%` }}
      />
    </ProgressPrimitive.Root>
  );
}
