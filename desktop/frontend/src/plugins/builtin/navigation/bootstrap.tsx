import { definePlugin, contributeLayout } from "@/plugins/sdk";
import { RUNTIME_SERVER_SCOPE } from "@/plugins/builtin/runtime/public/services";
import { installWorkingDirectoryPicker } from "./adapters/workingDirectorySelection";
import { WorkingDirectoryDialog } from "./ui/WorkingDirectoryDialog";

export default definePlugin({
  name: "flame.builtin.navigation-bootstrap",
  requires: { runtimeScope: RUNTIME_SERVER_SCOPE },
  setup(ctx) {
    const picker = installWorkingDirectoryPicker();
    ctx.cleanup(picker.dispose);
    ctx.cleanup(ctx.runtimeScope.subscribeReplacement(picker.cancel));
    contributeLayout(ctx, "app.overlay", {
      id: "navigation.directory-dialog",
      component: WorkingDirectoryDialog,
    });
  },
});
