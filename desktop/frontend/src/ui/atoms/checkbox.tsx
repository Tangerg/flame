import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type } from "@/styles/tokens.stylex";
import { Icon } from "@/ui/icons";
import { CheckboxPrimitive } from "@/ui/primitives";

const styles = stylex.create({
  row: {
    display: "inline-flex",
    alignItems: "center",
    gap: space.s2,
    color: color.fgMuted,
    userSelect: "none",
    cursor: "default",
  },
  off: { cursor: "not-allowed", opacity: "var(--control-disabled-opacity)" },
  box: {
    display: "grid",
    height: "var(--control-mark-size)",
    width: "var(--control-mark-size)",
    flexShrink: 0,
    placeItems: "center",
    borderRadius: radius.step2xs,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: { default: surface.field, ":is([data-checked])": color.accent },
    backgroundColor: { default: surface.canvas, ":is([data-checked])": color.accent },
    transitionProperty: "color, background-color, border-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  mark: { color: color.onAccent },
});

interface CheckboxProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  label: ReactNode;
  disabled?: boolean;
  className?: string;
}

export function Checkbox({ checked, onCheckedChange, label, disabled, className }: CheckboxProps) {
  const row = stylex.props(styles.row, type.uiMd, disabled && styles.off);
  return (
    <label {...row} className={cn(row.className, className)}>
      <CheckboxPrimitive.Root
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        {...stylex.props(styles.box)}
      >
        <CheckboxPrimitive.Indicator>
          <Icon name="check" size="xs" {...stylex.props(styles.mark)} />
        </CheckboxPrimitive.Indicator>
      </CheckboxPrimitive.Root>
      <span>{label}</span>
    </label>
  );
}
