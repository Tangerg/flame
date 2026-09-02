import type { ReactElement, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { TooltipPrimitive } from "@/ui/primitives";
import { FLOATING_LAYER, FLOATING_TIP } from "./floating-surface";

// The tip's own measure. Three call sites used to pass this, in three paddings, alongside an
// inverted fill — so a tooltip's material was decided wherever one happened to be raised.
// `FLOATING_TIP` already paints the elevated floating surface, which is what Codex gives a
// tooltip (`--bg-tooltip: var(--color-surface-elevated)`); an inverted one was a fourth
// material with no owner.
const TIP_PADDING = "px-2 py-1.5 font-sans text-ui-md leading-snug";

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
  className?: string;
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
      className="max-w-[280px]"
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
  className,
}: RichTooltipProps) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger render={trigger} delay={delay} />
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Positioner className={FLOATING_LAYER} side={side} sideOffset={sideOffset}>
          <TooltipPrimitive.Popup
            role="tooltip"
            className={cn(FLOATING_TIP, TIP_PADDING, className)}
          >
            {children}
          </TooltipPrimitive.Popup>
        </TooltipPrimitive.Positioner>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
