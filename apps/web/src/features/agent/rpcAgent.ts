import { type FlameClient } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { t } from "@/lib/i18n/index";
import { AGENT_SOURCE } from "@/features/agent/adapters/runtimeSources";
import { getActiveSessionId } from "@/features/agent/public/session";
import { runtimeRunsGateway } from "@/features/agent/adapters/runtimeRunsGateway";
import { rpcAgentSource } from "@/features/agent/adapters/rpcAgentSource";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";
export function createRpcAgentPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.rpc-agent",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      const gateway = runtimeRunsGateway(runtimeClient);
      ctx.cleanup(() => gateway.dispose());
      ctx.cleanup(followRuntimeGeneration(ctx.runtime, () => gateway.replaceRuntimeGeneration()));
      ctx.contribute(
        AGENT_SOURCE,
        rpcAgentSource(t, getActiveSessionId, () => gateway),
      );
    },
  });
}
