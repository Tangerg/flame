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
  box: {
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

export function Checkbox({
  checked,
  onCheckedChange,
  label,
  disabled,
  className,
}: CheckboxProps) {
  const row = stylex.props(styles.row, type.uiMd, disabled && styles.off);
  return (
    <label {...row} className={cn(row.className, className)}>
      <CheckboxPrimitive.Root
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
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
      <span>{label}</span>
    </label>
  );
}
