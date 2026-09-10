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
    paddingInline: space.s2,
    paddingBlock: space.s1_5,
    textAlign: "left",
    transitionProperty: "color, background-color, border-color",
    transitionDuration: motion.fast,
    cursor: { default: null, ':is(:disabled, [aria-disabled="true"])': "not-allowed" },
    opacity: {
      default: null,
      ':is(:disabled, [aria-disabled="true"])': "var(--control-disabled-opacity)",
    },
  },
  // A chosen row keeps the wash whether or not the pointer is on it; an open one only borrows it.
  rowChosen: { backgroundColor: surface.hover },
  rowOpen: { backgroundColor: { default: null, ":hover": surface.hover } },

  // Round for one-of, square for many-of — the distinction every platform makes, and the one
  // this list needs most before anything is selected: a multi-select's unchecked mark carries
  // no number and no check, so a circle there is three blank radios telling the reader to pick
  // one. `2xs` is the corner `Checkbox` already uses, so the two places the app asks for
  // several answers now ask the same way.
  mark: {
    display: "grid",
    height: space.s5,
    width: space.s5,
    flexShrink: 0,
    placeItems: "center",
    borderWidth: "1px",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: surface.surface2,
    lineHeight: 1,
    fontWeight: weight.medium,
    color: color.fgMuted,
  },
  markSquare: { borderRadius: radius.step2xs },
  markChosen: { borderColor: color.fg, backgroundColor: color.fg, color: surface.canvas },
  dot: { display: "block", height: space.s1_5, width: space.s1_5, backgroundColor: "currentColor" },
});

interface ChoiceListProps {
  multiple: boolean;
  value: string[];
  values: readonly string[];
  labelledBy: string;
  disabled?: boolean;
  /** The answer this list belongs to is being submitted. See `TextField`'s `pending`. */
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
  pending,
  onValueChange,
  children,
}: ChoiceListProps) {
  const selectNumberedChoice = (event: KeyboardEvent<HTMLDivElement>) => {
    if (multiple || !/^[1-9]$/.test(event.key)) return;
    const selected = values[Number(event.key) - 1];
    if (selected === undefined) return;
    event.preventDefault();
    onValueChange([selected]);
  };

  const shared = {
    "aria-labelledby": labelledBy,
    className: stylex.props(styles.list).className,
    disabled,
    // In flight it stays focusable and announces itself as disabled, and the CHANGE is refused
    // here — `disabled` would take the group out of the tab order, so answering a question by
    // keyboard blurred the person answering it.
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
  ordinal: number;
  label: string;
  description?: string;
  disabled?: boolean;
  /** The answer this option belongs to is being submitted. */
  pending?: boolean;
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
  onReselect,
  children,
}: ChoiceOptionProps) {
  // The row's own state, from the state Base UI hands the class function — not from an
  // ancestor selector. `selected` says the same thing on the React side, and the mark below
  // reads it: what the row is showing is known here, so nothing has to be inherited for it.
  const className = ({ checked }: { checked: boolean }) =>
    stylex.props(styles.row, corner.pill, checked ? styles.rowChosen : styles.rowOpen).className ??
    "";

  const common = {
    value,
    disabled,
    "aria-disabled": pending ? true : undefined,
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
      {/* Round for one-of, square for many-of — the distinction every platform makes, and the
          one this list needs most before anything is selected: a multi-select's unchecked mark
          carries no number and no check, so a circle there is three blank radios telling the
          reader to pick one. `rounded-2xs` is the radius `Checkbox` already uses, so the two
          places the app asks for several answers now ask the same way. */}
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
            {/* The ordinal is the shortcut, so it stands in for the dot until one is chosen. */}
            {!selected && <span>{ordinal}</span>}
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
