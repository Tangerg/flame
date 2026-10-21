import * as stylex from "@stylexjs/stylex";
import { type KeyboardEvent, type ReactNode } from "react";
import { corner, motion, radius, space, surface, type } from "@flame/ui/tokens.stylex";
import { CHOICE_SURFACE } from "@flame/ui";
import { Icon } from "@/ui/icons/index";
import { CheckboxGroupPrimitive, RadioGroupPrimitive, RadioPrimitive } from "@/ui/primitives/index";
import { CheckboxPrimitive } from "@flame/ui/primitives";
import { Pressable } from "@/ui/atoms/pressable";
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
    textAlign: "start",
    transitionProperty: "color, background-color, border-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
    cursor: {
      default: null,
      ':is(:disabled, [aria-disabled="true"], [data-disabled])': "not-allowed",
    },
    opacity: {
      default: null,
      ':is(:disabled, [aria-disabled="true"], [data-disabled])': "var(--control-disabled-opacity)",
    },
  },
  rowChosen: {
    backgroundColor: {
      default: surface.selected,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": surface.selectedHover,
    },
  },
  rowOpen: {
    backgroundColor: {
      default: null,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": surface.hover,
    },
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
          CHOICE_SURFACE.mark,
          type.uiXs,
          multiple ? CHOICE_SURFACE.square : corner.pill,
          selected && CHOICE_SURFACE.selected,
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
