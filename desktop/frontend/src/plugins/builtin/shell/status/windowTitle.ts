// WINDOW-level by design: any Session whose current root is Running lights it, not just the
// active tab's. Descendant lifecycle stays inside its root-owned tree — a child never
// becomes an unrelated window-level activity fact.
//
// Writes through the registry's single title composer so the dot and the count badge
// compose instead of clobbering each other.

import { disposeOnHmr } from "@/lib/hmr";
import { subscribeAnySessionRunning } from "@/plugins/builtin/agent/public/run";
import { definePlugin, READY_HANDLER, WINDOW } from "@/plugins/sdk";

export const windowTitle = definePlugin({
  name: "flame.builtin.window-title",
  requires: { window: WINDOW },
  setup(ctx) {
    // Subscribe to the "any run working" signal only once the app is READY.
    // subscribeAnySessionRunning reads the agent view-state port, which another
    // plugin's setup binds; readiness orders that dependency after all setup.
    let unsubscribe: (() => void) | undefined;
    ctx.contribute(READY_HANDLER, () => {
      unsubscribe = subscribeAnySessionRunning((working) => ctx.window.setWorking(working));
      disposeOnHmr(unsubscribe);
    });
    ctx.cleanup(() => unsubscribe?.());
  },
});
