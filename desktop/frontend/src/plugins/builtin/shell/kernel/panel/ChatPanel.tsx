import * as stylex from "@stylexjs/stylex";
import { Activity, Fragment, useCallback, useState, type ReactNode } from "react";
import { dockWidthRow } from "./dockWidth";
import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import type { ViewPlacement } from "@/plugins/builtin/workspace/public/viewPlacement";
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
import { ViewPlacementProvider } from "@/plugins/builtin/workspace/public/viewPlacement";
import { WorkspaceViewBody } from "./WorkspaceViewBody";
import { useT } from "@/lib/i18n";
import { canPresentDock } from "@/lib/shellGeometry";
import { shellStyles as sh } from "../shellStyles";

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
      icon: knownIconName(destination.icon),
      keywords: [destination.viewId, group.id],
      active: openViewIds.has(destination.viewId),
    })),
  }));
}

function AddDockViewPicker({
  groups,
  openViewIds,
}: {
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
}) {
  const t = useT();
  const pickerGroups = useDockCatalogGroups(groups, openViewIds);

  return (
    <CatalogPicker
      groups={pickerGroups}
      label={t("dock.action.browse")}
      placeholder={t("dock.picker.placeholder")}
      emptyLabel={t("dock.picker.empty")}
      onSelect={(item) => openWorkspaceViewInDock(item.id)}
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
      onSelect={openWorkspaceViewInDock}
    />
  );
}

function DockHeader({
  tabs,
  groups,
  openViewIds,
}: {
  tabs: AgentDockTab[];
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
}) {
  const t = useT();
  return (
    <AgentSurfaceHeader divider={false}>
      <AgentDockTabs
        tabs={tabs}
        ariaLabel={t("dock.tabs.label")}
        onReorder={reorderWorkspaceDockView}
      />
      <AddDockViewPicker groups={groups} openViewIds={openViewIds} />
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

  const placementFor = (id: string, placement: "full" | "dock"): ViewPlacement => ({
    placement,
    splittable: viewsById.get(id)?.dock !== undefined,
    onOpenInDock: () => openWorkspaceViewInDock(id),
    onClose: () => (placement === "dock" ? closeWorkspaceDockView(id) : closeWorkspaceView(id)),
  });

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

  return (
    <AgentContentCard label={t("shell.region.workspace")}>
      {activeMainView !== null && (
        <SessionOwnedWorkspaceState sessionId={activeSessionId}>
          <ViewPlacementProvider value={placementFor(activeMainView, "full")}>
            <WorkspaceViewBody viewId={activeMainView} />
          </ViewPlacementProvider>
        </SessionOwnedWorkspaceState>
      )}
      <Activity mode={activeMainView === null ? "visible" : "hidden"}>
        <AgentDockRow
          ref={dockRowRef}
          open={dockOpen && dockAvailable}
          style={dockWidthRow(dockWidthRatio)}
        >
          <div {...stylex.props(sh.paneNarrow, conversationPane.container)}>
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
            <AgentContextDock>
              {hasDockOwner && (
                <DockHeader tabs={dockTabs} groups={catalog} openViewIds={openViewIds} />
              )}
              <div {...stylex.props(sh.anchor)}>
                {showingCatalog && (
                  <div {...stylex.props(sh.overlay)}>
                    <DockCatalogPage groups={catalog} openViewIds={openViewIds} />
                  </div>
                )}
                {ownedDockViewIds.map((viewId) => (
                  <Activity key={viewId} mode={viewId === dock.activeViewId ? "visible" : "hidden"}>
                    <div data-dock-view-id={viewId} {...stylex.props(sh.overlay)}>
                      <ViewPlacementProvider value={placementFor(viewId, "dock")}>
                        <WorkspaceViewBody viewId={viewId} />
                      </ViewPlacementProvider>
                    </div>
                  </Activity>
                ))}
              </div>
            </AgentContextDock>
          </SessionOwnedWorkspaceState>
          {hasDockOwner && (
            <AgentDockToggle
              open={dockOpen && dockAvailable}
              onToggle={dockOpen ? collapseWorkspaceDock : showWorkspaceDock}
              showLabel={t("dock.action.show")}
              hideLabel={t("dock.action.hide")}
              disabled={!dockAvailable}
              unavailableLabel={t("dock.action.unavailable")}
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
