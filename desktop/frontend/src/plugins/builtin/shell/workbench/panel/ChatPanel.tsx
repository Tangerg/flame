import * as stylex from "@stylexjs/stylex";
import { Activity, Fragment, useCallback, useRef, useState, type ReactNode } from "react";
import { PanelHost } from "@flame/ui/panel";
import { IconButton } from "@flame/ui";
import { iconGlyph } from "@/ui/icons";
import { dockWidthRow } from "./dockWidth";
import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import { CatalogPicker, knownIconName, type CatalogPickerGroup } from "@/ui";
import {
  AgentContentCard,
  AgentContextDock,
  AgentDockCatalog,
  AgentDockRow,
  type AgentDockTab,
  AgentDockTabs,
  AgentDockToggle,
  AgentSurfaceHeader,
} from "@/ui/agent";
import {
  useActiveSession,
  useActiveSessionId,
  useAgentSessions,
} from "@/plugins/builtin/agent/public/session";
import {
  closeAllWorkspaceDockViews,
  closeOtherWorkspaceDockViews,
  closeWorkspaceDockView,
  closeWorkspaceView,
  collapseWorkspaceDock,
  openWorkspaceView,
  openWorkspaceViewInDock,
  reorderWorkspaceDockView,
  selectWorkspaceDockView,
  showWorkspaceDock,
  useActiveWorkspaceViewId,
  useWorkspaceDock,
} from "@/plugins/builtin/workspace/public/navigation";
import {
  useContextDockCatalog,
  type ContextDockDestinationGroup,
} from "@/plugins/builtin/workspace/public/contextDockCatalog";
import { useWorkspaceViews } from "@/plugins/sdk";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { Slot } from "@/plugins/host/Slot";
import { WORKSPACE_DOCK_CATALOG } from "@/plugins/builtin/workspace/public/navigation";
import { ChatStream } from "./ChatStream";
import { RunAnnouncer } from "./RunAnnouncer";
import { RunStatusPill } from "./RunStatusPill";
import { SessionIdentity } from "./SessionIdentity";
import { DockResizer } from "./DockResizer";
import { HeaderDiffStat } from "./HeaderDiffStat";
import { WorkspaceViewBody } from "./WorkspaceViewBody";
import { WorkspaceWindowHeader } from "./WorkspaceWindowHeader";
import { useT } from "@/lib/i18n";
import { canPresentDock } from "@/lib/shellGeometry";
import { shellStyles as sh } from "../shellStyles";
import { useInspectionFocus } from "./useInspectionFocus";

interface Props {
  onSend: (input: AgentInput) => boolean;
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

export function ChatPanel({ onSend }: Props) {
  const activeMainView = useActiveWorkspaceViewId();
  const dock = useWorkspaceDock();
  const catalog = useContextDockCatalog();
  const views = useWorkspaceViews();
  const { width: dockWidthRatio } = useDockWidth();
  const { isLoading } = useAgentSessions();
  const activeSession = useActiveSession();
  const activeSessionId = useActiveSessionId();
  const t = useT();
  const [dockAvailable, setDockAvailable] = useState(true);

  const hasDockOwner = activeSessionId !== "";
  const showingCatalog = dock.activeViewId === WORKSPACE_DOCK_CATALOG;
  const dockOpen = hasDockOwner && dock.open && (showingCatalog || dock.viewIds.length > 0);
  const ownedDockViewIds = hasDockOwner ? dock.viewIds : [];
  const shellVisible = !isLoading || activeMainView !== null || dock.open;
  const { inspection, dockRef, conversationRef, mainViewRef, returnToSource, onDockKeyDown } =
    useInspectionFocus({
      sessionId: activeSessionId,
      mainViewId: activeMainView,
      dockViewId: dock.activeViewId,
      dockViewIds: ownedDockViewIds,
      dockVisible: dockOpen && dockAvailable,
    });

  const dockRowRef = useCallback((row: HTMLDivElement | null) => {
    if (!row) return;
    const reconcile = () => {
      const available = canPresentDock(row.clientWidth);
      setDockAvailable((current) => (current === available ? current : available));
    };
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
  const fullWidthViewId =
    dock.activeViewId && dock.activeViewId !== WORKSPACE_DOCK_CATALOG
      ? dock.activeViewId
      : (ownedDockViewIds[0] ?? "file");

  return (
    <AgentContentCard label={t("shell.region.workspace")}>
      {activeMainView !== null && (
        <>
          <WorkspaceWindowHeader
            view={viewsById.get(activeMainView)}
            onOpenInDock={(origin) => openWorkspaceViewInDock(activeMainView, origin)}
            onClose={() => closeWorkspaceView(activeMainView)}
          />
          <SessionOwnedWorkspaceState sessionId={activeSessionId}>
            <WorkspaceViewBody ref={mainViewRef} viewId={activeMainView} />
          </SessionOwnedWorkspaceState>
        </>
      )}
      <Activity mode={activeMainView === null ? "visible" : "hidden"}>
        <AgentDockRow
          ref={dockRowRef}
          open={dockOpen && dockAvailable}
          style={dockWidthRow(dockWidthRatio)}
        >
          <div
            ref={conversationRef}
            tabIndex={-1}
            role="region"
            aria-label={t("shell.region.conversation")}
            data-focus-inset=""
            {...stylex.props(sh.paneNarrow, conversationPane.container)}
          >
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
          </div>
          {dockOpen && dockAvailable && <DockResizer />}
          <SessionOwnedWorkspaceState sessionId={activeSessionId}>
            <AgentContextDock
              ref={dockRef}
              tabIndex={-1}
              aria-label={t(viewsById.get(dock.activeViewId ?? "")?.title ?? "dock.tabs.label")}
              data-focus-inset=""
              onKeyDown={onDockKeyDown}
            >
              {hasDockOwner && (
                <DockHeader
                  tabs={dockTabs}
                  groups={catalog}
                  openViewIds={openViewIds}
                  returnAction={returnAction}
                />
              )}
              <div {...stylex.props(sh.anchor)}>
                <Activity mode={showingCatalog ? "visible" : "hidden"}>
                  <div data-dock-view-id={WORKSPACE_DOCK_CATALOG} {...stylex.props(sh.overlay)}>
                    <DockCatalogPage groups={catalog} openViewIds={openViewIds} />
                  </div>
                </Activity>
                {ownedDockViewIds.map((viewId) => (
                  <Activity key={viewId} mode={viewId === dock.activeViewId ? "visible" : "hidden"}>
                    <div data-dock-view-id={viewId} {...stylex.props(sh.overlay)}>
                      <PanelHost>
                        <WorkspaceViewBody viewId={viewId} />
                      </PanelHost>
                    </div>
                  </Activity>
                ))}
              </div>
            </AgentContextDock>
          </SessionOwnedWorkspaceState>
          {hasDockOwner && (
            <AgentDockToggle
              open={dockOpen && dockAvailable}
              onToggle={
                !dockAvailable
                  ? () => openWorkspaceView(fullWidthViewId)
                  : dockOpen
                    ? collapseWorkspaceDock
                    : showWorkspaceDock
              }
              showLabel={dockAvailable ? t("dock.action.show") : t("dock.action.openFull")}
              hideLabel={t("dock.action.hide")}
            />
          )}
        </AgentDockRow>
      </Activity>
    </AgentContentCard>
  );
}

const conversationPane = stylex.create({
  container: { containerType: "inline-size", containerName: "conversation" },
});
