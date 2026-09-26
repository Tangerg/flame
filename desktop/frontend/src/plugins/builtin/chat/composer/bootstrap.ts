import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";
import { RUNTIME_SERVER_SCOPE } from "@/plugins/builtin/runtime/public/services";
import { installComposerStatePorts } from "./adapters/composerStatePorts";

export const composerBootstrap = definePlugin({
  name: "flame.builtin.composer-bootstrap",
  requires: { sessions: AGENT_SESSIONS, scope: RUNTIME_SERVER_SCOPE },
  setup(ctx) {
    ctx.cleanup(installComposerStatePorts(ctx.sessions, currentRuntimeEndpoint, ctx.scope));
  },
});
