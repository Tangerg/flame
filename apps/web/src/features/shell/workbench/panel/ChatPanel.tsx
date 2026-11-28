import * as stylex from "@stylexjs/stylex";
import { Group, Panel } from "react-resizable-panels";
import { PanelHost } from "@flame/ui/panel";
import { surface } from "@flame/ui/tokens.stylex";
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
import { ContentVisibility } from "@/ui/index";
import {
  AgentContentCard,
  AgentContextDock,
  AgentDockRow,
  AgentDockSeparator,
  AgentDockToggle,
} from "@/ui/agent/index";
import { useActiveSessionId, useAgentSessionSummary } from "@/features/agent/public/session";
import { useRuntimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { registerComposerFocusPresentation } from "@/features/composer/public/focus";
import {
  openWorkspaceView,
  useActiveWorkspaceViewId,
  useWorkspaceDock,
  useWorkspaceInspection,
  WORKSPACE_DOCK_CATALOG,
  WORKSPACE_SETTINGS_VIEW,
} from "@/features/workspace/public/navigation";
import { useContextDockCatalog } from "@/features/workspace/public/contextDockCatalog";
import { useWorkspaceViews } from "@/features/workspace/public/views";
import { useDockLayout } from "@/features/workspace/public/sidebarDrawer";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";
import { useT, labelText, type Label, type Translate } from "@/lib/i18n/index";
import { canPresentDock } from "@/features/shell/workbench/panel/shellGeometry";
import { useInspectionFocus } from "@/features/shell/workbench/panel/useInspectionFocus";
import {
  CATALOG_VIEW,
  applyComposerReadingLimit,
  fullPanePickable,
  returnAction,
  toggleChatDock,
  workspacePresentation,
} from "@/features/shell/workbench/panel/workspacePresentation";
import {
  ConversationPane,
  FullPaneHeader,
  paneStyles,
} from "@/features/shell/workbench/panel/ConversationPane";
import { DockHeader, WorkspaceViewPicker } from "@/features/shell/workbench/panel/DockHeader";
import { WorkspaceViewStack } from "@/features/shell/workbench/panel/WorkspaceViewStack";
import { WorkspaceViewBody } from "@/features/shell/workbench/panel/WorkspaceViewBody";

export interface ChatPanelHandle {
  focusComposer(): void;
}

interface Props {
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

const styles = stylex.create({
  // Cover the sidebar's retained slot while keeping Settings inside the workspace focus scope.
  settingsLayer: {
    position: "fixed",
    backgroundColor: "var(--app-content-surface)",
    backgroundImage: { default: null, ":is([data-paper-texture] *)": surface.texture },
  },
});

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
      closeLabel: t("dock.tabs.close", { title }),
      onCloseOthers: () => navigation.closeOtherDockViews(id),
      closeOthersLabel: t("dock.tabs.closeOthers"),
      onCloseAll: navigation.closeAllDockViews.bind(navigation),
      closeAllLabel: t("dock.tabs.closeAll"),
    };
  });
}

function dockLabel(
  viewsById: ReadonlyMap<string, { title: Label }>,
  activeViewId: string | null,
  t: Translate,
): string {
  const active = activeViewId === null ? undefined : viewsById.get(activeViewId);
  return active ? labelText(active.title, t) : t("dock.tabs.label");
}

export function ChatPanel({ ref }: Props) {
  const pickerRef = useRef<HTMLButtonElement>(null);
  const composerAccess = useComposer();
  const workspaceAccess = useWorkspace();

  const activeMainView = useActiveWorkspaceViewId();
  const settingsSourceViewId = workspaceAccess.navigation.useSettingsSourceViewId();
  const dock = useWorkspaceDock();
  const catalog = useContextDockCatalog();
  const views = useWorkspaceViews();
  const { layout: dockLayout, save: saveDockLayout } = useDockLayout();
  const [initialDockLayout] = useState(() => dockLayout ?? undefined);
  const activeSessionId = useActiveSessionId();
  const { data: activeSession } = useAgentSessionSummary(activeSessionId || undefined);
  const runtimeReady = useRuntimeCommandsAvailable();
  const conversationReady = runtimeReady || activeSession != null;
  const t = useT();
  const inspection = useWorkspaceInspection();
  const [rowWidth, dockRowRef] = useDockRowWidth();
  const dockAvailable = canPresentDock(rowWidth ?? 0);

  const {
    settingsOpen,
    settingsMounted,
    hasDockOwner,
    dockOpen,
    dockVisible,
    ownedDockViewIds,
    fullPaneViewId,
    inspectedFullPane,
    presentation,
    presentedViewId,
    panelLayout,
    dockChromeVisible,
    dockToggleShown,
    dockTabValue,
    viewsReady,
    dockOwnsMaterialKeys,
    dockAddressable,
    openViewIds,
    mountedViewIds,
  } = workspacePresentation({
    activeSessionId,
    dock,
    activeMainView,
    settingsSourceViewId,
    inspection,
    rowWidth,
    conversationReady,
  });
  const {
    workspaceLayerRef,
    settingsLayerRef,
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
    fullPaneViewId: settingsOpen ? WORKSPACE_SETTINGS_VIEW : fullPaneViewId,
    dockViewId: dock.activeViewId,
    dockViewIds: ownedDockViewIds,
    dockAvailable,
    dockVisible: !settingsOpen && dockVisible,
  });
  useLayoutEffect(
    () => registerComposerFocusPresentation(composerAccess, requestComposerFocus),
    [requestComposerFocus, composerAccess],
  );
  useImperativeHandle(ref, () => ({ focusComposer: requestComposerFocus }));

  const viewsById = new Map(views.map((view) => [view.id, view]));
  const dockReturn = returnAction(t, inspection, viewsById, returnToSource);

  const dockTabs = useDockTabs(ownedDockViewIds, viewsById, dock.activeViewId);
  const panelSizes = PANEL_SIZES[panelLayout];

  return (
    <AgentContentCard
      label={t("shell.region.workspace")}
      onFocusCapture={applyComposerReadingLimit}
    >
      <ContentVisibility visible={!settingsOpen} fallbackFocus={presentationRef}>
        <div
          ref={workspaceLayerRef}
          inert={settingsOpen}
          aria-hidden={settingsOpen || undefined}
          {...stylex.props(sh.paneNarrow, settingsOpen && paneStyles.inactiveView)}
        >
          {fullPaneViewId !== null && (
            <FullPaneHeader
              viewId={fullPaneViewId}
              view={
                fullPaneViewId === WORKSPACE_DOCK_CATALOG
                  ? CATALOG_VIEW
                  : viewsById.get(fullPaneViewId)
              }
              dockAvailable={hasDockOwner && dockAvailable}
              inspection={
                inspectedFullPane
                  ? { returnAction: dockReturn, onKeyDown: onMaterialKeyDown }
                  : undefined
              }
              picker={
                fullPanePickable(fullPaneViewId, hasDockOwner, viewsById) && (
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
          <AgentDockRow ref={dockRowRef} open={dockVisible} presentation={presentation}>
            <Group
              orientation="horizontal"
              defaultLayout={initialDockLayout}
              onLayoutChanged={(layout, meta) => {
                if (meta.isUserInteraction) saveDockLayout(meta.requestedLayout ?? layout);
              }}
              style={{ flex: 1, width: "100%", minHeight: 0 }}
            >
              <Panel
                id="conversation"
                {...panelSizes.conversation}
                style={{ overflow: "clip" }}
                onResize={({ inPixels }) => {
                  // Keep the reader's layout when the pane has no width, including
                  // when return focus arrives before the pane expands again.
                  const pane = conversationRef.current;
                  if (pane && inPixels > 0) pane.style.inlineSize = `${inPixels}px`;
                }}
              >
                <ConversationPane
                  ref={conversationRef}
                  hidden={presentation === "main"}
                  fallbackFocus={presentationRef}
                  ready={conversationReady}
                  sessionId={activeSessionId}
                />
              </Panel>
              {dockChromeVisible && <AgentDockSeparator label={t("dock.action.resize")} />}
              <Panel id="dock" defaultSize={480} {...panelSizes.dock}>
                {viewsReady && (
                  <SessionOwnedWorkspaceState sessionId={activeSessionId}>
                    <AgentContextDock
                      value={dockTabValue}
                      onValueChange={(id) => {
                        if (id !== null) workspaceAccess.navigation.selectDockView(id);
                      }}
                      ref={dockRef}
                      {...(presentation === "main"
                        ? { role: "presentation" }
                        : {
                            tabIndex: -1,
                            "aria-label": dockLabel(viewsById, dock.activeViewId, t),
                          })}
                      data-focus-inset=""
                      onKeyDown={dockOwnsMaterialKeys ? onMaterialKeyDown : undefined}
                    >
                      {dockChromeVisible && (
                        <DockHeader
                          key={activeSessionId}
                          tabs={dockTabs}
                          groups={catalog}
                          openViewIds={openViewIds}
                          returnAction={dockReturn}
                        />
                      )}
                      <WorkspaceViewStack
                        presentation={presentation}
                        shown={presentation === "main" || dockVisible}
                        presentedViewId={presentedViewId}
                        addressable={dockAddressable}
                        mountedViewIds={mountedViewIds}
                        mainViewRef={settingsOpen ? undefined : mainViewRef}
                        presentationRef={presentationRef}
                        catalog={catalog}
                        openViewIds={openViewIds}
                      />
                    </AgentContextDock>
                  </SessionOwnedWorkspaceState>
                )}
              </Panel>
            </Group>
            {dockToggleShown && (
              <AgentDockToggle
                open={dockVisible}
                onToggle={() => toggleChatDock(workspaceAccess, dockAvailable, dockOpen)}
                showLabel={dockAvailable ? t("dock.action.show") : t("dock.action.openFull")}
                hideLabel={t("dock.action.hide")}
              />
            )}
          </AgentDockRow>
        </div>
      </ContentVisibility>
      {settingsMounted && (
        <ContentVisibility visible={settingsOpen} fallbackFocus={presentationRef}>
          <div
            ref={settingsLayerRef}
            inert={!settingsOpen}
            aria-hidden={!settingsOpen || undefined}
            {...stylex.props(
              sh.overlay,
              styles.settingsLayer,
              !settingsOpen && paneStyles.inactiveView,
            )}
          >
            <FullPaneHeader
              viewId={WORKSPACE_SETTINGS_VIEW}
              view={viewsById.get(WORKSPACE_SETTINGS_VIEW)}
              inspection={undefined}
              dockAvailable={false}
              picker={null}
            />
            <AgentContextDock
              key={activeSessionId}
              value={WORKSPACE_SETTINGS_VIEW}
              role="presentation"
              data-focus-inset=""
              className={stylex.props(sh.paneNarrow).className}
            >
              <PanelHost density="page">
                <WorkspaceViewBody
                  ref={settingsOpen ? mainViewRef : undefined}
                  viewId={WORKSPACE_SETTINGS_VIEW}
                  presentation="main"
                />
              </PanelHost>
            </AgentContextDock>
          </div>
        </ContentVisibility>
      )}
    </AgentContentCard>
  );
}
