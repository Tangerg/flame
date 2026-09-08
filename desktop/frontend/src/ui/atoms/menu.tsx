import * as stylex from "@stylexjs/stylex";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { space, surface } from "@/styles/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons";
import { ContextMenuPrimitive, MenuPrimitive } from "@/ui/primitives";
import { FLOATING_LAYER, FLOATING_PANEL } from "./floating-surface";
import { floatingRow, floatingRowStyles, type RowLayout } from "./option-row";

const menuStyles = stylex.create({
  // A submenu positions itself against its trigger, so the trigger has to be a containing block.
  item: { position: "relative" },
  separator: {
    position: "relative",
    marginInline: space.s1,
    marginBlock: space.s1,
    height: "1px",
    backgroundColor: surface.divider,
  },
  // Both of these are what a menu IS, not what a call site decides. The width had been spelled
  // out at fourteen of them through a variable whose only job was to let them share a number —
  // and a number a call site has to name can still be named differently by the fifteenth. The
  // cap had never been offered at all, so four call sites each guessed one, and only one of the
  // four respected the viewport: `--available-height` is measured by the positioner from the
  // anchor to the screen edge, which is the thing `60vh` was estimating and `280px` ignored.
  content: {
    minWidth: "12rem",
    maxHeight: "min(380px, var(--available-height))",
    overflowY: "auto",
    overscrollBehavior: "contain",
    // So the keyboard's highlighted row does not arrive flush against the scroller's edge.
    scrollPaddingBlock: space.s1,
    padding: space.s1,
    outline: { default: null, ":focus-visible": "none" },
  },
  label: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
});

// The popup takes focus so the keyboard can drive it, which makes it match `:focus-visible`
// even when a mouse opened it — a ring around the whole menu, every time, on right-click. The
// highlighted ITEM is the indicator here, so the popup opts out the way the design system
// says a row state may: `data-chrome-focus`.
const MENU_CONTENT = [FLOATING_PANEL, menuStyles.content];

const menuItem = (layout: RowLayout = "grid") => [menuStyles.item, floatingRow(layout, "sm")];

type DropdownPositionerProps = ComponentProps<typeof MenuPrimitive.Positioner>;
type DropdownPopupProps = ComponentProps<typeof MenuPrimitive.Popup>;
type ContextPopupProps = ComponentProps<typeof ContextMenuPrimitive.Popup>;

interface FloatingContentProps {
  children: ReactNode;
  className?: string;
  side?: DropdownPositionerProps["side"];
  align?: DropdownPositionerProps["align"];
  sideOffset?: DropdownPositionerProps["sideOffset"];
  alignOffset?: DropdownPositionerProps["alignOffset"];
}

type DropdownContentProps = FloatingContentProps &
  Omit<DropdownPopupProps, keyof FloatingContentProps | "className">;

type ContextContentProps = FloatingContentProps &
  Omit<ContextPopupProps, keyof FloatingContentProps | "className">;

type DropdownItemProps = ComponentProps<typeof MenuPrimitive.Item>;
type DropdownSubmenuTriggerProps = ComponentProps<typeof MenuPrimitive.SubmenuTrigger>;
type ContextItemProps = ComponentProps<typeof ContextMenuPrimitive.Item>;
type ContextSubmenuTriggerProps = ComponentProps<typeof ContextMenuPrimitive.SubmenuTrigger>;

interface ContextIconItemProps extends Omit<ContextItemProps, "children" | "onClick" | "onSelect"> {
  icon: IconName;
  onSelect: () => void;
  destructive?: boolean;
  children: ReactNode;
}

function DropdownContent({
  children,
  className,
  side,
  align,
  sideOffset,
  alignOffset,
  ...popupProps
}: DropdownContentProps) {
  const content = stylex.props(MENU_CONTENT);
  return (
    <MenuPrimitive.Portal>
      <MenuPrimitive.Positioner
        side={side}
        align={align}
        sideOffset={sideOffset}
        alignOffset={alignOffset}
        {...stylex.props(FLOATING_LAYER)}
      >
        <MenuPrimitive.Popup
          {...popupProps}
          data-chrome-focus=""
          {...content}
          className={cn(content.className, className)}
        >
          {children}
        </MenuPrimitive.Popup>
      </MenuPrimitive.Positioner>
    </MenuPrimitive.Portal>
  );
}

function ContextContent({
  children,
  className,
  side,
  align,
  sideOffset,
  alignOffset,
  ...popupProps
}: ContextContentProps) {
  const content = stylex.props(MENU_CONTENT);
  return (
    <ContextMenuPrimitive.Portal>
      <ContextMenuPrimitive.Positioner
        side={side}
        align={align}
        sideOffset={sideOffset}
        alignOffset={alignOffset}
        {...stylex.props(FLOATING_LAYER)}
      >
        <ContextMenuPrimitive.Popup
          {...popupProps}
          data-chrome-focus=""
          {...content}
          className={cn(content.className, className)}
        >
          {children}
        </ContextMenuPrimitive.Popup>
      </ContextMenuPrimitive.Positioner>
    </ContextMenuPrimitive.Portal>
  );
}

function DropdownSeparator({
  className,
  ...props
}: ComponentProps<typeof MenuPrimitive.Separator>) {
  const separator = stylex.props(menuStyles.separator);
  return (
    <MenuPrimitive.Separator
      {...props}
      {...separator}
      className={cn(separator.className, className)}
    />
  );
}

function ContextSeparator({
  className,
  ...props
}: ComponentProps<typeof ContextMenuPrimitive.Separator>) {
  const separator = stylex.props(menuStyles.separator);
  return (
    <ContextMenuPrimitive.Separator
      {...props}
      {...separator}
      className={cn(separator.className, className)}
    />
  );
}

function DropdownItem({ layout, className, ...props }: DropdownItemProps & { layout?: RowLayout }) {
  const item = stylex.props(menuItem(layout));
  return <MenuPrimitive.Item {...props} {...item} className={cn(item.className, className)} />;
}

function DropdownSubmenuTrigger({
  layout,
  className,
  ...props
}: DropdownSubmenuTriggerProps & { layout?: RowLayout }) {
  const item = stylex.props(menuItem(layout));
  return (
    <MenuPrimitive.SubmenuTrigger {...props} {...item} className={cn(item.className, className)} />
  );
}

function ContextItem({ layout, className, ...props }: ContextItemProps & { layout?: RowLayout }) {
  const item = stylex.props(menuItem(layout));
  return (
    <ContextMenuPrimitive.Item {...props} {...item} className={cn(item.className, className)} />
  );
}

function ContextSubmenuTrigger({
  layout,
  className,
  ...props
}: ContextSubmenuTriggerProps & { layout?: RowLayout }) {
  const item = stylex.props(menuItem(layout));
  return (
    <ContextMenuPrimitive.SubmenuTrigger
      {...props}
      {...item}
      className={cn(item.className, className)}
    />
  );
}

function ContextIconItem({
  icon,
  onSelect,
  destructive,
  children,
  className,
  ...props
}: ContextIconItemProps) {
  const tone = stylex.props(destructive && floatingRowStyles.destructive);
  return (
    <ContextItem
      {...props}
      layout="glyph"
      onClick={onSelect}
      className={cn(tone.className, className)}
    >
      <Icon name={icon} size="xs" />
      <span {...stylex.props(menuStyles.label)}>{children}</span>
    </ContextItem>
  );
}

export const DropdownMenu = {
  Root: MenuPrimitive.Root,
  Trigger: MenuPrimitive.Trigger,
  Content: DropdownContent,
  Item: DropdownItem,
  Separator: DropdownSeparator,
  SubmenuRoot: MenuPrimitive.SubmenuRoot,
  SubmenuTrigger: DropdownSubmenuTrigger,
} as const;

export const ContextMenu = {
  Root: ContextMenuPrimitive.Root,
  Trigger: ContextMenuPrimitive.Trigger,
  Content: ContextContent,
  Item: ContextItem,
  IconItem: ContextIconItem,
  Separator: ContextSeparator,
  SubmenuRoot: ContextMenuPrimitive.SubmenuRoot,
  SubmenuTrigger: ContextSubmenuTrigger,
} as const;
