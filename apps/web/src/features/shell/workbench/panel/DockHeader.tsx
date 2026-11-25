import { useWorkspace } from "@/features/workspace/public/workspace";
import * as stylex from "@stylexjs/stylex";
import { useRef, type RefObject } from "react";
import { IconButton } from "@flame/ui";
import { iconGlyph } from "@/ui/icons/index";
import { ReturnButton } from "@/features/shell/workbench/panel/ReturnButton";
import { CatalogPicker, type CatalogPickerGroup } from "@/ui/index";
import {
  type AgentDockTab,
  AgentDockTabs,
  AgentDockCatalog,
  AgentSurfaceHeader,
} from "@/ui/agent/index";
import {
  openWorkspaceViewInDock,
  showWorkspaceDock,
  dockViewMarker,
  WORKSPACE_DOCK_CATALOG,
} from "@/features/workspace/public/navigation";
import { type ContextDockDestinationGroup } from "@/features/workspace/public/contextDockCatalog";
import { useT, labelText } from "@/lib/i18n/index";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";

function useDockCatalogGroups(
  groups: ContextDockDestinationGroup[],
  openViewIds: ReadonlySet<string>,
): CatalogPickerGroup[] {
  const t = useT();
  return groups.map((group) => ({
    id: group.id,
    label: labelText(group.title, t),
    items: group.destinations.map((destination) => ({
      id: destination.viewId,
      label: labelText(destination.title, t),
      keywords: [destination.viewId, group.id, labelText(group.title, t)],
      active: openViewIds.has(destination.viewId),
    })),
  }));
}

export function WorkspaceViewPicker({
  triggerRef,
  groups,
  openViewIds,
  activeViewId,
  onSelect,
}: {
  triggerRef: RefObject<HTMLButtonElement | null>;
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
  activeViewId: string;
  onSelect: (viewId: string, origin?: HTMLElement) => void;
}) {
  const t = useT();
  const pickerGroups = useDockCatalogGroups(groups, openViewIds);

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
          {...dockViewMarker(activeViewId)}
          className={stylex.props(sh.toEnd).className}
        />
      }
      onSelect={(item) => {
        if (item.id === activeViewId) {
          const trigger = triggerRef.current;
          const active = trigger?.ownerDocument.activeElement;
          if (
            trigger?.isConnected &&
            (!active ||
              active === trigger.ownerDocument.body ||
              active === trigger.ownerDocument.documentElement)
          ) {
            trigger.focus({ preventScroll: true });
          }
          return;
        }
        onSelect(item.id, triggerRef.current ?? undefined);
      }}
    />
  );
}

export function DockCatalogPage({
  groups,
  openViewIds,
}: {
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  return (
    <AgentDockCatalog
      groups={useDockCatalogGroups(groups, openViewIds)}
      title={t("dock.catalog.title")}
      placeholder={t("dock.picker.placeholder")}
      emptyLabel={t("dock.picker.empty")}
      clearLabel={t("common.clearSearch")}
      onSelect={openWorkspaceViewInDock.bind(null, workspaceAccess)}
    />
  );
}

export function DockHeader({
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
  const pickerRef = useRef<HTMLButtonElement>(null);
  return (
    <AgentSurfaceHeader divider={false}>
      {returnAction && <ReturnButton label={returnAction.label} onReturn={returnAction.onReturn} />}
      <AgentDockTabs
        tabs={tabs}
        ariaLabel={t("dock.tabs.label")}
        fallbackFocus={pickerRef}
        reorder={{
          onMove: (id, toIndex) => {
            const result = workspaceAccess.navigation.reorderDockView(id, toIndex);
            const tab = tabs.find((tab) => tab.id === id);
            return result && tab ? t("dock.tabs.moved", { title: tab.title, ...result }) : null;
          },
          handleLabel: (title) => t("dock.tabs.reorder", { title }),
          moveLeftLabel: t("dock.tabs.moveLeft"),
          moveRightLabel: t("dock.tabs.moveRight"),
        }}
      />
      <WorkspaceViewPicker
        triggerRef={pickerRef}
        groups={groups}
        openViewIds={openViewIds}
        activeViewId={tabs.find((tab) => tab.active)?.id ?? WORKSPACE_DOCK_CATALOG}
        onSelect={openWorkspaceViewInDock.bind(null, workspaceAccess)}
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
