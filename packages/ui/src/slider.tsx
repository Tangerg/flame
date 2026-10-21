import * as stylex from "@stylexjs/stylex";
import { clsx as cn } from "clsx";
import { color, corner, space, surface } from "./tokens.stylex";
import { SliderPrimitive } from "./primitives/slider";

const styles = stylex.create({
  root: {
    position: "relative",
    display: "flex",
    height: "var(--control-height-xs)",
    width: "calc(var(--spacing) * 36)",
    touchAction: "none",
    userSelect: "none",
    alignItems: "center",
  },
  control: {
    position: "relative",
    display: "flex",
    height: "var(--control-height-xs)",
    flexGrow: 1,
    alignItems: "center",
  },
  track: {
    position: "relative",
    height: space.s1,
    flexGrow: 1,
    backgroundColor: surface.surface3,
  },
  fill: { position: "absolute", height: "100%", backgroundColor: color.accent },
  thumb: {
    display: "block",
    height: "var(--control-mark-size)",
    width: "var(--control-mark-size)",
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.controlEdge,
    backgroundColor: color.onAccent,
    boxShadow: "var(--shadow-control)",
  },
});

interface SliderProps {
  value: number;
  min?: number;
  max?: number;
  step?: number;
  onValueChange: (value: number) => void;
  ariaLabel: string;
  className?: string;
}

export function Slider({
  value,
  min = 0,
  max = 100,
  step = 1,
  onValueChange,
  ariaLabel,
  className,
}: SliderProps) {
  const root = stylex.props(styles.root);
  return (
    <SliderPrimitive.Root
      {...root}
      className={cn(root.className, className)}
      value={value}
      min={min}
      max={max}
      step={step}
      onValueChange={onValueChange}
    >
      <SliderPrimitive.Control {...stylex.props(styles.control)}>
        <SliderPrimitive.Track {...stylex.props(styles.track, corner.pill)}>
          <SliderPrimitive.Indicator
            {...stylex.props(styles.fill, corner.pill)}
          />
        </SliderPrimitive.Track>
        <SliderPrimitive.Thumb
          data-focus-proxy=""
          getAriaLabel={() => ariaLabel}
          {...stylex.props(styles.thumb, corner.pill)}
        />
      </SliderPrimitive.Control>
    </SliderPrimitive.Root>
  );
}
