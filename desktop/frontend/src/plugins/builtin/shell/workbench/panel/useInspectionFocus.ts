import { useLayoutEffect, useRef, type KeyboardEvent } from "react";
import {
  returnFromWorkspaceInspection,
  useWorkspaceInspection,
  type WorkspaceInspection,
} from "@/plugins/builtin/workspace/public/navigation";

interface ReturnFocus {
  inspection: WorkspaceInspection;
  activeElement: Element | null;
  sessionId: string;
}

export function useInspectionFocus({
  sessionId,
  mainViewId,
  dockViewId,
  dockViewIds,
  dockVisible,
}: {
  sessionId: string;
  mainViewId: string | null;
  dockViewId: string | null;
  dockViewIds: readonly string[];
  dockVisible: boolean;
}) {
  const inspection = useWorkspaceInspection();
  const delivered = useRef<WorkspaceInspection | null>(null);
  const returningRef = useRef<ReturnFocus | null>(null);
  const dockRef = useRef<HTMLElement>(null);
  const conversationRef = useRef<HTMLDivElement>(null);
  const mainViewRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (!inspection) {
      delivered.current = null;
      return;
    }
    const dock = dockRef.current;
    if (
      delivered.current === inspection ||
      !dock ||
      !dockVisible ||
      mainViewId !== null ||
      dockViewId !== inspection.viewId
    )
      return;
    delivered.current = inspection;
    const doc = dock.ownerDocument;
    const active = doc.activeElement;
    if (active === inspection.activeElement || active === doc.body) {
      dock.focus({ preventScroll: true });
    }
  }, [inspection, dockVisible, mainViewId, dockViewId]);

  useLayoutEffect(() => {
    const returning = returningRef.current;
    if (!returning) return;
    const request = returning.inspection;
    if (returning.sessionId !== sessionId || (inspection && inspection !== request)) {
      returningRef.current = null;
      return;
    }
    if (request.returnViewId ? mainViewId !== request.returnViewId : mainViewId !== null) return;
    if (
      !request.returnViewId &&
      request.returnDockId &&
      dockViewIds.includes(request.returnDockId) &&
      (dockViewId !== request.returnDockId || !dockVisible)
    )
      return;
    const doc = request.origin.ownerDocument;
    if (doc.activeElement === returning.activeElement || doc.activeElement === doc.body) {
      if (request.origin.isConnected) request.origin.focus();
      if (doc.activeElement !== request.origin) {
        (request.returnViewId ? mainViewRef : conversationRef).current?.focus({
          preventScroll: true,
        });
      }
    }
    returningRef.current = null;
  }, [inspection, sessionId, mainViewId, dockViewId, dockViewIds, dockVisible]);

  const returnToSource = () => {
    if (!inspection) return;
    const activeElement = inspection.origin.ownerDocument.activeElement;
    returningRef.current = { inspection, activeElement, sessionId };
    if (!returnFromWorkspaceInspection(inspection)) returningRef.current = null;
  };
  const onDockKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (
      !inspection ||
      event.key !== "Escape" ||
      event.defaultPrevented ||
      event.nativeEvent.isComposing ||
      !event.currentTarget.contains(event.target as Node)
    )
      return;
    event.preventDefault();
    event.stopPropagation();
    returnToSource();
  };

  return { inspection, dockRef, conversationRef, mainViewRef, returnToSource, onDockKeyDown };
}
