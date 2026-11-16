import { useEffectEvent, useLayoutEffect, useRef, type RefObject } from "react";
import { useContentVisible } from "./content-visibility";
interface RetirableRootProps<Actions, Details> {
  open?: boolean | undefined;
  defaultOpen?: boolean | undefined;
  onOpenChange?: ((open: boolean, eventDetails: Details) => void) | undefined;
  actionsRef?: RefObject<Actions | null> | undefined;
}

export function useHiddenRootRetirement<
  Actions extends { close: () => void },
  Details extends { isCanceled: boolean },
>({
  open,
  defaultOpen = false,
  onOpenChange,
  actionsRef,
}: RetirableRootProps<Actions, Details>) {
  const visible = useContentVisible();
  const ownActions = useRef<Actions | null>(null);
  const actions = actionsRef ?? ownActions;
  // Accepted closes precede controlled prop commits, including a commit that unmounts the root.
  const acceptedOpen = useRef(open ?? defaultOpen);
  useLayoutEffect(() => {
    if (open !== undefined) acceptedOpen.current = open;
  }, [open]);
  const retire = useEffectEvent(() => {
    if (acceptedOpen.current) actions.current?.close();
  });
  useLayoutEffect(() => {
    if (!visible) retire();
  }, [visible, open, actions]);
  useLayoutEffect(() => () => retire(), []);
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
