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

const styles = stylex.create({
  // The panel is as wide as what it is anchored to, less the inset its own edges want.
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
  /** What to sit above. A ref, because the anchor mounts after the panel's first render. */
  anchor: PositionerProps["anchor"];
  children: ReactNode;
  className?: string;
  id?: string;
  role?: string;
  "aria-label"?: string;
}

/**
 * A panel the caller opens, over an element it owns no trigger for.
 *
 * The composer's suggestion lists are this shape: typing opens them, and focus must stay in the
 * textarea because what drives the selection is `aria-activedescendant` on the input, not focus
 * in the list. Hence `initialFocus={false}`.
 *
 * It portals because it has to. Rendered as a child of the composer surface — which clips to its
 * own corner with `overflow: hidden` — a panel placed above that surface paints nothing at all,
 * which is exactly what the file-mention popup did.
 */
function AnchoredPanel({
  open,
  onOpenChange,
  anchor,
  children,
  className,
  ...aria
}: AnchoredPanelProps) {
  const panel = stylex.props(FLOATING_PANEL, styles.matchAnchor);
  return (
    <PopoverPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <PopoverPrimitive.Portal>
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
    </PopoverPrimitive.Root>
  );
}

export const Popover = {
  Root: PopoverPrimitive.Root,
  Trigger: PopoverPrimitive.Trigger,
  Content: PopoverContent,
  Anchored: AnchoredPanel,
} as const;
