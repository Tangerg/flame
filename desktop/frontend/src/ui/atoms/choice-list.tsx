import type { KeyboardEvent, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon } from "@/ui/icons";
import {
  CheckboxGroupPrimitive,
  CheckboxPrimitive,
  RadioGroupPrimitive,
  RadioPrimitive,
} from "@/ui/primitives";
import { Pressable } from "./pressable";

interface ChoiceListProps {
  multiple: boolean;
  value: string[];
  values: readonly string[];
  labelledBy: string;
  disabled?: boolean;
  onValueChange: (value: string[]) => void;
  children: ReactNode;
}

export function ChoiceList({
  multiple,
  value,
  values,
  labelledBy,
  disabled,
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
    className: "flex flex-col gap-1",
    disabled,
    onKeyDown: selectNumberedChoice,
  };

  return multiple ? (
    <CheckboxGroupPrimitive {...shared} value={value} onValueChange={onValueChange}>
      {children}
    </CheckboxGroupPrimitive>
  ) : (
    <RadioGroupPrimitive
      {...shared}
      value={value[0]}
      onValueChange={(selected) => onValueChange([selected])}
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
  onReselect,
  children,
}: ChoiceOptionProps) {
  const className = ({ checked }: { checked: boolean }) =>
    cn(
      "group/choice flex min-h-8 w-full items-center gap-2 rounded-full px-2 py-1.5 text-left outline-none transition-colors duration-[var(--dur-fast)] disabled:cursor-not-allowed disabled:opacity-64",
      checked ? "bg-hover" : "hover:bg-hover",
    );

  const common = {
    value,
    disabled,
    nativeButton: true,
    "aria-label": label,
    "aria-description": description || undefined,
    onClick: () => {
      if (!disabled && !multiple && selected) onReselect?.();
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
        className={cn(
          "grid size-5 shrink-0 place-items-center border border-field bg-surface-2 text-ui-xs leading-none font-medium text-fg-muted group-data-[checked]/choice:border-fg group-data-[checked]/choice:bg-fg group-data-[checked]/choice:text-canvas",
          multiple ? "rounded-2xs" : "rounded-full",
        )}
      >
        {multiple ? (
          <CheckboxPrimitive.Indicator>
            <Icon name="check" size="xs" />
          </CheckboxPrimitive.Indicator>
        ) : (
          <>
            <span className="group-data-[checked]/choice:hidden">{ordinal}</span>
            <RadioPrimitive.Indicator>
              <span className="block size-1.5 rounded-full bg-current" />
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
