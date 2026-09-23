import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";
import type { ReactElement, ReactNode } from "react";
import { leading, space, type } from "@/styles/tokens.stylex";
import { TooltipPrimitive } from "@/ui/primitives";
import { FLOATING_LAYER, FLOATING_TIP } from "./floating-surface";

// The tip's own measure. `FLOATING_TIP` already paints the elevated floating surface; an
// inverted fill was a fourth material with no owner.
const tip = stylex.create({
  // zcode's tip: 12px copy on a 12px by 6px inset.
  measure: {
    paddingInline: space.s3,
    paddingBlock: space.s1_5,
    fontFamily: "var(--font-sans)",
    lineHeight: leading.snug,
  },
  label: { maxWidth: "280px" },
});

export interface TooltipProviderProps {
  children: ReactNode;
}

interface Props {
  label?: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  sideOffset?: number;
  delayDuration?: number;
  children: ReactNode;
}

interface RichTooltipProps {
  trigger: ReactElement;
  children: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  sideOffset?: number;
  delay?: number;
  /** Composed with the tip's own measure in one `stylex.props`, so a card can restate it. */
  styles?: StyleXStyles;
}

export function TooltipProvider({ children }: TooltipProviderProps) {
  return (
    <TooltipPrimitive.Provider delay={250} closeDelay={0} timeout={150}>
      {children}
    </TooltipPrimitive.Provider>
  );
}

export function Tooltip({ label, side = "top", sideOffset = 6, delayDuration, children }: Props) {
  if (label == null || label === "") return <>{children}</>;
  return (
    <RichTooltip
      trigger={children as ReactElement}
      side={side}
      sideOffset={sideOffset}
      delay={delayDuration}
      styles={tip.label}
    >
      {label}
    </RichTooltip>
  );
}

export function RichTooltip({
  trigger,
  children,
  side = "top",
  sideOffset = 6,
  delay,
  styles,
}: RichTooltipProps) {
  const popup = stylex.props(FLOATING_TIP, type.uiSm, tip.measure, styles);
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger render={trigger} delay={delay} />
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Positioner
          {...stylex.props(FLOATING_LAYER)}
          side={side}
          sideOffset={sideOffset}
        >
          <TooltipPrimitive.Popup role="tooltip" {...popup}>
            {children}
          </TooltipPrimitive.Popup>
        </TooltipPrimitive.Positioner>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
