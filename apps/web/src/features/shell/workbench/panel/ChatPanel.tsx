import * as stylex from "@stylexjs/stylex";
import {
  Activity,
  Fragment,
  useCallback,
  useImperativeHandle,
  useRef,
  useState,
  type ReactNode,
  type Ref,
} from "react";
import { PanelHost } from "@flame/ui/panel";
import { IconButton } from "@flame/ui";
import { iconGlyph } from "@/ui/icons/index";
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
  const t = useT();
  const pickerGroups = useDockCatalogGroups(groups, openViewIds);
  const triggerRef = useRef<HTMLButtonElement>(null);

  return (
    <CatalogPicker
      groups={pickerGroups}
      label={t("dock.action.browse")}
      placeholder={t("dock.picker.placeholder")}
      emptyLabel={t("dock.picker.empty")}
      trigger={
        <IconButton
          ref={triggerRef}
          icon={iconGlyph("plus")}
          size="sm"
          quiet
          aria-label={t("dock.action.browse")}
          data-dock-view-id={activeViewId}
        />
      }
      onSelect={(item) => openWorkspaceViewInDock(item.id, triggerRef.current ?? undefined)}
    />
  );
}

function DockCatalogPage({
  groups,
  openViewIds,
}: {
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
}) {
  const t = useT();
  return (
    <AgentDockCatalog
      groups={useDockCatalogGroups(groups, openViewIds)}
      title={t("dock.catalog.title")}
      description={t("dock.catalog.description")}
      placeholder={t("dock.picker.placeholder")}
      emptyLabel={t("dock.picker.empty")}
      onSelect={openWorkspaceViewInDock}
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
  const t = useT();
  return (
    <AgentSurfaceHeader divider={false}>
      {returnAction && (
        <IconButton
          icon={iconGlyph("arrow-left")}
          size="sm"
          quiet
          aria-label={returnAction.label}
          onClick={returnAction.onReturn}
        />
      )}
      <AgentDockTabs
        tabs={tabs}
        ariaLabel={t("dock.tabs.label")}
        reorder={{
          onMove: reorderWorkspaceDockView,
          moveLeftLabel: t("dock.tabs.moveLeft"),
          moveRightLabel: t("dock.tabs.moveRight"),
        }}
      />
      <AddDockViewPicker
        groups={groups}
        openViewIds={openViewIds}
        activeViewId={tabs.find((tab) => tab.active)?.id ?? WORKSPACE_DOCK_CATALOG}
      />
    </AgentSurfaceHeader>
  );
}

export function ChatPanel({ onSend, ref }: Props) {
  const activeMainView = useActiveWorkspaceViewId();
  const dock = useWorkspaceDock();
  const catalog = useContextDockCatalog();
  const views = useWorkspaceViews();
  const { width: dockWidthRatio } = useDockWidth();
  const { isLoading } = useAgentSessions();
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
  const shellVisible = !isLoading || activeMainView !== null || dock.open;
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

  if (!shellVisible) return null;

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
      onSelect: () => selectWorkspaceDockView(id),
      onClose: () => closeWorkspaceDockView(id),
      closeLabel: `${t("common.close")} ${title}`,
      onCloseOthers: () => closeOtherWorkspaceDockViews(id),
      closeOthersLabel: t("dock.tabs.closeOthers"),
      onCloseAll: closeAllWorkspaceDockViews,
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
              : { kind: "close", onClose: () => closeWorkspaceView(fullPaneViewId) }
          }
          onOpenInDock={
            dockAvailable ? (origin) => openWorkspaceViewInDock(fullPaneViewId, origin) : undefined
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
          {...stylex.props(sh.paneNarrow, conversationPane.container)}
        >
          <ContentVisibility visible={!presentedMain}>
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
            <ChatStream onSend={onSend} />
            <RunAnnouncer />
          </ContentVisibility>
        </div>
        {!presentedMain && dockVisible && <DockResizer width={dockWidth} />}
        <SessionOwnedWorkspaceState sessionId={activeSessionId}>
          <AgentContextDock
            ref={dockRef}
            role={presentedMain ? "presentation" : undefined}
            tabIndex={presentedMain ? undefined : -1}
            aria-label={
              presentedMain
                ? undefined
                : t(viewsById.get(dock.activeViewId ?? "")?.title ?? "dock.tabs.label")
            }
            data-focus-inset=""
            onKeyDown={activeMainView === null || inspectedFullPane ? onMaterialKeyDown : undefined}
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
                  <DockCatalogPage groups={catalog} openViewIds={openViewIds} />
                </div>
              </Activity>
              {[...mountedViewIds].map((viewId) => (
                <div
                  key={viewId}
                  hidden={viewId !== presentedViewId}
                  data-dock-view-id={activeMainView === null ? viewId : undefined}
                  {...stylex.props(sh.overlay)}
                >
                  <ContentVisibility
                    visible={viewId === presentedViewId && (presentedMain || dockVisible)}
                  >
                    <PanelHost hosted={!presentedMain}>
                      <WorkspaceViewBody
                        ref={viewId === fullPaneViewId ? mainViewRef : undefined}
                        viewId={viewId}
                      />
                    </PanelHost>
                  </ContentVisibility>
                </div>
              ))}
            </div>
          </AgentContextDock>
        </SessionOwnedWorkspaceState>
        {!presentedMain && hasDockOwner && (
          <AgentDockToggle
            open={dockVisible}
            onToggle={
              !dockAvailable
                ? () => showWorkspaceDock("main")
                : dockOpen
                  ? collapseWorkspaceDock
                  : showWorkspaceDock
            }
            showLabel={dockAvailable ? t("dock.action.show") : t("dock.action.openFull")}
            hideLabel={t("dock.action.hide")}
          />
        )}
      </AgentDockRow>
    </AgentContentCard>
  );
}

const conversationPane = stylex.create({
  container: { containerType: "inline-size", containerName: "conversation" },
});
