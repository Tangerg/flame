import { type AgentAccess } from "@/features/agent/public/agent";

import { definePlugin } from "@/extensions/definePlugin";
import { READY_HANDLER } from "@/extensions/lifecycle";
import { WINDOW } from "@/features/shell/public/window";
export const windowTitle = (agentAccess: AgentAccess) =>
  definePlugin({
    name: "flame.builtin.window-title",
    requires: { window: WINDOW },
    setup(ctx) {
      ctx.contribute(READY_HANDLER, () => {
        ctx.cleanup(
          agentAccess.subscribeAnySessionRunning((working) => ctx.window.setWorking(working)),
        );
      });
    },
  });
