import { definePlugin } from "@/extensions/definePlugin";
import { AGENT_SESSIONS } from "@/features/agent/public/services";
import { RUNTIME_SERVER_SCOPE } from "@/features/connection/public/services";
import { type ComposerOwner } from "@/features/composer/adapters/composerStatePorts";
export function createComposerBootstrap(composer: ComposerOwner) {
  return definePlugin({
    name: "flame.builtin.composer-bootstrap",
    requires: { sessions: AGENT_SESSIONS, scope: RUNTIME_SERVER_SCOPE },
    setup(ctx) {
      composer.start();
      ctx.cleanup(composer.dispose);
    },
  });
}
