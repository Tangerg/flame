import { PluginToaster } from "@/plugins/host/PluginToaster";
import { contributeLayout, definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "flame.builtin.toaster",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "toaster",
      order: 100,
      component: PluginToaster,
    });
  },
});
