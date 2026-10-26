import { definePlugin } from "@/extensions/definePlugin";
import { WORKSPACE, type WorkspaceService } from "@/features/workspace/public/workspaceService";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { type Navigator } from "@/lib/navigation";
import { type WorkspaceOwner } from "@/features/workspace/adapters/workspace";
export function createWorkspaceService(navigation: Navigator, owner: WorkspaceOwner) {
  const workspace: WorkspaceService = {
    openView(id) {
      if (!lookupExtensionByKey(WORKSPACE_VIEW, id)) {
        console.warn(`[plugin] workspace.openView("${id}"): no view registered`);
        return;
      }
      navigation.go({ view: id });
    },
    closeView(id) {
      if (navigation.get().view === id) navigation.go({ view: null });
      owner.dock.store.getState().closeDockTab(id);
    },
  };

  return definePlugin({
    name: "flame.builtin.workspace-service",
    provides: { workspace: WORKSPACE },
    setup: () => ({ workspace }),
  });
}
