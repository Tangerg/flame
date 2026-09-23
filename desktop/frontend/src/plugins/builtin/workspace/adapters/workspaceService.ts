import { definePlugin, WORKSPACE, type WorkspaceService } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey } from "@/plugins/sdk";
import { navigator } from "@/lib/navigation";
import { useContextDockStore } from "./contextDockStore";

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
