import { definePlugin } from "@/extensions/definePlugin";
import { AGENT_SESSIONS } from "@/features/agent/public/services";
import { currentRuntimeEndpoint } from "@/features/connection/public/endpoint";
import { RUNTIME_SERVER_SCOPE } from "@/features/connection/public/services";
import { installComposerStatePorts } from "@/features/composer/adapters/composerStatePorts";
export const composerBootstrap = definePlugin({
  name: "flame.builtin.composer-bootstrap",
  requires: { sessions: AGENT_SESSIONS, scope: RUNTIME_SERVER_SCOPE },
  setup(ctx) {
    ctx.cleanup(installComposerStatePorts(ctx.sessions, currentRuntimeEndpoint, ctx.scope));
  },
});
