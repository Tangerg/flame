import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, corner, space, surface } from "@/styles/tokens.stylex";
import { SliderPrimitive } from "@/ui/primitives";

const styles = stylex.create({
  root: {
    position: "relative",
    display: "flex",
    height: space.s4,
    width: "calc(var(--spacing) * 36)",
    touchAction: "none",
    userSelect: "none",
    alignItems: "center",
  },
  control: {
    position: "relative",
    display: "flex",
    height: space.s4,
    flexGrow: 1,
    alignItems: "center",
  },
  track: { position: "relative", height: space.s1, flexGrow: 1, backgroundColor: surface.sunken },
  fill: { position: "absolute", height: "100%", backgroundColor: color.accent },
  thumb: {
    display: "block",
    height: "14px",
    width: "14px",
    backgroundColor: surface.canvas,
    boxShadow: "var(--shadow-control)",
    transitionProperty: "translate, scale",
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
          <SliderPrimitive.Indicator {...stylex.props(styles.fill, corner.pill)} />
        </SliderPrimitive.Track>
        <SliderPrimitive.Thumb
          getAriaLabel={() => ariaLabel}
          {...stylex.props(styles.thumb, corner.pill)}
        />
      </SliderPrimitive.Control>
    </SliderPrimitive.Root>
  );
}
