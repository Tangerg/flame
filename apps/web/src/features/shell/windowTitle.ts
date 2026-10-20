import { subscribeAnySessionRunning } from "@/features/agent/public/run";
import { definePlugin } from "@/extensions/definePlugin";
import { READY_HANDLER } from "@/extensions/lifecycle";
import { WINDOW } from "@/features/shell/public/window";
export const windowTitle = definePlugin({
  name: "flame.builtin.window-title",
  requires: { window: WINDOW },
  setup(ctx) {
    ctx.contribute(READY_HANDLER, () => {
      ctx.cleanup(subscribeAnySessionRunning((working) => ctx.window.setWorking(working)));
    });
  },
});
