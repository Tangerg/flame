import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, corner, motion, space, surface } from "@/styles/tokens.stylex";
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
  bar: { height: space.s1_5 },
  row: { height: space.s1 },
  seam: { height: space.s0_5 },
  // A seam runs edge to edge, so it has no corner to round; the other two are capsules.
  square: { borderRadius: 0 },
});

// The fill wears the track's corner. It used to be spelled twice, once per element, which is
// two places to disagree about one shape.
const CORNER = { bar: corner.pill, row: corner.pill, seam: styles.square } as const;

export function ProgressBar({ value, label, weight = "bar", className }: ProgressBarProps) {
  const bounded = Math.max(0, Math.min(100, value));
  const track = stylex.props(styles.track, styles[weight], CORNER[weight]);
  return (
    <ProgressPrimitive.Root
      value={bounded}
      aria-label={label}
      {...track}
      className={cn(track.className, className)}
    >
      <ProgressPrimitive.Indicator
        {...stylex.props(styles.fill, CORNER[weight])}
        style={{ width: `${bounded}%` }}
      />
    </ProgressPrimitive.Root>
  );
}
