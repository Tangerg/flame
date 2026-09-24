import * as stylex from "@stylexjs/stylex";
import { color, corner, motion, radius, space, surface } from "@/styles/tokens.stylex";
import { SliderPrimitive } from "@/ui/primitives";

const THUMB_WIDTH_PX = 36;

const styles = stylex.create({
  root: { display: "flex", width: "100%", touchAction: "none", userSelect: "none" },
  control: {
    position: "relative",
    display: "flex",
    height: space.s8,
    flexGrow: 1,
    alignItems: "center",
  },
  track: {
    position: "relative",
    height: "100%",
    flexGrow: 1,
    borderRadius: radius.field,
    backgroundColor: surface.sunken,
    boxShadow: "var(--shadow-well)",
  },
  fill: {
    position: "absolute",
    height: "100%",
    borderRadius: radius.field,
    backgroundColor: surface.hover,
  },
  tick: {
    position: "absolute",
    top: "50%",
    height: space.s3,
    width: "2px",
    translate: "-50% -50%",
    backgroundColor: color.fgFaint,
    opacity: 0.45,
  },
  thumb: {
    display: "block",
    height: "calc(100% - var(--spacing) * 1)",
    width: `${THUMB_WIDTH_PX}px`,
    borderRadius: radius.segment,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: `var(--segment-chip-edge, ${surface.field})`,
    backgroundColor: surface.elevated,
    boxShadow: "var(--shadow-raised-chip)",
    transitionProperty: "translate",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },
});

// A choice among a few ordered stops, shown as a well the thumb clicks into. The
// thumb stays inside the well at both ends, so each tick sits where the thumb's
// centre lands for its stop rather than at a plain percentage of the track.
export function StepSlider({
  stops,
  value,
  onValueChange,
  onValueCommitted,
  ariaLabel,
}: {
  stops: readonly string[];
  value: number;
  onValueChange: (index: number) => void;
  onValueCommitted: (index: number) => void;
  ariaLabel: string;
}) {
  const last = Math.max(1, stops.length - 1);
  return (
    <SliderPrimitive.Root
      {...stylex.props(styles.root)}
      value={value}
      min={0}
      max={last}
      step={1}
      thumbAlignment="edge"
      onValueChange={(next) => onValueChange(next as number)}
      onValueCommitted={(next) => onValueCommitted(next as number)}
    >
      <SliderPrimitive.Control {...stylex.props(styles.control)}>
        <SliderPrimitive.Track {...stylex.props(styles.track)}>
          <SliderPrimitive.Indicator {...stylex.props(styles.fill)} />
          {stops.map((stop, index) => (
            <span
              key={stop}
              aria-hidden
              {...stylex.props(styles.tick, corner.pill)}
              style={{
                left: `calc(${THUMB_WIDTH_PX / 2}px + ${index / last} * (100% - ${THUMB_WIDTH_PX}px))`,
              }}
            />
          ))}
        </SliderPrimitive.Track>
        <SliderPrimitive.Thumb
          data-focus-proxy=""
          getAriaLabel={() => ariaLabel}
          getAriaValueText={(_, index) => stops[index] ?? ""}
          {...stylex.props(styles.thumb)}
        />
      </SliderPrimitive.Control>
    </SliderPrimitive.Root>
  );
}
