import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { AGENT_SOURCE } from "@/plugins/sdk/kernelPoints";
import { getActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { runtimeRunsGateway } from "./adapters/runtimeRunsGateway";
import { rpcAgentSource } from "./application/rpcAgentSource";
import {
  RUNTIME_STREAM_PORTS,
  followRuntimeGeneration,
} from "@/plugins/builtin/runtime/public/ports";

export default definePlugin({
  name: "flame.builtin.rpc-agent",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = runtimeRunsGateway();
    const unsubscribeRuntime = followRuntimeGeneration(ctx.runtime, () =>
      gateway.replaceRuntimeGeneration(),
    );
    ctx.contribute(
      AGENT_SOURCE,
      rpcAgentSource(t, getActiveSessionId, () => gateway),
    );
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
