import { useWorkspace } from "@/features/workspace/public/workspace";
import { useComposer } from "@/features/composer/public/composer";
import * as stylex from "@stylexjs/stylex";
import {
  Activity,
  Fragment,
  useCallback,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type Ref,
} from "react";
import { PanelHost } from "@flame/ui/panel";
import { IconButton } from "@flame/ui";
import { iconGlyph } from "@/ui/icons/index";
import { ReturnButton } from "@/features/shell/workbench/panel/ReturnButton";
import { type AgentInput } from "@/features/agent/public/input";
import { CatalogPicker, type CatalogPickerGroup } from "@/ui/index";
import { ContentVisibility } from "@/ui/index";
import { knownIconName } from "@/ui/index";
import { AgentContentCard } from "@/ui/agent/index";
import { AgentContextDock, AgentDockRow, type AgentDockTab, AgentDockTabs } from "@/ui/agent/index";
import { AgentDockCatalog } from "@/ui/agent/index";
import { AgentDockToggle, AgentSurfaceHeader } from "@/ui/agent/index";
import { useActiveSession, useActiveSessionId } from "@/features/agent/public/session";
import { useAgentSessions } from "@/features/agent/public/session";
import { registerComposerFocusPresentation } from "@/features/composer/public/focus";
import {
  closeAllWorkspaceDockViews,
  closeOtherWorkspaceDockViews,
  closeWorkspaceDockView,
  closeWorkspaceView,
  collapseWorkspaceDock,
  openWorkspaceViewInDock,
  reorderWorkspaceDockView,
  selectWorkspaceDockView,
  showWorkspaceDock,
  useActiveWorkspaceViewId,
  useWorkspaceDock,
  useWorkspaceInspection,
} from "@/features/workspace/public/navigation";
import { useContextDockCatalog } from "@/features/workspace/public/contextDockCatalog";
import { type ContextDockDestinationGroup } from "@/features/workspace/public/contextDockCatalog";
import { useWorkspaceViews } from "@/features/workspace/public/views";
import { useDockWidth } from "@/features/workspace/public/sidebarDrawer";
import { Slot } from "@/extensions/host/Slot";
import { WORKSPACE_DOCK_CATALOG } from "@/features/workspace/public/navigation";
import { ChatStream } from "@/features/shell/workbench/panel/ChatStream";
import { RuntimeConnectionNotice } from "@/features/shell/workbench/panel/RuntimeConnectionNotice";
import { readingColumn as rc } from "@/features/chat/public/message/rendering";
import { space } from "@flame/ui/tokens.stylex";
import { RunAnnouncer } from "@/features/shell/workbench/panel/RunAnnouncer";
import { RunStatusPill } from "@/features/shell/workbench/panel/RunStatusPill";
import { SessionIdentity } from "@/features/shell/workbench/panel/SessionIdentity";
import { DockResizer } from "@/features/shell/workbench/panel/DockResizer";
import { HeaderDiffStat } from "@/features/shell/workbench/panel/HeaderDiffStat";
import { WorkspaceViewBody } from "@/features/shell/workbench/panel/WorkspaceViewBody";
import { WorkspaceWindowHeader } from "@/features/shell/workbench/panel/WorkspaceWindowHeader";
import { useT } from "@/lib/i18n/index";
import { canPresentDock, defaultDockWidth, dockWidthFromRatio } from "@/lib/shellGeometry";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";
import { useInspectionFocus } from "@/features/shell/workbench/panel/useInspectionFocus";
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

function useDockCatalogGroups(
  groups: ContextDockDestinationGroup[],
  openViewIds: ReadonlySet<string>,
): CatalogPickerGroup[] {
  const t = useT();
  return groups.map((group) => ({
    id: group.id,
    label: t(group.title),
    items: group.destinations.map((destination) => ({
      id: destination.viewId,
      label: t(destination.title),
      icon: knownIconName(destination.icon) ?? "panel-r",
      keywords: [destination.viewId, group.id, t(group.title)],
      active: openViewIds.has(destination.viewId),
    })),
  }));
}

function AddDockViewPicker({
  groups,
  openViewIds,
  activeViewId,
}: {
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
  activeViewId: string;
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const pickerGroups = useDockCatalogGroups(groups, openViewIds);
  const triggerRef = useRef<HTMLButtonElement>(null);

  return (
    <CatalogPicker
      groups={pickerGroups}
      label={t("dock.action.browse")}
      placeholder={t("dock.picker.placeholder")}
      emptyLabel={t("dock.picker.empty")}
      clearLabel={t("common.clearSearch")}
      trigger={
        <IconButton
          ref={triggerRef}
          icon={iconGlyph("plus")}
          size="sm"
          quiet
          aria-label={t("dock.action.browse")}
          data-dock-view-id={activeViewId}
          className={stylex.props(sh.toEnd).className}
        />
      }
      onSelect={(item) =>
        openWorkspaceViewInDock(workspaceAccess, item.id, triggerRef.current ?? undefined)
      }
    />
  );
}

function DockCatalogPage({
  groups,
  openViewIds,
  titleHidden,
}: {
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
  titleHidden: boolean;
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  return (
    <AgentDockCatalog
      groups={useDockCatalogGroups(groups, openViewIds)}
      title={t("dock.catalog.title")}
      titleHidden={titleHidden}
      placeholder={t("dock.picker.placeholder")}
      emptyLabel={t("dock.picker.empty")}
      clearLabel={t("common.clearSearch")}
      onSelect={openWorkspaceViewInDock.bind(null, workspaceAccess)}
    />
  );
}

function DockHeader({
  tabs,
  groups,
  openViewIds,
  returnAction,
}: {
  tabs: AgentDockTab[];
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
  returnAction?: { label: string; onReturn: () => void };
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  return (
    <AgentSurfaceHeader divider={false}>
      {returnAction && <ReturnButton label={returnAction.label} onReturn={returnAction.onReturn} />}
      <AgentDockTabs
        tabs={tabs}
        ariaLabel={t("dock.tabs.label")}
        reorder={{
          onMove: reorderWorkspaceDockView.bind(null, workspaceAccess),
          moveLeftLabel: t("dock.tabs.moveLeft"),
          moveRightLabel: t("dock.tabs.moveRight"),
        }}
      />
      <AddDockViewPicker
        groups={groups}
        openViewIds={openViewIds}
        activeViewId={tabs.find((tab) => tab.active)?.id ?? WORKSPACE_DOCK_CATALOG}
      />
      <IconButton
        icon={iconGlyph("maximize")}
        size="sm"
        aria-label={t("dock.action.expand")}
        onClick={() => showWorkspaceDock(workspaceAccess, "main")}
      />
    </AgentSurfaceHeader>
  );
}

export function ChatPanel({ onSend, ref }: Props) {
  const composerAccess = useComposer();
  const workspaceAccess = useWorkspace();

  const activeMainView = useActiveWorkspaceViewId();
  const dock = useWorkspaceDock();
  const catalog = useContextDockCatalog();
  const views = useWorkspaceViews();
  const { width: dockWidthRatio } = useDockWidth();
  const { data: sessions } = useAgentSessions();
  const activeSession = useActiveSession();
  const activeSessionId = useActiveSessionId();
  const t = useT();
  const inspection = useWorkspaceInspection();
  const [rowWidth, setRowWidth] = useState<number | null>(null);
  const measuredWidth = rowWidth ?? 0;
  const dockAvailable = canPresentDock(measuredWidth);
  const dockWidth =
    dockWidthRatio === null
      ? defaultDockWidth(measuredWidth)
      : dockWidthFromRatio(dockWidthRatio, measuredWidth);

  const hasDockOwner = activeSessionId !== "";
  const showingCatalog = dock.activeViewId === WORKSPACE_DOCK_CATALOG;
  const dockOpen = hasDockOwner && dock.open && (showingCatalog || dock.viewIds.length > 0);
  const dockVisible = dockOpen && dockAvailable;
  const ownedDockViewIds = hasDockOwner ? dock.viewIds : [];
  const conversationReady = sessions !== undefined;
  const viewsReady = conversationReady || activeMainView !== null || dock.open;
  const fullPaneViewId =
    activeMainView ??
    (dockOpen && rowWidth !== null && !dockAvailable && inspection?.viewId === dock.activeViewId
      ? dock.activeViewId
      : null);
  const inspectedFullPane = fullPaneViewId !== null && inspection?.viewId === fullPaneViewId;
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

  const dockRowRef = useCallback((row: HTMLDivElement | null) => {
    if (!row) return;
    const reconcile = () => setRowWidth(row.clientWidth);
    reconcile();
    let delivered = false;
    let queued = 0;
    const observer = new ResizeObserver(() => {
      if (!delivered) {
        delivered = true;
        reconcile();
        return;
      }
      if (queued) return;
      queued = requestAnimationFrame(() => {
        queued = 0;
        reconcile();
      });
    });
    observer.observe(row);
    return () => {
      if (queued) cancelAnimationFrame(queued);
      observer.disconnect();
    };
  }, []);

  const viewsById = new Map(views.map((view) => [view.id, view]));
  const returnViewId = inspection?.returnViewId ?? inspection?.returnDockId;
  const returnTitle = returnViewId ? viewsById.get(returnViewId)?.title : undefined;
  const returnAction = inspection
    ? {
        label:
          returnViewId === "subagents" && inspection.returnSubagentRunId
            ? t("dock.action.returnToTask")
            : returnViewId === WORKSPACE_DOCK_CATALOG
              ? t("dock.action.returnToCatalog")
              : returnTitle
                ? t("dock.action.returnToView", { view: t(returnTitle) })
                : t("dock.action.returnToConversation"),
        onReturn: returnToSource,
      }
    : undefined;

  const dockTabs = ownedDockViewIds.map((id) => {
    const view = viewsById.get(id);
    const title = view ? t(view.title) : id;
    const Badge = view?.badge;
    return {
      id,
      title,
      icon: knownIconName(view?.icon),
      badge: Badge ? <Badge /> : undefined,
      active: id === dock.activeViewId,
      onClose: () => closeWorkspaceDockView(workspaceAccess, id),
      closeLabel: `${t("common.close")} ${title}`,
      onCloseOthers: () => closeOtherWorkspaceDockViews(workspaceAccess, id),
      closeOthersLabel: t("dock.tabs.closeOthers"),
      onCloseAll: closeAllWorkspaceDockViews.bind(null, workspaceAccess),
      closeAllLabel: t("dock.tabs.closeAll"),
    };
  });
  const openViewIds = new Set(ownedDockViewIds);
  const presentedMain = fullPaneViewId !== null;
  const mountedViewIds = new Set(ownedDockViewIds);
  if (activeMainView !== null) mountedViewIds.add(activeMainView);
  for (let source = inspection; source; source = source.previous) {
    if (source.returnViewId && source.returnViewId !== source.viewId) {
      mountedViewIds.add(source.returnViewId);
    }
  }
  mountedViewIds.delete(WORKSPACE_DOCK_CATALOG);
  const presentedViewId = presentedMain ? fullPaneViewId : dock.activeViewId;
  const catalogView = { title: "dock.catalog.title", icon: "panel-r", dock: "workspace" } as const;

  return (
    <AgentContentCard label={t("shell.region.workspace")}>
      {presentedMain && (
        <WorkspaceWindowHeader
          view={
            fullPaneViewId === WORKSPACE_DOCK_CATALOG ? catalogView : viewsById.get(fullPaneViewId)
          }
          exit={
            inspectedFullPane && returnAction
              ? { kind: "return", ...returnAction }
              : {
                  kind: "close",
                  onClose: () => closeWorkspaceView(workspaceAccess, fullPaneViewId),
                }
          }
          onOpenInDock={
            dockAvailable
              ? () => openWorkspaceViewInDock(workspaceAccess, fullPaneViewId)
              : undefined
          }
          onKeyDown={inspectedFullPane ? onMaterialKeyDown : undefined}
        />
      )}
      <AgentDockRow
        ref={dockRowRef}
        open={dockVisible}
        width={dockWidth}
        presentation={presentedMain ? "main" : "dock"}
      >
        <div
          ref={conversationRef}
          hidden={presentedMain}
          tabIndex={-1}
          role="region"
          aria-label={t("shell.region.conversation")}
          data-focus-inset=""
          {...stylex.props(sh.paneNarrow, paneStyles.conversation)}
        >
          <ContentVisibility visible={!presentedMain} fallbackFocus={presentationRef}>
            {conversationReady ? (
              <>
                <AgentSurfaceHeader corner="window">
                  <SessionIdentity
                    sessionId={activeSessionId}
                    title={activeSession?.title.trim() || t("sidebar.action.newSession")}
                    workspacePath={activeSession?.workspace.path}
                  />
                  <RunStatusPill />
                  <span {...stylex.props(sh.spacer)} />
                  <Slot name="chat.header.meta" />
                  <HeaderDiffStat />
                </AgentSurfaceHeader>
                <Slot name="chat.search" />
                <ChatStream onSend={onSend} />
                <RunAnnouncer />
              </>
            ) : (
              <div {...stylex.props(rc.box, rc.gutter, paneStyles.pendingConnection)}>
                <RuntimeConnectionNotice />
              </div>
            )}
          </ContentVisibility>
        </div>
        {!presentedMain && dockVisible && <DockResizer width={dockWidth} />}
        {viewsReady && (
          <SessionOwnedWorkspaceState sessionId={activeSessionId}>
            <AgentContextDock
              value={presentedViewId === WORKSPACE_DOCK_CATALOG ? null : presentedViewId}
              onValueChange={(id) => {
                if (id !== null) selectWorkspaceDockView(workspaceAccess, id);
              }}
              ref={dockRef}
              role={presentedMain ? "presentation" : undefined}
              tabIndex={presentedMain ? undefined : -1}
              aria-label={
                presentedMain
                  ? undefined
                  : t(viewsById.get(dock.activeViewId ?? "")?.title ?? "dock.tabs.label")
              }
              data-focus-inset=""
              onKeyDown={
                activeMainView === null || inspectedFullPane ? onMaterialKeyDown : undefined
              }
            >
              {!presentedMain && dockVisible && (
                <DockHeader
                  tabs={dockTabs}
                  groups={catalog}
                  openViewIds={openViewIds}
                  returnAction={returnAction}
                />
              )}
              <div {...stylex.props(sh.anchor)}>
                <Activity mode={presentedViewId === WORKSPACE_DOCK_CATALOG ? "visible" : "hidden"}>
                  <div
                    ref={fullPaneViewId === WORKSPACE_DOCK_CATALOG ? mainViewRef : undefined}
                    role={presentedMain ? "region" : undefined}
                    aria-label={presentedMain ? t(catalogView.title) : undefined}
                    tabIndex={presentedMain ? -1 : undefined}
                    data-focus-inset=""
                    data-dock-view-id={WORKSPACE_DOCK_CATALOG}
                    {...stylex.props(sh.overlay)}
                  >
                    <DockCatalogPage
                      groups={catalog}
                      openViewIds={openViewIds}
                      titleHidden={presentedMain}
                    />
                  </div>
                </Activity>
                {[...mountedViewIds].map((viewId) => {
                  const visible = viewId === presentedViewId && (presentedMain || dockVisible);
                  return (
                    <div
                      key={viewId}
                      inert={!visible}
                      aria-hidden={!visible || undefined}
                      data-dock-view-id={activeMainView === null ? viewId : undefined}
                      {...stylex.props(sh.overlay, !visible && paneStyles.inactiveView)}
                    >
                      <ContentVisibility visible={visible} fallbackFocus={presentationRef}>
                        <PanelHost density={presentedMain ? "page" : "compact"}>
                          <WorkspaceViewBody
                            ref={viewId === fullPaneViewId ? mainViewRef : undefined}
                            viewId={viewId}
                            presentation={presentedMain ? "main" : "dock"}
                          />
                        </PanelHost>
                      </ContentVisibility>
                    </div>
                  );
                })}
              </div>
            </AgentContextDock>
          </SessionOwnedWorkspaceState>
        )}
        {viewsReady && !presentedMain && hasDockOwner && (
          <AgentDockToggle
            open={dockVisible}
            onToggle={
              !dockAvailable
                ? () => showWorkspaceDock(workspaceAccess, "main")
                : dockOpen
                  ? collapseWorkspaceDock.bind(null, workspaceAccess)
                  : showWorkspaceDock.bind(null, workspaceAccess)
            }
            showLabel={dockAvailable ? t("dock.action.show") : t("dock.action.openFull")}
            hideLabel={t("dock.action.hide")}
          />
        )}
      </AgentDockRow>
    </AgentContentCard>
  );
}

const paneStyles = stylex.create({
  conversation: { containerType: "inline-size", containerName: "conversation" },
  pendingConnection: { paddingBlock: space.s4 },
  // Removing layout makes WebKit reset the retained scrollport's reading position.
  inactiveView: { visibility: "hidden" },
});
