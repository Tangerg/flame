import * as stylex from "@stylexjs/stylex";
import { clsx } from "clsx";
import {
  color,
  corner,
  leading,
  motion,
  space,
  surface,
  weight as fontWeight,
} from "./tokens.stylex";
import { Progress as ProgressPrimitive } from "@base-ui/react/progress";
type ProgressWeight = "bar" | "row" | "seam";

interface ProgressBarProps {
  value: number;
  label: string;
  max?: number;
  locale?: string;
  labelVisibility?: "visible" | "hidden";
  weight?: ProgressWeight;
  className?: string;
}

const styles = stylex.create({
  root: { display: "grid", gap: space.s1_5, minWidth: 0 },
  caption: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "baseline",
    justifyContent: "space-between",
    columnGap: space.s2,
    rowGap: space.s1,
    minWidth: 0,
    lineHeight: leading.body,
    fontWeight: fontWeight.regular,
  },
  label: {
    flex: "1 1 12rem",
    minWidth: 0,
    overflowWrap: "anywhere",
    color: color.fg,
  },
  value: {
    flexShrink: 0,
    fontVariantNumeric: "tabular-nums",
    color: color.fgMuted,
  },
  track: {
    overflow: "hidden",
    backgroundColor: surface.surface3,
    outline: {
      default: null,
      "@media (forced-colors: active)": "1px solid CanvasText",
    },
  },
  fill: {
    height: "100%",
    backgroundColor: {
      default: color.accent,
      "@media (forced-colors: active)": "Highlight",
    },
    forcedColorAdjust: {
      default: null,
      "@media (forced-colors: active)": "none",
    },
    transitionProperty: "width",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },
  bar: { height: space.s1_5 },
  row: { height: space.s1 },
  seam: { height: space.s0_5 },
  square: { borderRadius: 0 },
});

const CORNER = {
  bar: corner.pill,
  row: corner.pill,
  seam: styles.square,
} as const;

export function ProgressBar({
  value,
  label,
  max = 100,
  locale,
  labelVisibility = "hidden",
  weight = "bar",
  className,
}: ProgressBarProps) {
  const root = stylex.props(styles.root);
  return (
    <ProgressPrimitive.Root
      value={value}
      max={max}
      locale={locale}
      aria-label={labelVisibility === "hidden" ? label : undefined}
      {...root}
      className={clsx(root.className, className)}
    >
      {labelVisibility === "visible" && (
        <div {...stylex.props(styles.caption)}>
          <ProgressPrimitive.Label {...stylex.props(styles.label)}>
            {label}
          </ProgressPrimitive.Label>
          <ProgressPrimitive.Value {...stylex.props(styles.value)} />
        </div>
      )}
      <ProgressPrimitive.Track
        {...stylex.props(styles.track, styles[weight], CORNER[weight])}
      >
        <ProgressPrimitive.Indicator
          {...stylex.props(styles.fill, CORNER[weight])}
        />
      </ProgressPrimitive.Track>
    </ProgressPrimitive.Root>
  );
}
