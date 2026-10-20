import { registerAgentDataProviders } from "@/features/agent/adapters/runtimeDataProviders";
import { type FlameClient } from "@flame/runtime-client";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { PendingRunOpenings } from "@/features/agent/ui/PendingRunOpenings";
import { installAgentSessionScope } from "@/features/agent/adapters/agentSessionScope";
import { installAgentDefaultSessionPort } from "@/features/agent/adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "@/features/agent/adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "@/features/agent/adapters/agentStatePorts";
import { installInterruptResponseCoordinator } from "@/features/agent/application/hitl/interruptResponseCoordinator";
import {
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeAgentSessionLifecycle,
  subscribeDeletedAgentSession,
} from "@/features/agent/public/session";
import { AGENT_SESSIONS } from "@/features/agent/public/services";
import {
  RUNTIME_SERVER_SCOPE,
  RUNTIME_STREAM,
  followRuntimeGeneration,
} from "@/features/connection/public/services";
import { currentRuntimeEndpoint } from "@/features/connection/public/endpoint";
export function createAgentBootstrapPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.agent-bootstrap",
    requires: { runtime: RUNTIME_STREAM, scope: RUNTIME_SERVER_SCOPE },
    provides: { sessions: AGENT_SESSIONS },
    setup(ctx) {
      contributeLayout(ctx, "chat.banner.top", {
        id: "pending-run-openings",
        order: 0,
        component: PendingRunOpenings,
      });
      registerAgentDataProviders(ctx, runtimeClient);
      ctx.cleanup(installAgentStatePorts());
      ctx.cleanup(
        installAgentDefaultSessionPort(runtimeClient, () => {
          const generation = ctx.runtime.connectionGeneration();
          if (generation) ctx.runtime.reportConnectionLoss(generation);
        }),
      );
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
