import { subscribeAnySessionRunning } from "@/plugins/builtin/agent/public/run";
import { definePlugin, READY_HANDLER, WINDOW } from "@/plugins/sdk";

export const windowTitle = definePlugin({
  name: "flame.builtin.window-title",
  requires: { window: WINDOW },
  setup(ctx) {
    ctx.contribute(READY_HANDLER, () => {
      ctx.cleanup(subscribeAnySessionRunning((working) => ctx.window.setWorking(working)));
    });
  },
});
