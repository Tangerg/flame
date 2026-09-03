import { describe, expect, it } from "vitest";
import {
  groupContextDockDestinations,
  resolveContextDockItems,
  type ContextDockItem,
} from "./contextDockDestinationGroups";

const item = (
  patch: Pick<ContextDockItem, "viewId" | "scope"> & Partial<ContextDockItem>,
): ContextDockItem => ({
  title: `title.${patch.viewId}`,
  ...patch,
});

describe("resolveContextDockItems", () => {
  it("takes the views that name a scope and leaves the rest to the content card", () => {
    const items = resolveContextDockItems([
      {
        id: "search",
        title: "workspace.view.title.search",
        icon: "search",
        order: 10,
        dock: "workspace",
      },
      { id: "settings", title: "settings.title", icon: "settings", order: 200 },
    ]);

    expect(items).toEqual([
      {
        viewId: "search",
        title: "workspace.view.title.search",
        icon: "search",
        scope: "workspace",
        order: 10,
      },
    ]);
  });
});

describe("groupContextDockDestinations", () => {
  it("groups items by workspace mental-model scope and sorts by order", () => {
    const groups = groupContextDockDestinations([
      item({ viewId: "timeline", scope: "session", order: 10 }),
      item({ viewId: "plan", scope: "run", order: 10 }),
      item({ viewId: "files", scope: "workspace", order: 20 }),
      item({ viewId: "search", scope: "workspace", order: 10 }),
    ]);

    expect(groups.map((group) => group.id)).toEqual(["workspace", "run", "session"]);
    expect(groups[0]?.destinations.map((d) => d.viewId)).toEqual(["search", "files"]);
    expect(groups[1]?.destinations.map((d) => d.viewId)).toEqual(["plan"]);
    expect(groups[2]?.destinations.map((d) => d.viewId)).toEqual(["timeline"]);
  });
});
