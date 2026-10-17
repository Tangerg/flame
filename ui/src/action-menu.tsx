import * as stylex from "@stylexjs/stylex";
import { Ellipsis } from "@flame/icons/react";
import { MENU_SURFACE } from "./control-surface";
import { Button } from "./button";
import { Icon } from "./font-scale";
import { FLOATING_LAYER, FLOATING_OPTIONS } from "./floating-surface";
import { MenuPrimitive } from "./primitives/menu";
import { color, type } from "./tokens.stylex";

const styles = stylex.create({
  popup: {
    minWidth: "12rem",
    maxWidth: "min(var(--available-width), calc(100dvw - var(--spacing) * 8))",
    maxHeight: "var(--available-height)",
    overflowY: "auto",
  },
  item: { display: "flex", alignItems: "center" },
  negative: { color: color.negative },
});

export interface ActionMenuItem {
  label: string;
  onSelect(): void;
  tone?: "negative";
  disabled?: boolean;
}

export function ActionMenu({
  label,
  items,
  disabled,
}: {
  label: string;
  items: readonly ActionMenuItem[];
  disabled?: boolean;
}) {
  return (
    <MenuPrimitive.Root>
      <MenuPrimitive.Trigger
        disabled={disabled}
        aria-label={label}
        render={<Button size="icon-md" variant="ghost" />}
      >
        <Icon glyph={Ellipsis} size="md" />
      </MenuPrimitive.Trigger>
      <MenuPrimitive.Portal>
        <MenuPrimitive.Positioner
          align="end"
          sideOffset={4}
          {...stylex.props(FLOATING_LAYER)}
        >
          <MenuPrimitive.Popup
            aria-label={label}
            {...stylex.props(
              FLOATING_OPTIONS,
              MENU_SURFACE.content,
              styles.popup,
            )}
          >
            {items.map((item) => (
              <MenuPrimitive.Item
                key={item.label}
                disabled={disabled || item.disabled}
                onClick={item.onSelect}
                {...stylex.props(
                  MENU_SURFACE.item,
                  styles.item,
                  type.uiMd,
                  item.tone === "negative" && styles.negative,
                )}
              >
                {item.label}
              </MenuPrimitive.Item>
            ))}
          </MenuPrimitive.Popup>
        </MenuPrimitive.Positioner>
      </MenuPrimitive.Portal>
    </MenuPrimitive.Root>
  );
}
