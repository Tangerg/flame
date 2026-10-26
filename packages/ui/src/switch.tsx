import * as stylex from "@stylexjs/stylex";
import { clsx as cn } from "clsx";
import { color, corner, motion, space, surface } from "./tokens.stylex";
import { Switch as SwitchPrimitive } from "@base-ui/react/switch";

const styles = stylex.create({
  track: {
    position: "relative",
    display: "inline-flex",
    height: "var(--control-height-xs)",
    width: "calc(var(--control-height-xs) * 5 / 3)",
    padding: space.s0_5,
    flexShrink: 0,
    alignItems: "center",
    transitionProperty: "background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
    cursor: { default: null, ":disabled": "not-allowed" },
    opacity: { default: null, ":disabled": "var(--control-disabled-opacity)" },
  },
  on: { backgroundColor: color.accent },
  off: { backgroundColor: surface.controlEdge },
  thumb: {
    display: "block",
    height: "calc(var(--control-height-xs) - var(--spacing))",
    width: "calc(var(--control-height-xs) - var(--spacing))",
    backgroundColor: color.onAccent,
    boxShadow: "var(--shadow-control)",
    transitionProperty: "translate",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
    translate: {
      default: 0,
      ":is([data-checked])": "calc(var(--control-height-xs) * 2 / 3)",
      ":is([data-checked]):dir(rtl)": "calc(var(--control-height-xs) * -2 / 3)",
    },
  },
});

interface SwitchProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  ariaLabel: string;
  className?: string;
}

export function Switch({
  checked,
  onCheckedChange,
  disabled,
  ariaLabel,
  className,
}: SwitchProps) {
  const track = stylex.props(
    styles.track,
    corner.pill,
    checked ? styles.on : styles.off,
  );
  return (
    <SwitchPrimitive.Root
      checked={checked}
      onCheckedChange={onCheckedChange}
      disabled={disabled}
      aria-label={ariaLabel}
      data-focus-outset=""
      {...track}
      className={cn(track.className, className)}
    >
      <SwitchPrimitive.Thumb {...stylex.props(styles.thumb, corner.pill)} />
    </SwitchPrimitive.Root>
  );
}
