import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import { installComposerStatePorts } from "./adapters/composerStatePorts";

export const composerBootstrap = definePlugin({
  name: "flame.builtin.composer-bootstrap",
  requires: { sessions: AGENT_SESSIONS },
  setup(ctx) {
    ctx.cleanup(installComposerStatePorts(ctx.sessions));
  },
});
