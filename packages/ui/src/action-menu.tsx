import * as stylex from "@stylexjs/stylex";
import { useRef, type ComponentProps, type ReactNode } from "react";
import { Ellipsis } from "@flame/icons/react";
import { MENU_SURFACE } from "./control-surface";
import { Button } from "./button";
import { Icon } from "./font-scale";
import { FLOATING_LAYER, FLOATING_PANEL } from "./floating-surface";
import { Menu as MenuPrimitive } from "@base-ui/react/menu";
import {
  useHiddenPopupRelease,
  useHiddenRootRetirement,
} from "./popup-visibility";
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
  onSelect(opener: HTMLButtonElement): void;
  tone?: "negative";
  disabled?: boolean;
}

export function ActionMenu({
  label,
  items,
  disabled,
  triggerContent,
  finalFocus,
}: {
  label: string;
  items: readonly ActionMenuItem[];
  disabled?: boolean;
  triggerContent?: ReactNode;
  finalFocus?: ComponentProps<typeof MenuPrimitive.Popup>["finalFocus"];
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const retirement = useHiddenRootRetirement<
    MenuPrimitive.Root.Actions,
    MenuPrimitive.Root.ChangeEventDetails
  >({});
  const { visible, portalRef } = useHiddenPopupRelease();
  return (
    <MenuPrimitive.Root {...retirement}>
      <MenuPrimitive.Trigger
        ref={trigger}
        disabled={disabled}
        aria-label={label}
        render={
          <Button
            size={triggerContent === undefined ? "icon-md" : "md"}
            variant="ghost"
          />
        }
      >
        {triggerContent === undefined ? (
          <Icon glyph={Ellipsis} size="md" />
        ) : (
          triggerContent
        )}
      </MenuPrimitive.Trigger>
      <MenuPrimitive.Portal ref={portalRef}>
        <MenuPrimitive.Positioner
          align="end"
          sideOffset={4}
          {...stylex.props(FLOATING_LAYER)}
        >
          <MenuPrimitive.Popup
            aria-label={label}
            finalFocus={visible ? finalFocus : false}
            {...stylex.props(
              FLOATING_PANEL,
              MENU_SURFACE.content,
              styles.popup,
            )}
          >
            {items.map((item) => (
              <MenuPrimitive.Item
                key={item.label}
                disabled={disabled || item.disabled}
                onClick={() => item.onSelect(trigger.current!)}
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
