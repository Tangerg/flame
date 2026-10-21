import { useLayoutEffect, useRef, type RefObject } from "react";
import { useContentVisible } from "@/ui/content-visibility";
interface RetirableRootProps<Actions, Details> {
  open?: boolean | undefined;
  defaultOpen?: boolean | undefined;
  onOpenChange?: ((open: boolean, eventDetails: Details) => void) | undefined;
  actionsRef?: RefObject<Actions | null> | undefined;
}

export function useHiddenRootRetirement<
  Actions extends { close: () => void },
  Details extends { isCanceled: boolean },
>({ open, defaultOpen = false, onOpenChange, actionsRef }: RetirableRootProps<Actions, Details>) {
  const visible = useContentVisible();
  const ownActions = useRef<Actions | null>(null);
  const actions = actionsRef ?? ownActions;
  // Base UI owns uncontrolled state; this snapshot avoids redundant close callbacks.
  const acceptedOpen = useRef(defaultOpen);
  useLayoutEffect(() => {
    if (!visible && (open ?? acceptedOpen.current)) actions.current?.close();
  }, [visible, open, actions]);
  return {
    actionsRef: actions,
    onOpenChange: (next: boolean, eventDetails: Details) => {
      onOpenChange?.(next, eventDetails);
      if (!eventDetails.isCanceled) acceptedOpen.current = next;
    },
  };
}

export function useHiddenPopupRelease() {
  const visible = useContentVisible();
  const portalRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const portal = portalRef.current;
    if (visible || !portal) return;
    const active = portal.ownerDocument.activeElement;
    if (active instanceof HTMLElement && portal.contains(active)) active.blur();
  }, [visible]);
  return { visible, portalRef };
}
