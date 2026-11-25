import { useWorkspace } from "@/features/workspace/public/workspace";
import { type FocusEvent } from "react";
import {
  collapseWorkspaceDock,
  showWorkspaceDock,
  useWorkspaceDock,
  useWorkspaceInspection,
  WORKSPACE_DOCK_CATALOG,
} from "@/features/workspace/public/navigation";
import { composerFocusRegion } from "@/features/shell/workbench/panel/FloatingComposer";
import { type Translate, labelText, type Label } from "@/lib/i18n/index";
import { canPresentDock } from "@/features/shell/workbench/panel/shellGeometry";
import { useInspectionFocus } from "@/features/shell/workbench/panel/useInspectionFocus";

export function returnActionLabel(
  t: Translate,
  inspection: NonNullable<ReturnType<typeof useWorkspaceInspection>>,
  viewsById: ReadonlyMap<string, { title: Label }>,
): string {
  const returnViewId = inspection.returnViewId ?? inspection.returnDockId;
  if (returnViewId === "subagents" && inspection.returnSubagentRunId)
    return t("dock.action.returnToTask");
  if (returnViewId === WORKSPACE_DOCK_CATALOG) return t("dock.action.returnToCatalog");
  const returnTitle = returnViewId ? viewsById.get(returnViewId)?.title : undefined;
  return returnTitle
    ? t("dock.action.returnToView", { view: labelText(returnTitle, t) })
    : t("dock.action.returnToConversation");
}

export function applyComposerReadingLimit({ currentTarget, target }: FocusEvent<HTMLElement>) {
  // Portaled menus keep their opener's reading context through native focus handoffs.
  if (!currentTarget.contains(target)) return;
  const region = composerFocusRegion(target);
  // Jump navigates the current frame without changing its editing or reading budget.
  if (region === "overlay") return;
  currentTarget.style.setProperty(
    "--composer-reading-limit",
    region === "editing" ? "100%" : "calc(100% / 3)",
  );
}

export const CATALOG_VIEW = { title: "dock.catalog.title", dock: "workspace" } as const;

// Without room for the dock the toggle opens the view full-page instead.
export function toggleChatDock(
  workspaceAccess: ReturnType<typeof useWorkspace>,
  dockAvailable: boolean,
  dockOpen: boolean,
) {
  if (!dockAvailable) showWorkspaceDock(workspaceAccess, "main");
  else if (dockOpen) collapseWorkspaceDock(workspaceAccess);
  else showWorkspaceDock(workspaceAccess);
}

export type InspectionFocus = ReturnType<typeof useInspectionFocus>;

type WorkspaceDock = ReturnType<typeof useWorkspaceDock>;

type WorkspaceInspection = ReturnType<typeof useWorkspaceInspection>;

// Which view the workspace presents, and where: a main view wins; otherwise a
// dock too narrow to sit beside the conversation shows an inspected view full
// pane instead.
export function workspacePresentation({
  activeSessionId,
  dock,
  activeMainView,
  inspection,
  rowWidth,
}: {
  activeSessionId: string;
  dock: WorkspaceDock;
  activeMainView: string | null;
  inspection: WorkspaceInspection;
  rowWidth: number | null;
}) {
  const dockAvailable = canPresentDock(rowWidth ?? 0);
  const hasDockOwner = activeSessionId !== "";
  const showingCatalog = dock.activeViewId === WORKSPACE_DOCK_CATALOG;
  const dockOpen = hasDockOwner && dock.open && (showingCatalog || dock.viewIds.length > 0);
  const fullPaneViewId =
    activeMainView ??
    (dockOpen && rowWidth !== null && !dockAvailable && inspection?.viewId === dock.activeViewId
      ? dock.activeViewId
      : null);
  return {
    hasDockOwner,
    dockOpen,
    dockVisible: dockOpen && dockAvailable,
    ownedDockViewIds: hasDockOwner ? dock.viewIds : [],
    fullPaneViewId,
    inspectedFullPane: fullPaneViewId !== null && inspection?.viewId === fullPaneViewId,
  };
}

// Views stay mounted while open or while an inspection can return to them, so
// a return finds the reader where it was left.
export function mountedWorkspaceViews(
  openViewIds: ReadonlySet<string>,
  inspection: WorkspaceInspection,
): Set<string> {
  const mounted = new Set(openViewIds);
  for (let source = inspection; source; source = source.previous) {
    if (source.returnViewId && source.returnViewId !== source.viewId)
      mounted.add(source.returnViewId);
  }
  mounted.delete(WORKSPACE_DOCK_CATALOG);
  return mounted;
}
