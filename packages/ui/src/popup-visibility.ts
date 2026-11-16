import { useEffectEvent, useLayoutEffect, useRef, type RefObject } from "react";
import { useContentVisible } from "./content-visibility";
interface RetirableRootProps<Actions, Details> {
  open?: boolean | undefined;
  defaultOpen?: boolean | undefined;
  onOpenChange?: ((open: boolean, eventDetails: Details) => void) | undefined;
  actionsRef?: RefObject<Actions | null> | undefined;
}

interface PopupConnection {
  phase: "active" | "retiring" | "retired";
}

export function useHiddenRootRetirement<
  Actions extends { close: () => void },
  Details extends { isCanceled: boolean; cancel: () => void },
>({
  open,
  defaultOpen = false,
  onOpenChange,
  actionsRef,
}: RetirableRootProps<Actions, Details>) {
  const visible = useContentVisible();
  const ownActions = useRef<Actions | null>(null);
  const actions = actionsRef ?? ownActions;
  const connection = useRef<PopupConnection>({ phase: "active" });
  const openIntent = useRef({ open: open ?? defaultOpen });
  useLayoutEffect(() => {
    if (open !== undefined) openIntent.current = { open };
  }, [open]);
  const retire = useEffectEvent((scope: PopupConnection) => {
    scope.phase = "retiring";
    try {
      if (openIntent.current.open) actions.current?.close();
    } finally {
      scope.phase = "retired";
    }
  });
  useLayoutEffect(() => {
    if (!visible) return;
    const scope: PopupConnection = { phase: "active" };
    connection.current = scope;
    return () => retire(scope);
  }, [visible]);
  // Controlled props can introduce another open intent while the owner remains hidden.
  const retireHidden = useEffectEvent(() => {
    if (!visible) retire(connection.current);
  });
  useLayoutEffect(() => retireHidden(), [open, actions]);
  return {
    actionsRef: actions,
    onOpenChange: (next: boolean, eventDetails: Details) => {
      const scope = connection.current;
      if (
        scope.phase === "retired" ||
        (next && (!visible || scope.phase !== "active"))
      ) {
        eventDetails.cancel();
        return;
      }
      // The callback can synchronously unmount the root or commit a newer intent.
      const previous = openIntent.current;
      const change = { open: next ? previous.open : false };
      openIntent.current = change;
      onOpenChange?.(next, eventDetails);
      // A reconnect creates a new connection; this opening still belongs to the retired one.
      if (next && scope.phase !== "active") eventDetails.cancel();
      if (openIntent.current === change) {
        openIntent.current = eventDetails.isCanceled
          ? previous
          : { open: next };
      }
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
