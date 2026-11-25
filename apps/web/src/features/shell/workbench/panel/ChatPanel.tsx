import { Group, Panel } from "react-resizable-panels";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { useComposer } from "@/features/composer/public/composer";
import {
  Fragment,
  useCallback,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type Ref,
} from "react";
import { flushSync } from "react-dom";
import { type AgentInput } from "@/features/agent/public/input";
import {
  AgentContentCard,
  AgentContextDock,
  AgentDockRow,
  AgentDockSeparator,
  AgentDockToggle,
} from "@/ui/agent/index";
import { useActiveSessionId, useAgentSessions } from "@/features/agent/public/session";
import { registerComposerFocusPresentation } from "@/features/composer/public/focus";
import {
  openWorkspaceView,
  useActiveWorkspaceViewId,
  useWorkspaceDock,
  useWorkspaceInspection,
  WORKSPACE_DOCK_CATALOG,
} from "@/features/workspace/public/navigation";
import { useContextDockCatalog } from "@/features/workspace/public/contextDockCatalog";
import { useWorkspaceViews } from "@/features/workspace/public/views";
import { useDockLayout } from "@/features/workspace/public/sidebarDrawer";
import { useT, labelText } from "@/lib/i18n/index";
import { canPresentDock } from "@/features/shell/workbench/panel/shellGeometry";
import { useInspectionFocus } from "@/features/shell/workbench/panel/useInspectionFocus";
import {
  CATALOG_VIEW,
  applyComposerReadingLimit,
  mountedWorkspaceViews,
  returnActionLabel,
  toggleChatDock,
  workspacePresentation,
} from "@/features/shell/workbench/panel/workspacePresentation";
import {
  ConversationPane,
  FullPaneHeader,
} from "@/features/shell/workbench/panel/ConversationPane";
import { DockHeader, WorkspaceViewPicker } from "@/features/shell/workbench/panel/DockHeader";
import { WorkspaceViewStack } from "@/features/shell/workbench/panel/WorkspaceViewStack";

export interface ChatPanelHandle {
  focusComposer(): void;
}

interface Props {
  onSend: (input: AgentInput) => boolean;
  ref?: Ref<ChatPanelHandle>;
}

function SessionOwnedWorkspaceState({
  sessionId,
  children,
}: {
  sessionId: string;
  children: ReactNode;
}) {
  return <Fragment key={sessionId}>{children}</Fragment>;
}

const PANEL_SIZES = {
  main: { conversation: { minSize: 0, maxSize: 0 }, dock: { minSize: "100%" } },
  dock: { conversation: { minSize: 440 }, dock: { minSize: 320 } },
  closed: { conversation: { minSize: 440 }, dock: { minSize: 0, maxSize: 0 } },
} as const;

// Capacity commits can resize body portals and start ancestor observers, so the row
// publishes its width from animation frames before ResizeObserver delivery.
function useDockRowWidth() {
  const [rowWidth, setRowWidth] = useState<number | null>(null);
  const dockRowRef = useCallback((row: HTMLDivElement | null) => {
    if (!row) return;
    const view = row.ownerDocument.defaultView!;
    let width = row.clientWidth;
    let capacity = canPresentDock(width);
    let frame = 0;
    let retired = false;
    setRowWidth(width);
    const sample = () => {
      if (retired) return;
      frame = 0;
      const next = row.clientWidth;
      if (next !== width) {
        width = next;
        const nextCapacity = canPresentDock(width);
        if (nextCapacity !== capacity) {
          capacity = nextCapacity;
          flushSync(() => setRowWidth(width));
        } else {
          setRowWidth(width);
        }
      }
      if (!retired) frame = view.requestAnimationFrame(sample);
    };
    frame = view.requestAnimationFrame(sample);
    return () => {
      retired = true;
      view.cancelAnimationFrame(frame);
    };
  }, []);
  return [rowWidth, dockRowRef] as const;
}

function useDockTabs(
  viewIds: readonly string[],
  viewsById: ReadonlyMap<string, ReturnType<typeof useWorkspaceViews>[number]>,
  activeViewId: string | null,
) {
  const t = useT();
  const workspaceAccess = useWorkspace();
  const navigation = workspaceAccess.navigation;
  return viewIds.map((id) => {
    const view = viewsById.get(id);
    const title = view ? labelText(view.title, t) : id;
    const Badge = view?.badge;
    return {
      id,
      title,
      badge: Badge ? <Badge /> : undefined,
      active: id === activeViewId,
      onClose: () => navigation.closeDockView(id),
      closeLabel: `${t("common.close")} ${title}`,
      onCloseOthers: () => navigation.closeOtherDockViews(id),
      closeOthersLabel: t("dock.tabs.closeOthers"),
      onCloseAll: navigation.closeAllDockViews.bind(navigation),
      closeAllLabel: t("dock.tabs.closeAll"),
    };
  });
}

export function ChatPanel({ onSend, ref }: Props) {
  const pickerRef = useRef<HTMLButtonElement>(null);
  const composerAccess = useComposer();
  const workspaceAccess = useWorkspace();

  const activeMainView = useActiveWorkspaceViewId();
  const dock = useWorkspaceDock();
  const catalog = useContextDockCatalog();
  const views = useWorkspaceViews();
  const { layout: dockLayout, save: saveDockLayout } = useDockLayout();
  const [initialDockLayout] = useState(() => dockLayout ?? undefined);
  const { data: sessions } = useAgentSessions();
  const activeSessionId = useActiveSessionId();
  const t = useT();
  const inspection = useWorkspaceInspection();
  const [rowWidth, dockRowRef] = useDockRowWidth();
  const dockAvailable = canPresentDock(rowWidth ?? 0);

  const {
    hasDockOwner,
    dockOpen,
    dockVisible,
    ownedDockViewIds,
    fullPaneViewId,
    inspectedFullPane,
  } = workspacePresentation({ activeSessionId, dock, activeMainView, inspection, rowWidth });
  const conversationReady = sessions !== undefined;
  const viewsReady = conversationReady || activeMainView !== null || dock.open;
  const {
    dockRef,
    conversationRef,
    mainViewRef,
    presentationRef,
    returnToSource,
    onMaterialKeyDown,
    requestComposerFocus,
  } = useInspectionFocus({
    sessionId: activeSessionId,
    inspection,
    mainViewId: activeMainView,
    fullPaneViewId,
    dockViewId: dock.activeViewId,
    dockViewIds: ownedDockViewIds,
    dockAvailable,
    dockVisible,
  });
  useLayoutEffect(
    () => registerComposerFocusPresentation(composerAccess, requestComposerFocus),
    [requestComposerFocus, composerAccess],
  );
  useImperativeHandle(ref, () => ({ focusComposer: requestComposerFocus }));

  const viewsById = new Map(views.map((view) => [view.id, view]));
  const returnAction = inspection
    ? { label: returnActionLabel(t, inspection, viewsById), onReturn: returnToSource }
    : undefined;

  const dockTabs = useDockTabs(ownedDockViewIds, viewsById, dock.activeViewId);
  const openViewIds = new Set(ownedDockViewIds);
  if (activeMainView !== null) openViewIds.add(activeMainView);
  const presentedMain = fullPaneViewId !== null;
  const panelSizes = PANEL_SIZES[presentedMain ? "main" : dockVisible ? "dock" : "closed"];
  const mountedViewIds = mountedWorkspaceViews(openViewIds, inspection);
  const presentedViewId = presentedMain ? fullPaneViewId : dock.activeViewId;

  return (
    <AgentContentCard
      label={t("shell.region.workspace")}
      onFocusCapture={applyComposerReadingLimit}
    >
      {presentedMain && (
        <FullPaneHeader
          viewId={fullPaneViewId}
          view={
            fullPaneViewId === WORKSPACE_DOCK_CATALOG ? CATALOG_VIEW : viewsById.get(fullPaneViewId)
          }
          returnAction={inspectedFullPane ? returnAction : undefined}
          dockAvailable={dockAvailable}
          onKeyDown={inspectedFullPane ? onMaterialKeyDown : undefined}
          picker={
            hasDockOwner &&
            fullPaneViewId !== WORKSPACE_DOCK_CATALOG &&
            viewsById.get(fullPaneViewId)?.dock !== undefined && (
              <WorkspaceViewPicker
                triggerRef={pickerRef}
                key={activeSessionId}
                groups={catalog}
                openViewIds={openViewIds}
                activeViewId={fullPaneViewId}
                onSelect={openWorkspaceView.bind(null, workspaceAccess)}
              />
            )
          }
        />
      )}
      <AgentDockRow
        ref={dockRowRef}
        open={dockVisible}
        presentation={presentedMain ? "main" : "dock"}
      >
        <Group
          orientation="horizontal"
          defaultLayout={initialDockLayout}
          onLayoutChanged={(layout, meta) => {
            if (meta.isUserInteraction) saveDockLayout(meta.requestedLayout ?? layout);
          }}
          style={{ flex: 1, width: "100%", minHeight: 0 }}
        >
          <Panel id="conversation" {...panelSizes.conversation}>
            <ConversationPane
              ref={conversationRef}
              hidden={presentedMain}
              fallbackFocus={presentationRef}
              ready={conversationReady}
              sessionId={activeSessionId}
              onSend={onSend}
            />
          </Panel>
          {!presentedMain && dockVisible && <AgentDockSeparator label={t("dock.action.resize")} />}
          <Panel id="dock" defaultSize={480} {...panelSizes.dock}>
            {viewsReady && (
              <SessionOwnedWorkspaceState sessionId={activeSessionId}>
                <AgentContextDock
                  value={presentedViewId === WORKSPACE_DOCK_CATALOG ? null : presentedViewId}
                  onValueChange={(id) => {
                    if (id !== null) workspaceAccess.navigation.selectDockView(id);
                  }}
                  ref={dockRef}
                  role={presentedMain ? "presentation" : undefined}
                  tabIndex={presentedMain ? undefined : -1}
                  aria-label={
                    presentedMain
                      ? undefined
                      : (() => {
                          const active = viewsById.get(dock.activeViewId ?? "");
                          return active ? labelText(active.title, t) : t("dock.tabs.label");
                        })()
                  }
                  data-focus-inset=""
                  onKeyDown={
                    activeMainView === null || inspectedFullPane ? onMaterialKeyDown : undefined
                  }
                >
                  {!presentedMain && dockVisible && (
                    <DockHeader
                      key={activeSessionId}
                      tabs={dockTabs}
                      groups={catalog}
                      openViewIds={openViewIds}
                      returnAction={returnAction}
                    />
                  )}
                  <WorkspaceViewStack
                    presentation={presentedMain ? "main" : "dock"}
                    shown={presentedMain || dockVisible}
                    presentedViewId={presentedViewId}
                    fullPaneViewId={fullPaneViewId}
                    addressable={activeMainView === null}
                    mountedViewIds={mountedViewIds}
                    mainViewRef={mainViewRef}
                    presentationRef={presentationRef}
                    catalog={catalog}
                    openViewIds={openViewIds}
                  />
                </AgentContextDock>
              </SessionOwnedWorkspaceState>
            )}
          </Panel>
        </Group>
        {viewsReady && !presentedMain && hasDockOwner && (
          <AgentDockToggle
            open={dockVisible}
            onToggle={() => toggleChatDock(workspaceAccess, dockAvailable, dockOpen)}
            showLabel={dockAvailable ? t("dock.action.show") : t("dock.action.openFull")}
            hideLabel={t("dock.action.hide")}
          />
        )}
      </AgentDockRow>
    </AgentContentCard>
  );
}
