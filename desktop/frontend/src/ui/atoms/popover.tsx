import * as stylex from "@stylexjs/stylex";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { PopoverPrimitive } from "@/ui/primitives";
import { FLOATING_LAYER, FLOATING_PANEL } from "./floating-surface";

type PositionerProps = ComponentProps<typeof PopoverPrimitive.Positioner>;
type PopupProps = ComponentProps<typeof PopoverPrimitive.Popup>;

interface PopoverContentBaseProps {
  children: ReactNode;
  className?: string;
  side?: PositionerProps["side"];
  align?: PositionerProps["align"];
  sideOffset?: PositionerProps["sideOffset"];
  alignOffset?: PositionerProps["alignOffset"];
}

type PopoverContentProps = PopoverContentBaseProps &
  Omit<PopupProps, keyof PopoverContentBaseProps | "className">;

function PopoverContent({
  children,
  className,
  side,
  align,
  sideOffset,
  alignOffset,
  ...popupProps
}: PopoverContentProps) {
  const panel = stylex.props(FLOATING_PANEL);
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Positioner
        side={side}
        align={align}
        sideOffset={sideOffset}
        alignOffset={alignOffset}
        {...stylex.props(FLOATING_LAYER)}
      >
        <PopoverPrimitive.Popup
          {...popupProps}
          {...panel}
          className={cn(panel.className, className)}
        >
          {children}
        </PopoverPrimitive.Popup>
      </PopoverPrimitive.Positioner>
    </PopoverPrimitive.Portal>
  );
}

export const Popover = {
  Root: PopoverPrimitive.Root,
  Trigger: PopoverPrimitive.Trigger,
  Content: PopoverContent,
} as const;
