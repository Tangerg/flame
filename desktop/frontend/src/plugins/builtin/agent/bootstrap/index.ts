import { registerAgentDataProviders } from "../adapters/runtimeDataProviders";
import type { FlameClient } from "@flame/runtime-contract/client";
import { definePlugin } from "@/plugins/sdk";
import { installAgentSessionScope } from "../adapters/agentSessionScope";
import { installAgentDefaultSessionPort } from "../adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "../adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "../adapters/agentStatePorts";
import { installInterruptResponseCoordinator } from "../application/hitl/interruptResponseCoordinator";
import {
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeAgentSessionLifecycle,
  subscribeDeletedAgentSession,
} from "@/plugins/builtin/agent/public/session";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import {
  RUNTIME_SERVER_SCOPE,
  RUNTIME_STREAM,
  followRuntimeGeneration,
} from "@/plugins/builtin/runtime/public/services";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";

export function createAgentBootstrapPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.agent-bootstrap",
    requires: { runtime: RUNTIME_STREAM, scope: RUNTIME_SERVER_SCOPE },
    provides: { sessions: AGENT_SESSIONS },
    setup(ctx) {
      registerAgentDataProviders(ctx, runtimeClient);
      ctx.cleanup(installAgentStatePorts());
      ctx.cleanup(installAgentDefaultSessionPort(runtimeClient));
      const runtimeGateway = installAgentRuntimeGateway(runtimeClient);
      ctx.cleanup(() => runtimeGateway.dispose());
      ctx.cleanup(
        followRuntimeGeneration(ctx.runtime, () => runtimeGateway.replaceRuntimeGeneration()),
      );
      ctx.cleanup(installInterruptResponseCoordinator());
      ctx.cleanup(installAgentSessionScope(ctx.scope, currentRuntimeEndpoint));
      return {
        sessions: {
          getActiveSessionId,
          getLifecycleSnapshot: getAgentSessionLifecycleSnapshot,
          subscribeActiveSessionId,
          subscribeLifecycle: subscribeAgentSessionLifecycle,
          subscribeDeleted: subscribeDeletedAgentSession,
        },
      };
    },
  });
}
