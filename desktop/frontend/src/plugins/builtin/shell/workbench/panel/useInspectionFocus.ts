import { useLayoutEffect, useRef, type KeyboardEvent } from "react";
import { isImeKey } from "@flame/ui";
import {
  returnFromWorkspaceInspection,
  useWorkspaceInspection,
  useWorkspaceSubagentRunId,
  WORKSPACE_DOCK_CATALOG,
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
  const subagentRunId = useWorkspaceSubagentRunId();
  const delivered = useRef<WorkspaceInspection | null>(null);
  const returningRef = useRef<ReturnFocus | null>(null);
  const dockRef = useRef<HTMLElement>(null);
  const conversationRef = useRef<HTMLDivElement>(null);
  const mainViewRef = useRef<HTMLDivElement>(null);
  const previousPresentation = useRef({ sessionId, mainViewId: null as string | null });

  useLayoutEffect(() => {
    if (!inspection) {
      delivered.current = null;
      return;
    }
    if (returningRef.current?.inspection.previous === inspection) {
      delivered.current = inspection;
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
    if (returning.sessionId !== sessionId || inspection !== request.previous) {
      returningRef.current = null;
      return;
    }
    if (request.returnViewId ? mainViewId !== request.returnViewId : mainViewId !== null) return;
    if (
      (request.returnViewId === "subagents" || request.returnDockId === "subagents") &&
      subagentRunId !== request.returnSubagentRunId
    )
      return;
    if (
      !request.returnViewId &&
      request.returnDockId &&
      (request.returnDockId === WORKSPACE_DOCK_CATALOG ||
        dockViewIds.includes(request.returnDockId)) &&
      (dockViewId !== request.returnDockId || !dockVisible)
    )
      return;
    const doc = request.origin.ownerDocument;
    if (doc.activeElement === returning.activeElement || doc.activeElement === doc.body) {
      if (request.origin.isConnected) request.origin.focus();
      if (doc.activeElement !== request.origin) {
        (request.returnViewId
          ? mainViewRef
          : request.returnDockId
            ? dockRef
            : conversationRef
        ).current?.focus({
          preventScroll: true,
        });
      }
    }
    returningRef.current = null;
  }, [inspection, sessionId, mainViewId, dockViewId, dockViewIds, dockVisible, subagentRunId]);

  useLayoutEffect(() => {
    const previous = previousPresentation.current;
    previousPresentation.current = { sessionId, mainViewId };
    if (
      (previous.sessionId === sessionId && previous.mainViewId === mainViewId) ||
      returningRef.current
    )
      return;
    const target = (mainViewId ? mainViewRef : dockVisible ? dockRef : conversationRef).current;
    if (target && target.ownerDocument.activeElement === target.ownerDocument.body) {
      target.focus({ preventScroll: true });
    }
  }, [sessionId, mainViewId, dockVisible]);

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
      isImeKey(event.nativeEvent) ||
      !event.currentTarget.contains(event.target as Node)
    )
      return;
    event.preventDefault();
    event.stopPropagation();
    returnToSource();
  };

  return { inspection, dockRef, conversationRef, mainViewRef, returnToSource, onDockKeyDown };
}
