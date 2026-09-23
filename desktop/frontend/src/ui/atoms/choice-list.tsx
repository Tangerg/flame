import * as stylex from "@stylexjs/stylex";
import type { KeyboardEvent, ReactNode } from "react";
import {
  color,
  corner,
  motion,
  radius,
  space,
  surface,
  type,
  weight,
} from "@/styles/tokens.stylex";
import { Icon } from "@/ui/icons";
import {
  CheckboxGroupPrimitive,
  CheckboxPrimitive,
  RadioGroupPrimitive,
  RadioPrimitive,
} from "@/ui/primitives";
import { Pressable } from "./pressable";

const styles = stylex.create({
  list: { display: "flex", flexDirection: "column", gap: space.s1 },
  row: {
    display: "flex",
    minHeight: space.s8,
    width: "100%",
    alignItems: "center",
    gap: space.s2,
    borderRadius: radius.row,
    paddingInline: space.s2,
    paddingBlock: space.s1_5,
    textAlign: "left",
    transitionProperty: "color, background-color, border-color",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
    cursor: { default: null, ':is(:disabled, [aria-disabled="true"])': "not-allowed" },
    opacity: {
      default: null,
      ':is(:disabled, [aria-disabled="true"])': "var(--control-disabled-opacity)",
    },
  },
  rowChosen: { backgroundColor: surface.hover },
  rowOpen: { backgroundColor: { default: null, ":hover": surface.hover } },

  mark: {
    display: "grid",
    height: "var(--control-mark-size)",
    width: "var(--control-mark-size)",
    flexShrink: 0,
    placeItems: "center",
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.controlEdge,
    backgroundColor: surface.canvas,
    lineHeight: 1,
    fontWeight: weight.medium,
    color: color.fgMuted,
  },
  markSquare: { borderRadius: radius.step2xs },
  markChosen: {
    borderColor: color.accent,
    backgroundColor: color.accent,
    color: color.onAccent,
  },
  dot: { display: "block", height: space.s1_5, width: space.s1_5, backgroundColor: "currentColor" },
});

interface ChoiceListProps {
  multiple: boolean;
  value: string[];
  values: readonly string[];
  labelledBy: string;
  disabled?: boolean;
  numbered?: boolean;
  pending?: boolean;
  onValueChange: (value: string[]) => void;
  children: ReactNode;
}

export function ChoiceList({
  multiple,
  value,
  values,
  labelledBy,
  disabled,
  numbered,
  pending,
  onValueChange,
  children,
}: ChoiceListProps) {
  const selectNumberedChoice = (event: KeyboardEvent<HTMLDivElement>) => {
    if (multiple || !numbered || !/^[1-9]$/.test(event.key)) return;
    const selected = values[Number(event.key) - 1];
    if (selected === undefined) return;
    event.preventDefault();
    onValueChange([selected]);
  };

  const shared = {
    "aria-labelledby": labelledBy,
    className: stylex.props(styles.list).className,
    disabled,
    "aria-disabled": pending ? true : undefined,
    onKeyDown: pending ? undefined : selectNumberedChoice,
  };

  return multiple ? (
    <CheckboxGroupPrimitive
      {...shared}
      value={value}
      onValueChange={pending ? undefined : onValueChange}
    >
      {children}
    </CheckboxGroupPrimitive>
  ) : (
    <RadioGroupPrimitive
      {...shared}
      value={value[0]}
      onValueChange={pending ? undefined : (selected) => onValueChange([selected])}
    >
      {children}
    </RadioGroupPrimitive>
  );
}

interface ChoiceOptionProps {
  multiple: boolean;
  value: string;
  selected: boolean;
  ordinal?: number;
  label: string;
  description?: string;
  disabled?: boolean;
  pending?: boolean;
  busy?: boolean;
  onReselect?: () => void;
  children: ReactNode;
}

export function ChoiceOption({
  multiple,
  value,
  selected,
  ordinal,
  label,
  description,
  disabled,
  pending,
  busy,
  onReselect,
  children,
}: ChoiceOptionProps) {
  const className = ({ checked }: { checked: boolean }) =>
    stylex.props(styles.row, checked ? styles.rowChosen : styles.rowOpen).className ?? "";

  const common = {
    value,
    disabled,
    "aria-disabled": pending ? true : undefined,
    "aria-busy": busy ? true : undefined,
    nativeButton: true,
    "aria-label": label,
    "aria-description": description || undefined,
    onClick: () => {
      if (!disabled && !pending && !multiple && selected) onReselect?.();
    },
    render: <Pressable />,
  };

  const content = (
    <>
      <span
        aria-hidden
        {...stylex.props(
          styles.mark,
          type.uiXs,
          multiple ? styles.markSquare : corner.pill,
          selected && styles.markChosen,
        )}
      >
        {multiple ? (
          <CheckboxPrimitive.Indicator>
            <Icon name="check" size="xs" />
          </CheckboxPrimitive.Indicator>
        ) : (
          <>
            {!selected && ordinal !== undefined && <span>{ordinal}</span>}
            <RadioPrimitive.Indicator>
              <span {...stylex.props(styles.dot, corner.pill)} />
            </RadioPrimitive.Indicator>
          </>
        )}
      </span>
      {children}
    </>
  );

  return multiple ? (
    <CheckboxPrimitive.Root {...common} className={className}>
      {content}
    </CheckboxPrimitive.Root>
  ) : (
    <RadioPrimitive.Root {...common} className={className}>
      {content}
    </RadioPrimitive.Root>
  );
}
