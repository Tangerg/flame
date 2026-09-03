import { describe, expect, it } from "vitest";
import * as workspaceViews from "./workspace/workspace-views";
import contextDockDestinations from "./workspace/context-dock";
import diagnostics from "./workspace/diagnostics";
import iconGallery from "./settings/icon-gallery";
import { kernelSettings } from "./shell/kernel";
import { lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { CONTEXT_DOCK_DESTINATION, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

// A composition invariant, so it lives with the manifest rather than inside one
// context: destinations are contributed by one plugin and the views they name by
// several others, and it is the assembled set that has to agree. Read off the
// registry — the same data the dock's add-panel menu reads.
describe("assembled context dock destinations", () => {
  async function assemble() {
    // One kernel holding all of them: the invariant is about the ASSEMBLED set,
    // and each call stands up a fresh Host.
    await loadPluginsForTest(
      ...Object.values(workspaceViews),
      diagnostics,
      // The two that register a view outside the views registry. They were missing, so the
      // set this reasons about was not the assembled one — which is how `icon-gallery` was
      // registered, listed nowhere, and invisible to a file whose whole job is to catch that.
      // A view-contributing plugin left out of this list still fails the first assertion if
      // it owns any destination; one that owns none is the remaining blind spot.
      iconGallery,
      kernelSettings,
      contextDockDestinations,
    );
    return {
      destinations: lookupExtensionPoint(CONTEXT_DOCK_DESTINATION),
      views: new Map(lookupExtensionPoint(WORKSPACE_VIEW).map((view) => [view.id, view])),
    };
  }

  // A destination whose viewId no longer resolves would render as a title-less
  // ghost (resolveContextDockItems drops it), so the menu would silently
  // lose an entry.
  it("every destination names a registered view", async () => {
    const { destinations, views } = await assemble();

    const missing = destinations
      .map((destination) => destination.viewId)
      .filter((viewId) => !views.has(viewId));

    expect(missing).toEqual([]);
  });

  // The reverse direction, which nothing checked: a view that can sit in the dock and is
  // listed nowhere never appears in the add-panel menu — which is how the Inbox and Tool
  // stats views shipped unreachable. `splittable` is the question, not the whole registry:
  // a view that cannot sit in the dock is not missing from this list, it does not belong in
  // it, and the command menu opens it on the content card instead.
  it("lists every view that can sit in the dock", async () => {
    const { destinations, views } = await assemble();
    const listed = new Set(destinations.map((destination) => destination.viewId));

    const missing = [...views.values()]
      .filter((view) => view.splittable === true)
      .map((view) => view.id)
      .filter((viewId) => !listed.has(viewId));

    expect(missing).toEqual([]);
  });

  // Every destination opens in the dock, so a destination that cannot live there
  // is a one-way trip: the view would have no "open in the dock" affordance to
  // get back with.
  it("every destination's view can sit in the dock", async () => {
    const { destinations, views } = await assemble();

    const notSplittable = destinations
      .map((destination) => destination.viewId)
      .filter((viewId) => views.get(viewId)?.splittable !== true);

    expect(notSplittable).toEqual([]);
  });
});
