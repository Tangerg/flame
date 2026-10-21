import { useLayoutEffect, useRef, useState, type KeyboardEvent } from "react";
import { isImeKey } from "@flame/ui";
import { focusComposer as focusComposerTarget } from "@/features/composer/public/focus";
import {
  returnFromWorkspaceInspection,
  selectWorkspaceChat,
  useWorkspaceSubagentRunId,
  WORKSPACE_DOCK_CATALOG,
} from "@/features/workspace/public/navigation";
import { type WorkspaceInspection } from "@/features/workspace/public/navigation";
interface ReturnFocus {
  inspection: WorkspaceInspection;
  activeElement: Element | null;
  sessionId: string;
}

interface ComposerRequest {
  readonly completion: symbol;
  readonly sessionId: string;
  readonly origin: Element | null;
  readonly inspection: WorkspaceInspection | null;
  readonly dockViewId: string | null;
}

export function useInspectionFocus({
  sessionId,
  inspection,
  mainViewId,
  fullPaneViewId,
  dockViewId,
  dockViewIds,
  dockAvailable,
  dockVisible,
}: {
  sessionId: string;
  inspection: WorkspaceInspection | null;
  mainViewId: string | null;
  fullPaneViewId: string | null;
  dockViewId: string | null;
  dockViewIds: readonly string[];
  dockAvailable: boolean;
  dockVisible: boolean;
}) {
  const subagentRunId = useWorkspaceSubagentRunId();
  const delivered = useRef<WorkspaceInspection | null>(null);
  const returningRef = useRef<ReturnFocus | null>(null);
  const dockRef = useRef<HTMLElement>(null);
  const conversationRef = useRef<HTMLDivElement>(null);
  const mainViewRef = useRef<HTMLDivElement>(null);
  const previousPresentation = useRef({ sessionId, fullPaneViewId: null as string | null });
  const composerRequest = useRef<ComposerRequest | null>(null);
  const [composerRender, setComposerRender] = useState<symbol | null>(null);

  // Deliver before generic presentation focus. The token also retires requests
  // when a later navigation coalesces back to the original hidden view.
  useLayoutEffect(() => {
    const request = composerRequest.current;
    if (!request) return;
    const revealed = fullPaneViewId === null;
    if (!revealed && composerRender !== request.completion) return;
    composerRequest.current = null;
    const conversation = conversationRef.current;
    if (
      !revealed ||
      !conversation ||
      request.sessionId !== sessionId ||
      request.inspection !== inspection ||
      request.dockViewId !== dockViewId
    )
      return;
    const doc = conversation.ownerDocument;
    if (doc.activeElement === request.origin || doc.activeElement === doc.body) {
      focusComposerTarget();
    }
  }, [composerRender, sessionId, inspection, fullPaneViewId, dockViewId]);

  useLayoutEffect(() => {
    if (!inspection) {
      delivered.current = null;
      return;
    }
    if (returningRef.current?.inspection.previous === inspection) {
      delivered.current = inspection;
      return;
    }
    const material = fullPaneViewId !== null ? mainViewRef.current : dockVisible && dockRef.current;
    if (
      delivered.current === inspection ||
      !material ||
      mainViewId !== null ||
      dockViewId !== inspection.viewId
    )
      return;
    delivered.current = inspection;
    const doc = material.ownerDocument;
    const active = doc.activeElement;
    if (active === inspection.activeElement || active === doc.body) {
      material.focus({ preventScroll: true });
    }
  }, [inspection, dockVisible, mainViewId, fullPaneViewId, dockViewId]);

  useLayoutEffect(() => {
    const returning = returningRef.current;
    if (!returning) return;
    const request = returning.inspection;
    if (returning.sessionId !== sessionId || inspection !== request.previous) {
      returningRef.current = null;
      return;
    }
    const returnDockId =
      !request.returnViewId &&
      request.returnDockId &&
      (request.returnDockId === WORKSPACE_DOCK_CATALOG ||
        dockViewIds.includes(request.returnDockId))
        ? request.returnDockId
        : null;
    const presentedViewId = fullPaneViewId ?? (dockVisible ? dockViewId : null);
    if (
      request.returnViewId
        ? mainViewId !== request.returnViewId
        : returnDockId
          ? presentedViewId !== returnDockId
          : mainViewId !== null
    )
      return;
    if (
      (request.returnViewId === "subagents" || request.returnDockId === "subagents") &&
      subagentRunId !== request.returnSubagentRunId
    )
      return;
    const doc = request.origin.ownerDocument;
    if (doc.activeElement === returning.activeElement || doc.activeElement === doc.body) {
      if (request.origin.isConnected) request.origin.focus();
      if (doc.activeElement !== request.origin) {
        (fullPaneViewId !== null
          ? mainViewRef
          : returnDockId
            ? dockRef
            : conversationRef
        ).current?.focus({
          preventScroll: true,
        });
      }
    }
    returningRef.current = null;
  }, [
    inspection,
    sessionId,
    mainViewId,
    fullPaneViewId,
    dockViewId,
    dockViewIds,
    dockVisible,
    subagentRunId,
  ]);

  useLayoutEffect(() => {
    const previous = previousPresentation.current;
    previousPresentation.current = { sessionId, fullPaneViewId };
    if (
      (previous.sessionId === sessionId && previous.fullPaneViewId === fullPaneViewId) ||
      returningRef.current
    )
      return;
    const target = (fullPaneViewId ? mainViewRef : dockVisible ? dockRef : conversationRef).current;
    if (!target) return;
    const doc = target.ownerDocument;
    const active = doc.activeElement;
    // Full-pane presentation hides other owned content before WebKit settles focus.
    const hiddenOrigin =
      fullPaneViewId !== null &&
      !target.contains(active) &&
      [conversationRef.current, dockRef.current].some((pane) => pane?.contains(active));
    if (
      active === doc.body ||
      (previous.fullPaneViewId === null && active === dockRef.current) ||
      hiddenOrigin
    ) {
      target.focus({ preventScroll: true });
    }
  }, [sessionId, fullPaneViewId, dockVisible]);

  const returnToSource = () => {
    if (!inspection) return;
    const activeElement = inspection.origin.ownerDocument.activeElement;
    returningRef.current = { inspection, activeElement, sessionId };
    const presentation = dockAvailable || inspection.previous ? "dock" : "main";
    if (!returnFromWorkspaceInspection(inspection, presentation)) returningRef.current = null;
  };
  const requestComposerFocus = () => {
    const conversation = conversationRef.current;
    if (!conversation) return;
    const doc = conversation.ownerDocument;
    if (doc.activeElement?.closest('[role="dialog"], [role="alertdialog"]')) return;
    if (fullPaneViewId === null) {
      composerRequest.current = null;
      focusComposerTarget();
      return;
    }
    const hideDock = !dockAvailable && inspection !== null && inspection.viewId === dockViewId;
    const request: ComposerRequest = {
      completion: Symbol(),
      sessionId,
      origin: doc.activeElement,
      inspection,
      dockViewId: hideDock ? null : dockViewId,
    };
    composerRequest.current = request;
    selectWorkspaceChat(hideDock ? "hide-dock" : "preserve-dock");
    setComposerRender(request.completion);
  };
  const onMaterialKeyDown = (event: KeyboardEvent<HTMLElement>) => {
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

  return {
    dockRef,
    conversationRef,
    mainViewRef,
    returnToSource,
    onMaterialKeyDown,
    requestComposerFocus,
  };
}
