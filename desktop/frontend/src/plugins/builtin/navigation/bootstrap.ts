import { definePlugin } from "@/plugins/sdk";
import { installDesktopWorkingDirectoryPicker } from "./adapters/desktopWorkingDirectoryPicker";

export default definePlugin({
  name: "flame.builtin.navigation-bootstrap",
  setup(ctx) {
    ctx.cleanup(installDesktopWorkingDirectoryPicker());
  },
});
