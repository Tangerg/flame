import { disposeOnHmr } from "@/lib/hmr";
import { subscribeAnySessionRunning } from "@/plugins/builtin/agent/public/run";
import { definePlugin, READY_HANDLER, WINDOW } from "@/plugins/sdk";

export const windowTitle = definePlugin({
  name: "flame.builtin.window-title",
  requires: { window: WINDOW },
  setup(ctx) {
    let unsubscribe: (() => void) | undefined;
    ctx.contribute(READY_HANDLER, () => {
      unsubscribe = subscribeAnySessionRunning((working) => ctx.window.setWorking(working));
      disposeOnHmr(unsubscribe);
    });
    ctx.cleanup(() => unsubscribe?.());
  },
});
