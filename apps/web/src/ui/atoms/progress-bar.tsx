import * as stylex from "@stylexjs/stylex";
import { clamp } from "@/lib/clamp";
import { cn } from "@/lib/classNames";
import { color, corner, motion, space, surface } from "@flame/ui/tokens.stylex";
import { Progress as ProgressPrimitive } from "@base-ui/react/progress";
type ProgressWeight = "bar" | "row" | "seam";

interface ProgressBarProps {
  value: number;
  label: string;
  weight?: ProgressWeight;
  className?: string;
}

const styles = stylex.create({
  track: { overflow: "hidden", backgroundColor: surface.surface3 },
  fill: {
    height: "100%",
    backgroundColor: color.accent,
    transitionProperty: "width",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },
  bar: { height: space.s1_5 },
  row: { height: space.s1 },
  seam: { height: space.s0_5 },
  square: { borderRadius: 0 },
});

const CORNER = { bar: corner.pill, row: corner.pill, seam: styles.square } as const;

export function ProgressBar({ value, label, weight = "bar", className }: ProgressBarProps) {
  const bounded = clamp(value, 0, 100);
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
