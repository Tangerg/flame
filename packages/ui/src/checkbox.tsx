import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { clsx as cn } from "clsx";
import { color, motion, space, type } from "./tokens.stylex";
import { CHOICE_SURFACE } from "./control-surface";
import { Icon } from "./font-scale";
import { Check } from "@flame/icons/react";
import { Checkbox as CheckboxPrimitive } from "@base-ui/react/checkbox";

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
  target: {
    minWidth: "max(var(--control-height-xs), var(--control-target-min))",
    minHeight: "max(var(--control-height-xs), var(--control-target-min))",
    justifyContent: "center",
  },
  box: {
    transitionProperty: "color, background-color, border-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  mark: { color: color.onAccent },
});

type CheckboxProps = {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
} & (
  { label: ReactNode; ariaLabel?: never } | { label?: never; ariaLabel: string }
);

export function Checkbox({
  checked,
  onCheckedChange,
  label,
  ariaLabel,
  disabled,
  className,
}: CheckboxProps) {
  const row = stylex.props(
    styles.row,
    type.uiMd,
    ariaLabel !== undefined && styles.target,
    disabled && styles.off,
  );
  return (
    <label {...row} className={cn(row.className, className)}>
      <CheckboxPrimitive.Root
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        aria-label={ariaLabel}
        data-focus-outset={checked ? "" : undefined}
        {...stylex.props(
          CHOICE_SURFACE.mark,
          CHOICE_SURFACE.square,
          styles.box,
          checked && CHOICE_SURFACE.selected,
        )}
      >
        <CheckboxPrimitive.Indicator>
          <Icon glyph={Check} size="xs" {...stylex.props(styles.mark)} />
        </CheckboxPrimitive.Indicator>
      </CheckboxPrimitive.Root>
      {ariaLabel === undefined && <span>{label}</span>}
    </label>
  );
}
