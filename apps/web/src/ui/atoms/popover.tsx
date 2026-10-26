import * as stylex from "@stylexjs/stylex";
import { type ComponentProps, type ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Popover as PopoverPrimitive } from "@base-ui/react/popover";
import { FLOATING_LAYER, FLOATING_PANEL } from "@flame/ui/floating-surface";
import { useHiddenPopupRelease, useHiddenRootRetirement } from "@flame/ui";
type PositionerProps = ComponentProps<typeof PopoverPrimitive.Positioner>;

type PopupProps = ComponentProps<typeof PopoverPrimitive.Popup>;

interface PopoverContentBaseProps {
  children: ReactNode;
  className?: string;
  anchor?: PositionerProps["anchor"];
  positionMethod?: PositionerProps["positionMethod"];
  collisionPadding?: PositionerProps["collisionPadding"];
  side?: PositionerProps["side"];
  align?: PositionerProps["align"];
  sideOffset?: PositionerProps["sideOffset"];
  alignOffset?: PositionerProps["alignOffset"];
  surface?: "panel" | "none";
}

type PopoverContentProps = PopoverContentBaseProps &
  Omit<PopupProps, keyof PopoverContentBaseProps | "className">;

function PopoverContent({
  children,
  className,
  anchor,
  positionMethod,
  collisionPadding,
  side,
  align,
  sideOffset,
  alignOffset,
  surface = "panel",
  finalFocus,
  ...popupProps
}: PopoverContentProps) {
  const { visible, portalRef } = useHiddenPopupRelease();
  const panel = stylex.props(surface === "panel" && FLOATING_PANEL);
  return (
    <PopoverPrimitive.Portal ref={portalRef}>
      <PopoverPrimitive.Positioner
        anchor={anchor}
        positionMethod={positionMethod}
        collisionPadding={collisionPadding}
        side={side}
        align={align}
        sideOffset={sideOffset}
        alignOffset={alignOffset}
        {...stylex.props(FLOATING_LAYER)}
      >
        <PopoverPrimitive.Popup
          {...popupProps}
          finalFocus={visible ? finalFocus : false}
          {...panel}
          className={cn(panel.className, className)}
        >
          {children}
        </PopoverPrimitive.Popup>
      </PopoverPrimitive.Positioner>
    </PopoverPrimitive.Portal>
  );
}

const styles = stylex.create({
  matchAnchor: {
    width: "calc(var(--anchor-width) - calc(var(--spacing) * 4))",
    maxHeight: "min(320px, var(--available-height))",
    overflowY: "auto",
    overscrollBehavior: "contain",
  },
});

interface AnchoredPanelProps {
  open: boolean;
  onOpenChange?: (open: boolean) => void;
  anchor: PositionerProps["anchor"];
  children: ReactNode;
  className?: string;
  id?: string;
  role?: string;
  "aria-label"?: string;
}

function AnchoredPanel({
  open,
  onOpenChange,
  anchor,
  children,
  className,
  ...aria
}: AnchoredPanelProps) {
  const { portalRef } = useHiddenPopupRelease();
  const panel = stylex.props(FLOATING_PANEL, styles.matchAnchor);
  return (
    <PopoverRoot open={open} onOpenChange={onOpenChange}>
      <PopoverPrimitive.Portal ref={portalRef}>
        <PopoverPrimitive.Positioner
          anchor={anchor}
          side="top"
          align="center"
          sideOffset={8}
          {...stylex.props(FLOATING_LAYER)}
        >
          <PopoverPrimitive.Popup
            {...aria}
            initialFocus={false}
            finalFocus={false}
            {...panel}
            className={cn(panel.className, className)}
          >
            {children}
          </PopoverPrimitive.Popup>
        </PopoverPrimitive.Positioner>
      </PopoverPrimitive.Portal>
    </PopoverRoot>
  );
}

function PopoverRoot<Payload = unknown>(props: PopoverPrimitive.Root.Props<Payload>) {
  const retirement = useHiddenRootRetirement(props);
  return <PopoverPrimitive.Root {...props} {...retirement} />;
}

export const Popover = {
  Root: PopoverRoot,
  Trigger: PopoverPrimitive.Trigger,
  Content: PopoverContent,
  Anchored: AnchoredPanel,
} as const;
