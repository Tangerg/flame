import { definePlugin } from "@/extensions/definePlugin";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { RUNTIME_SERVER_SCOPE } from "@/features/connection/public/services";
import { type createWorkingDirectoryPicker } from "@/features/navigation/adapters/workingDirectorySelection";
import { WorkingDirectoryDialog } from "@/features/navigation/ui/WorkingDirectoryDialog";
export function createNavigationBootstrapPlugin(
  picker: ReturnType<typeof createWorkingDirectoryPicker>,
) {
  return definePlugin({
    name: "flame.builtin.navigation-bootstrap",
    requires: { runtimeScope: RUNTIME_SERVER_SCOPE },
    setup(ctx) {
      ctx.cleanup(picker.dispose);
      ctx.cleanup(ctx.runtimeScope.subscribeReplacement(picker.cancel));
      contributeLayout(ctx, "app.overlay", {
        id: "navigation.directory-dialog",
        component: WorkingDirectoryDialog,
      });
    },
  });
}
