import { CONFIG } from "@/extensions/config";
import { definePlugin } from "@/extensions/definePlugin";
import { bindRuntimeEndpointConfiguration } from "@/features/connection/adapters/runtimeEndpointConfiguration";
import { RUNTIME_SERVER_SCOPE, RUNTIME_STREAM } from "@/features/connection/public/services";
import { createNavigationBootstrapPlugin } from "@/features/navigation/bootstrap";
import { VISUAL_RUNTIME_GENERATION } from "./agentFixtureFacts";
export const visualRuntimeStream = definePlugin({
  name: "flame.visual.runtime-stream-ports",
  provides: { stream: RUNTIME_STREAM, scope: RUNTIME_SERVER_SCOPE },
  requires: { config: CONFIG },
  setup(ctx) {
    bindRuntimeEndpointConfiguration(ctx, (commit) => commit(), {
      endpoint: "https://visual.flame.test",
    });
    return {
      scope: { subscribeReplacement: () => () => undefined },
      stream: {
        connectionGeneration: () => VISUAL_RUNTIME_GENERATION,
        subscribeConnection: () => () => undefined,
        reportConnectionLoss: () => Promise.resolve(),
      },
    };
  },
});

export const visualNavigationBootstrap = createNavigationBootstrapPlugin(
  async () => "/Users/visual/selected-project",
  () => true,
);
