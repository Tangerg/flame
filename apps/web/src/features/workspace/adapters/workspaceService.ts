import { definePlugin } from "@/extensions/definePlugin";
import { WORKSPACE, type WorkspaceService } from "@/features/workspace/public/workspaceService";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { navigator } from "@/lib/navigation";
import { useContextDockStore } from "@/features/workspace/adapters/contextDockStore";
const workspace: WorkspaceService = {
  openView(id) {
    if (!lookupExtensionByKey(WORKSPACE_VIEW, id)) {
      console.warn(`[plugin] workspace.openView("${id}"): no view registered`);
      return;
    }
    navigator().go({ view: id });
  },
  closeView(id) {
    if (navigator().get().view === id) navigator().go({ view: null });
    useContextDockStore.getState().closeDockTab(id);
  },
};

export const workspaceService = definePlugin({
  name: "flame.builtin.workspace-service",
  provides: { workspace: WORKSPACE },
  setup: () => ({ workspace }),
});
