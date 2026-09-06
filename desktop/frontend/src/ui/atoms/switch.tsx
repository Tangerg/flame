import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, corner, motion, space, surface } from "@/styles/tokens.stylex";
import { SwitchPrimitive } from "@/ui/primitives";

const styles = stylex.create({
  track: {
    position: "relative",
    display: "inline-flex",
    height: space.s5,
    width: space.s8,
    flexShrink: 0,
    alignItems: "center",
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    transitionProperty: "color, background-color, border-color",
    transitionDuration: motion.color,
    cursor: { default: null, ":disabled": "not-allowed" },
    opacity: { default: null, ":disabled": 0.5 },
  },
  on: { borderColor: color.accent, backgroundColor: color.accent },
  off: { borderColor: surface.field, backgroundColor: surface.sunken },
  // The thumb travels by `translate` rather than by moving in the layout, so the slide is one
  // compositable property and the track never reflows mid-gesture.
  thumb: {
    display: "block",
    height: space.s4,
    width: space.s4,
    backgroundColor: { default: surface.canvas, ":is([data-checked])": color.onAccent },
    boxShadow: "var(--shadow-control)",
    transitionProperty: "translate",
    transitionDuration: motion.fast,
    translate: { default: space.s0_5, ":is([data-checked])": "14px" },
  },
});

interface SwitchProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  ariaLabel?: string;
  className?: string;
}

export function Switch({ checked, onCheckedChange, disabled, ariaLabel, className }: SwitchProps) {
  const track = stylex.props(styles.track, corner.pill, checked ? styles.on : styles.off);
  return (
    <SwitchPrimitive.Root
      checked={checked}
      onCheckedChange={onCheckedChange}
      disabled={disabled}
      aria-label={ariaLabel}
      {...track}
      className={cn(track.className, className)}
    >
      <SwitchPrimitive.Thumb {...stylex.props(styles.thumb, corner.pill)} />
    </SwitchPrimitive.Root>
  );
}
