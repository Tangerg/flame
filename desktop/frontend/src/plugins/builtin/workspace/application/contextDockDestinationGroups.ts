import type { ContextDockDestinationScope, WorkspaceViewSpec } from "@/plugins/sdk";

// A view that has told the dock where it belongs. This is the catalog rendered by the
// add-panel menu.
export interface ContextDockItem {
  viewId: string;
  title: string;
  icon?: string;
  scope: ContextDockDestinationScope;
  order?: number;
}

export interface ContextDockDestinationGroup {
  id: ContextDockDestinationScope;
  title: string;
  destinations: ContextDockItem[];
}

/** A view that names no scope is not missing from the menu — it takes the content card
 *  instead, and the command menu opens it there. */
export function resolveContextDockItems(
  views: readonly Pick<WorkspaceViewSpec, "id" | "title" | "icon" | "order" | "dock">[],
): ContextDockItem[] {
  const items: ContextDockItem[] = [];
  for (const view of views) {
    if (view.dock === undefined) continue;
    items.push({
      viewId: view.id,
      title: view.title,
      icon: view.icon,
      scope: view.dock,
      order: view.order,
    });
  }
  return items;
}

const groupOrder: Array<{ id: ContextDockDestinationScope; title: string }> = [
  { id: "workspace", title: "contextDock.group.workspace" },
  { id: "run", title: "contextDock.group.run" },
  { id: "session", title: "contextDock.group.session" },
];

export function groupContextDockDestinations(
  items: ContextDockItem[],
): ContextDockDestinationGroup[] {
  return groupOrder
    .map((group) => ({
      ...group,
      destinations: items
        .filter((item) => item.scope === group.id)
        .sort(
          (a, b) => (a.order ?? Number.MAX_SAFE_INTEGER) - (b.order ?? Number.MAX_SAFE_INTEGER),
        ),
    }))
    .filter((group) => group.destinations.length > 0);
}
