import { type VisualClient } from "./client";
import { CONFIG } from "@/extensions/config";
import { definePlugin } from "@/extensions/definePlugin";
import { bindRuntimeEndpointConfiguration } from "@/features/connection/adapters/runtimeEndpointConfiguration";
import { RUNTIME_SERVER_SCOPE, RUNTIME_STREAM } from "@/features/connection/public/services";
import { createNavigationBootstrapPlugin } from "@/features/navigation/bootstrap";
export const visualRuntimeStream = (visual: VisualClient) =>
  definePlugin({
    name: "flame.visual.runtime-stream-ports",
    provides: { stream: RUNTIME_STREAM, scope: RUNTIME_SERVER_SCOPE },
    requires: { config: CONFIG },
    setup(ctx) {
      const configuration = bindRuntimeEndpointConfiguration(
        ctx,
        (commit) => commit(),
        () => ({
          endpoint: "https://visual.flame.test",
        }),
      );
      visual.bindEndpoint(configuration);
      return { scope: visual.scope, stream: visual.stream };
    },
  });

export const visualNavigationBootstrap = (visual: VisualClient) =>
  createNavigationBootstrapPlugin(visual.directory);
