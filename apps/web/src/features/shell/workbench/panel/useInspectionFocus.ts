import { useComposer } from "@/features/composer/public/composer";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { useLayoutEffect, useRef, useState, type KeyboardEvent } from "react";
import { handoffFocus, isConcealed, isImeKey } from "@flame/ui";
import { focusComposer as focusComposerTarget } from "@/features/composer/public/focus";
import {
  resolveInspectionReturnDock,
  returnFromWorkspaceInspection,
  selectWorkspaceChat,
  useWorkspaceSubagentRunId,
  WORKSPACE_SETTINGS_VIEW,
} from "@/features/workspace/public/navigation";
import { type WorkspaceInspection } from "@/features/workspace/public/navigation";
interface ReturnFocus {
  inspection: WorkspaceInspection;
  activeElement: Element | null;
  sessionId: string;
}

interface SettingsReturnFocus {
  origin: HTMLElement;
  activeElement: Element | null;
  sessionId: string;
}

interface ComposerRequest {
  readonly completion: symbol;
  readonly sessionId: string;
  readonly origin: Element | null;
  readonly inspection: WorkspaceInspection | null;
  readonly dockViewId: string | null;
  readonly focus: () => void;
}

// An inspection's source is presented again once its main view, its dock view,
// or the conversation is back, for the same subagent run when it came from one.
function returnSourcePresented(
  request: WorkspaceInspection,
  presented: {
    returnDockId: string | null | undefined;
    mainViewId: string | null;
    presentedViewId: string | null;
    subagentRunId: string | null | undefined;
  },
): boolean {
  const back = request.returnViewId
    ? presented.mainViewId === request.returnViewId
    : presented.returnDockId
      ? presented.presentedViewId === presented.returnDockId
      : presented.mainViewId === null;
  const fromSubagents =
    request.returnViewId === "subagents" || request.returnDockId === "subagents";
  return back && (!fromSubagents || presented.subagentRunId === request.returnSubagentRunId);
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
  const composerAccess = useComposer();
  const workspaceAccess = useWorkspace();

  const subagentRunId = useWorkspaceSubagentRunId();
  const delivered = useRef<WorkspaceInspection | null>(null);
  const returningRef = useRef<ReturnFocus | null>(null);
  const settingsReturningRef = useRef<SettingsReturnFocus | null>(null);
  const dockRef = useRef<HTMLElement>(null);
  const conversationRef = useRef<HTMLDivElement>(null);
  const mainViewRef = useRef<HTMLDivElement>(null);
  const presentationRef = fullPaneViewId ? mainViewRef : dockVisible ? dockRef : conversationRef;
  const previousPresentation = useRef({
    sessionId,
    fullPaneViewId: null as string | null,
    dockVisible,
  });
  const composerRequest = useRef<ComposerRequest | null>(null);
  const [composerRender, setComposerRender] = useState<symbol | null>(null);

  useLayoutEffect(
    () =>
      workspaceAccess.navigation.registerSettingsFocusCapture(() => {
        // The full-pane header sits outside the Settings body but inside the workspace.
        const workspace = conversationRef.current?.closest("main");
        if (!workspace) return;
        const doc = workspace.ownerDocument;
        const origin = doc.activeElement;
        if (!(origin instanceof HTMLElement) || !workspace.contains(origin)) return;
        return () => {
          const activeElement = doc.activeElement;
          if (activeElement === doc.body || workspace.contains(activeElement)) {
            settingsReturningRef.current = { origin, activeElement, sessionId };
          }
        };
      }),
    [workspaceAccess, sessionId],
  );

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
      request.focus();
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
    const returnDockId = resolveInspectionReturnDock(request, dockViewIds);
    const presentedViewId = fullPaneViewId ?? (dockVisible ? dockViewId : null);
    if (
      !returnSourcePresented(request, {
        returnDockId,
        mainViewId,
        presentedViewId,
        subagentRunId,
      })
    )
      return;
    const doc = request.origin.ownerDocument;
    if (doc.activeElement === returning.activeElement || doc.activeElement === doc.body) {
      if (request.origin.isConnected) {
        const restoreEditor = composerAccess.captureEditorFocusReturn(request.origin);
        if (!restoreEditor) request.origin.focus();
        else if (!isConcealed(request.origin)) restoreEditor();
      }
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
    composerAccess,
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
    previousPresentation.current = { sessionId, fullPaneViewId, dockVisible };
    const settingsReturning = settingsReturningRef.current;
    settingsReturningRef.current = null;
    if (
      (previous.sessionId === sessionId &&
        previous.fullPaneViewId === fullPaneViewId &&
        (dockVisible || !previous.dockVisible)) ||
      returningRef.current
    )
      return;
    const target = presentationRef.current;
    if (!target) return;
    const doc = target.ownerDocument;
    const active = doc.activeElement;
    if (
      settingsReturning?.sessionId === sessionId &&
      previous.fullPaneViewId === WORKSPACE_SETTINGS_VIEW &&
      fullPaneViewId !== WORKSPACE_SETTINGS_VIEW &&
      (active === doc.body ||
        (active &&
          active === settingsReturning.activeElement &&
          (isConcealed(active) || active.closest('[data-slot="workspace-window-header"]')))) &&
      !settingsReturning.origin.closest("[aria-disabled='true']")
    ) {
      const { origin } = settingsReturning;
      const restoreEditor = composerAccess.captureEditorFocusReturn(origin);
      if (
        restoreEditor
          ? origin.isConnected && !isConcealed(origin) && restoreEditor()
          : handoffFocus(doc.body, origin)
      )
        return;
    }
    // A presentation change hides owned content before WebKit settles focus:
    // entering a full pane hides the conversation and Dock, and leaving one for
    // a narrow conversation keeps the retained view inside the hidden Dock, and
    // collapsing the Dock hides it in place. Revealing a Dock hides nothing.
    const hiddenPanes =
      fullPaneViewId !== null
        ? [conversationRef.current, dockRef.current]
        : dockVisible
          ? []
          : [dockRef.current];
    const hiddenOrigin =
      !target.contains(active) && hiddenPanes.some((pane) => pane?.contains(active));
    if (
      active === doc.body ||
      (previous.fullPaneViewId === null && active === dockRef.current) ||
      hiddenOrigin
    ) {
      target.focus({ preventScroll: true });
    }
  }, [composerAccess, sessionId, fullPaneViewId, dockVisible, presentationRef]);

  const returnToSource = () => {
    if (!inspection) return;
    const activeElement = inspection.origin.ownerDocument.activeElement;
    returningRef.current = { inspection, activeElement, sessionId };
    const presentation = dockAvailable || inspection.previous ? "dock" : "main";
    if (!returnFromWorkspaceInspection(workspaceAccess, inspection, presentation))
      returningRef.current = null;
  };
  const requestComposerFocus = (focus = focusComposerTarget.bind(null, composerAccess)) => {
    const conversation = conversationRef.current;
    if (!conversation) return;
    const doc = conversation.ownerDocument;
    if (doc.activeElement?.closest('[role="dialog"], [role="alertdialog"]')) return;
    if (fullPaneViewId === null) {
      composerRequest.current = null;
      focus();
      return;
    }
    const hideDock = !dockAvailable && inspection !== null && inspection.viewId === dockViewId;
    const request: ComposerRequest = {
      completion: Symbol(),
      sessionId,
      origin: doc.activeElement,
      inspection,
      dockViewId: hideDock ? null : dockViewId,
      focus,
    };
    composerRequest.current = request;
    selectWorkspaceChat(workspaceAccess, hideDock ? "hide-dock" : "preserve-dock");
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
    presentationRef,
    returnToSource,
    onMaterialKeyDown,
    requestComposerFocus,
  };
}
