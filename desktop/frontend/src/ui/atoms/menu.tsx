import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { space, surface } from "@/styles/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons";
import { ContextMenuPrimitive, MenuPrimitive } from "@/ui/primitives";
import { FLOATING_LAYER, FLOATING_OPTIONS } from "./floating-surface";
import { floatingRow, floatingRowStyles, type RowLayout } from "./option-row";

const menuStyles = stylex.create({
  item: { position: "relative" },
  separator: {
    position: "relative",
    marginInline: space.s1,
    marginBlock: space.s1,
    height: "1px",
    backgroundColor: surface.divider,
  },
  content: {
    display: "grid",
    alignContent: "start",
    rowGap: "2px",
    minWidth: "12rem",
    maxHeight: "min(380px, var(--available-height))",
    overflow: "hidden auto",
    overscrollBehavior: "contain",
    scrollPaddingBlock: space.s1,
    padding: space.s1,
  },
  label: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
});

const MENU_CONTENT = [FLOATING_OPTIONS, menuStyles.content];

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

type RowRefinement = { layout?: RowLayout; styles?: StyleXStyles };

function DropdownItem({
  layout,
  styles,
  destructive,
  className,
  ...props
}: DropdownItemProps & RowRefinement & { destructive?: boolean }) {
  const item = stylex.props(menuItem(layout), destructive && floatingRowStyles.destructive, styles);
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

function ContextItem({
  layout,
  styles,
  destructive,
  className,
  ...props
}: ContextItemProps & RowRefinement & { destructive?: boolean }) {
  const item = stylex.props(menuItem(layout), destructive && floatingRowStyles.destructive, styles);
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
  return (
    <ContextItem
      {...props}
      layout="glyph"
      destructive={destructive}
      onClick={onSelect}
      className={className}
    >
      <Icon name={icon} size="md" />
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

function ContextTrigger({
  onKeyDown,
  ...props
}: ComponentProps<typeof ContextMenuPrimitive.Trigger>) {
  return (
    <ContextMenuPrimitive.Trigger
      {...props}
      onKeyDown={(event) => {
        onKeyDown?.(event);
        if (event.defaultPrevented || event.key !== "F10" || !event.shiftKey) return;
        event.preventDefault();
        const target = event.target as HTMLElement;
        const box = target.getBoundingClientRect();
        target.dispatchEvent(
          new MouseEvent("contextmenu", {
            bubbles: true,
            cancelable: true,
            clientX: box.left,
            clientY: box.bottom,
          }),
        );
      }}
    />
  );
}

export const ContextMenu = {
  Root: ContextMenuPrimitive.Root,
  Trigger: ContextTrigger,
  Content: ContextContent,
  Item: ContextItem,
  IconItem: ContextIconItem,
  Separator: ContextSeparator,
  SubmenuRoot: ContextMenuPrimitive.SubmenuRoot,
  SubmenuTrigger: ContextSubmenuTrigger,
} as const;
