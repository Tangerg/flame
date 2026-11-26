import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";
import {
  useLayoutEffect,
  useState,
  type ReactElement,
  type ReactNode,
  type RefObject,
} from "react";
import { font, leading, space, type } from "./tokens.stylex";
import { Tooltip as TooltipPrimitive } from "@base-ui/react/tooltip";
import { FLOATING_LAYER, FLOATING_TIP } from "./floating-surface";
import {
  useHiddenPopupRelease,
  useHiddenRootRetirement,
} from "./popup-visibility";

const tip = stylex.create({
  layer: { pointerEvents: "none" },
  measure: {
    paddingInline: space.s3,
    paddingBlock: space.s1_5,
    fontFamily: font.sans,
    lineHeight: leading.snug,
    pointerEvents: {
      default: "auto",
      ':is([data-instant="focus"])': "none",
    },
  },
  label: { maxWidth: "280px", overflowWrap: "anywhere" },
});

export interface TooltipProviderProps {
  children: ReactNode;
}

export type TooltipActions = TooltipPrimitive.Root.Actions;

interface TooltipOpenBehavior {
  canOpen?: (trigger: Element) => boolean;
  actionsRef?: RefObject<TooltipActions | null>;
}

interface Props extends TooltipOpenBehavior {
  label?: ReactNode;
  disabled?: boolean;
  side?: "top" | "right" | "bottom" | "left";
  sideOffset?: number;
  delayDuration?: number;
  children: ReactElement;
}

interface RichTooltipProps extends TooltipOpenBehavior {
  trigger: ReactElement;
  children: ReactNode;
  disabled?: boolean;
  side?: "top" | "right" | "bottom" | "left";
  sideOffset?: number;
  delay?: number;
  styles?: StyleXStyles;
}

export function useClampedText(
  ref: RefObject<HTMLElement | null>,
  text: string,
  tag?: string,
): boolean {
  const [clamped, setClamped] = useState(false);
  useLayoutEffect(() => {
    const node = ref.current;
    if (!node) return;
    const { documentElement, fonts } = node.ownerDocument;
    const measure = () => {
      const { clientHeight, clientWidth, scrollHeight, scrollWidth } = node;
      setClamped(
        (clientHeight > 0 && scrollHeight > clientHeight) ||
          (clientWidth > 0 && scrollWidth > clientWidth),
      );
    };
    const resize = new ResizeObserver(measure);
    resize.observe(node);
    // A font switch can change text overflow without resizing the box.
    const appearance = new MutationObserver(measure);
    appearance.observe(documentElement, { attributeFilter: ["style"] });
    fonts?.addEventListener("loadingdone", measure);
    measure();
    return () => {
      resize.disconnect();
      appearance.disconnect();
      fonts?.removeEventListener("loadingdone", measure);
    };
  }, [ref, text, tag]);
  return clamped;
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
  canOpen,
  actionsRef,
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
      canOpen={canOpen}
      actionsRef={actionsRef}
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
  canOpen,
  actionsRef,
  styles,
}: RichTooltipProps) {
  const retirement = useHiddenRootRetirement<
    TooltipActions,
    TooltipPrimitive.Root.ChangeEventDetails
  >({
    actionsRef,
    onOpenChange: (open, details) => {
      if (open && canOpen && (!details.trigger || !canOpen(details.trigger)))
        details.cancel();
    },
  });
  const { portalRef } = useHiddenPopupRelease();
  const popup = stylex.props(FLOATING_TIP, type.uiSm, tip.measure, styles);
  return (
    <TooltipPrimitive.Root disabled={disabled} {...retirement}>
      <TooltipPrimitive.Trigger render={trigger} delay={delay} />
      <TooltipPrimitive.Portal ref={portalRef}>
        <TooltipPrimitive.Positioner
          {...stylex.props(FLOATING_LAYER, tip.layer)}
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
