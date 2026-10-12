import * as stylex from "@stylexjs/stylex";
import { ChevronDown, Check } from "@flame/icons/react";
import { SelectPrimitive } from "./primitives/index";
import { Button } from "./button";
import { Icon } from "./font-scale";
import { FLOATING_LAYER, FLOATING_OPTIONS } from "./floating-surface";
import { color, radius, space, surface, type } from "./tokens.stylex";

const styles = stylex.create({
  popup: {
    paddingInline: space.s1,
    paddingBlock: space.s1,
    overflow: "auto",
    maxHeight: "var(--available-height)",
    minWidth: "var(--anchor-width)",
    maxWidth: "min(var(--available-width), calc(100dvw - var(--spacing) * 8))",
    overscrollBehavior: "contain",
  },
  item: {
    display: "flex",
    alignItems: "center",
    gap: space.s2,
    minHeight: "var(--control-height-md)",
    overflowWrap: "anywhere",
    paddingInline: space.s2,
    paddingBlock: space.s1_5,
    borderRadius: radius.row,
    color: color.fg,
    cursor: "default",
    backgroundColor: {
      default: "transparent",
      ":is([data-highlighted])": surface.hover,
    },
  },
  trigger: {
    maxWidth: "100%",
    height: "auto",
    minHeight: "24px",
    whiteSpace: "normal",
    textAlign: "start",
    overflowWrap: "anywhere",
  },
});

export function Select({
  value,
  onValueChange,
  options,
  label,
  id,
  disabled,
}: {
  value: string;
  onValueChange: (value: string) => void;
  options: readonly { value: string; label: string }[];
  label: string;
  id?: string;
  disabled?: boolean;
}) {
  return (
    <SelectPrimitive.Root
      items={options}
      value={value}
      disabled={disabled}
      onValueChange={(next) => {
        if (next !== null) onValueChange(next);
      }}
    >
      <SelectPrimitive.Trigger
        render={<Button variant="outline" size="sm" styles={styles.trigger} />}
        id={id}
        aria-label={label}
      >
        <SelectPrimitive.Value />
        <SelectPrimitive.Icon>
          <Icon glyph={ChevronDown} size="xs" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Positioner
          {...stylex.props(FLOATING_LAYER)}
          sideOffset={4}
        >
          <SelectPrimitive.Popup
            {...stylex.props(FLOATING_OPTIONS, styles.popup, type.uiSm)}
          >
            {options.map((option) => (
              <SelectPrimitive.Item
                key={option.value}
                value={option.value}
                {...stylex.props(styles.item)}
              >
                <SelectPrimitive.ItemText>
                  {option.label}
                </SelectPrimitive.ItemText>
                <SelectPrimitive.ItemIndicator>
                  <Icon glyph={Check} size="xs" />
                </SelectPrimitive.ItemIndicator>
              </SelectPrimitive.Item>
            ))}
          </SelectPrimitive.Popup>
        </SelectPrimitive.Positioner>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
}
