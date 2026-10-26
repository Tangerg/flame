import { type AgentAccess } from "@/features/agent/public/agent";
import { subscribeAnySessionRunning } from "@/features/agent/public/run";
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
          subscribeAnySessionRunning(agentAccess, (working) => ctx.window.setWorking(working)),
        );
      });
    },
  });
