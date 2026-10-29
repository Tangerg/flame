import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";
import type { ReactElement, ReactNode } from "react";
import { leading, space, type } from "./tokens.stylex";
import { Tooltip as TooltipPrimitive } from "@base-ui/react/tooltip";
import { FLOATING_LAYER, FLOATING_TIP } from "./floating-surface";

const tip = stylex.create({
  measure: {
    paddingInline: space.s3,
    paddingBlock: space.s1_5,
    fontFamily: "var(--font-sans)",
    lineHeight: leading.snug,
  },
  label: { maxWidth: "280px", overflowWrap: "anywhere" },
});

export interface TooltipProviderProps {
  children: ReactNode;
}

interface Props {
  label?: ReactNode;
  disabled?: boolean;
  side?: "top" | "right" | "bottom" | "left";
  sideOffset?: number;
  delayDuration?: number;
  children: ReactElement;
}

interface RichTooltipProps {
  trigger: ReactElement;
  children: ReactNode;
  disabled?: boolean;
  side?: "top" | "right" | "bottom" | "left";
  sideOffset?: number;
  delay?: number;
  styles?: StyleXStyles;
}

export function TooltipProvider({ children }: TooltipProviderProps) {
  return (
    <TooltipPrimitive.Provider delay={250} closeDelay={0} timeout={150}>
      {children}
    </TooltipPrimitive.Provider>
  );
}

export function Tooltip({
  label,
  side,
  sideOffset,
  delayDuration,
  disabled,
  children,
}: Props) {
  if (label == null || label === "") return <>{children}</>;
  return (
    <RichTooltip
      trigger={children}
      side={side}
      sideOffset={sideOffset}
      delay={delayDuration}
      disabled={disabled}
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
  disabled,
  styles,
}: RichTooltipProps) {
  const popup = stylex.props(FLOATING_TIP, type.uiSm, tip.measure, styles);
  return (
    <TooltipPrimitive.Root disabled={disabled}>
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
